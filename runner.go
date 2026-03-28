package daneel

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// run executes the agent loop. This is the core of Daneel.
//
// Loop:
//  1. Build messages (system prompt + context funcs + history + user input)
//  2. Apply context window management (truncate if needed)
//  3. Run input guards
//  4. Call provider LLM
//  5. If response has ToolCalls → check permissions, execute, loop
//  6. Run output guards
//  7. Return RunResult
func run(ctx context.Context, agent *Agent, input string, opts ...RunOption) (*RunResult, error) {
	cfg := defaultRunConfig()
	applyRunOptions(opts, &cfg)

	// Pre-run hook (e.g. quota checks from tenant middleware).
	if cfg.preRunHook != nil {
		if err := cfg.preRunHook(ctx); err != nil {
			return nil, err
		}
	}

	// Resolve session ID
	sessionID := cfg.sessionID
	if sessionID == "" {
		sessionID = NewSessionID()
	}

	// Resolve provider
	provider := agent.config.provider
	if provider == nil {
		return nil, ErrNoProvider
	}

	// Build system prompt (instructions + context funcs)
	systemPrompt, err := agent.buildSystemPrompt(ctx)
	if err != nil {
		return nil, fmt.Errorf("context function failed: %w", err)
	}

	// Resolve max turns
	maxTurns := agent.config.maxTurns
	if cfg.maxTurns > 0 {
		maxTurns = cfg.maxTurns
	}
	if maxTurns <= 0 {
		maxTurns = 25
	}

	// Prepare tools: agent tools + handoff synthetic tools
	allTools := make([]Tool, 0, len(agent.config.tools)+len(agent.config.handoffs))
	allTools = append(allTools, agent.config.tools...)
	if len(agent.config.handoffs) > 0 {
		allTools = append(allTools, makeHandoffTools(agent.config.handoffs)...)
	}

	// Build ToolDef slice for the provider
	toolDefs := make([]ToolDef, len(allTools))
	for i, t := range allTools {
		toolDefs[i] = t.Def()
	}

	// Build tool lookup map
	toolMap := make(map[string]Tool, len(allTools))
	for _, t := range allTools {
		toolMap[t.Name] = t
	}

	// Initialize conversation
	var messages []Message
	if systemPrompt != "" {
		messages = append(messages, SystemMessage(systemPrompt))
	}

	// Load history from memory if available
	if agent.config.memory != nil {
		history, err := agent.config.memory.Retrieve(ctx, sessionID, "", 0)
		if err == nil && len(history) > 0 {
			messages = append(messages, history...)
		}
	}

	// Load history from session store (raw persistent history takes precedence
	// over in-memory history when both are configured).
	if agent.config.sessionStore != nil {
		stored, err := agent.config.sessionStore.LoadMessages(ctx, sessionID)
		if err == nil && len(stored) > 0 {
			// Replace any memory-retrieved history with the persisted one.
			// Remove the system message we already prepended, rebuild from store.
			base := messages[:0]
			for _, m := range messages {
				if m.Role == RoleSystem {
					base = append(base, m)
					break
				}
			}
			messages = append(base, stored...)
		}
	}

	// Prepend externally-provided history (e.g. from Bridge)
	if len(cfg.history) > 0 && agent.config.memory == nil {
		messages = append(messages, cfg.history...)
	}

	// Add user input
	userMsg := UserMessage(input)
	if len(cfg.images) > 0 {
		userMsg.ContentParts = cfg.images
	}
	messages = append(messages, userMsg)

	// Run input guards
	for _, guard := range agent.config.inputGuards {
		if err := guard(ctx, input); err != nil {
			return nil, &GuardError{Agent: agent.name, Guard: "input", Message: err.Error()}
		}
	}

	// Result tracking
	startTime := time.Now()
	var totalUsage Usage
	var toolCallRecords []ToolCallRecord

	// Tracing: create root span
	tracer := agent.config.tracer
	if tracer == nil {
		tracer = defaultTracer{}
	}
	ctx, rootSpan := tracer.StartSpan(ctx, "daneel.run",
		Attr{Key: "agent.name", Value: agent.name},
		Attr{Key: "session.id", Value: sessionID},
	)
	defer rootSpan.End()

	// Rate limiting state
	var toolCallsInWindow int
	windowStart := startTime

	// Inject response format into system prompt if requested
	if cfg.responseFormat == JSON {
		formatInstr := "\n\nYou MUST respond with valid JSON only. No markdown, no explanation, just the JSON object."
		if cfg.responseSchema != nil {
			if schemaJSON, err := json.Marshal(cfg.responseSchema); err == nil {
				formatInstr = "\n\nYou MUST respond with valid JSON matching this schema:\n" + string(schemaJSON) + "\nRespond with valid JSON only. No markdown, no explanation."
			}
		}
		if len(messages) > 0 && messages[0].Role == RoleSystem {
			messages[0].Content += formatInstr
		} else {
			messages = append([]Message{{Role: RoleSystem, Content: formatInstr[2:]}}, messages...)
		}
	}

	// Agent loop
	for turn := 0; turn < maxTurns; turn++ {
		// Apply context window management
		messages = truncateMessages(messages, agent.config.contextStrategy)

		// Call LLM
		chatCtx, chatSpan := tracer.StartSpan(ctx, "daneel.llm.chat",
			Attr{Key: "turn", Value: turn},
		)
		resp, err := provider.Chat(chatCtx, messages, toolDefs)
		if err != nil {
			chatSpan.RecordError(err)
			chatSpan.End()
			rootSpan.RecordError(err)
			return nil, err
		}
		chatSpan.SetAttributes(
			Attr{Key: "usage.prompt_tokens", Value: resp.Usage.PromptTokens},
			Attr{Key: "usage.completion_tokens", Value: resp.Usage.CompletionTokens},
		)
		chatSpan.End()

		// Accumulate usage
		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens

		// No tool calls → final response
		if len(resp.ToolCalls) == 0 {
			// Run output guards
			for _, guard := range agent.config.outputGuards {
				if err := guard(ctx, resp.Content); err != nil {
					return nil, &GuardError{Agent: agent.name, Guard: "output", Message: err.Error()}
				}
			}

			// Append assistant message
			messages = append(messages, AssistantMessage(resp.Content))

			// Save to memory
			if agent.config.memory != nil {
				_ = agent.config.memory.Save(ctx, sessionID, messages)
			}

			// Save to session store (raw persistent history)
			if agent.config.sessionStore != nil {
				// Strip the system message before persisting — it is
				// rebuilt at the start of every Run() from instructions.
				persisted := make([]Message, 0, len(messages))
				for _, m := range messages {
					if m.Role != RoleSystem {
						persisted = append(persisted, m)
					}
				}
				_ = agent.config.sessionStore.SaveMessages(ctx, sessionID, persisted)
			}

			// Stream done
			if cfg.streamFn != nil {
				cfg.streamFn(StreamChunk{Type: StreamDone})
			}

			result := &RunResult{
				Output:    resp.Content,
				Messages:  messages,
				ToolCalls: toolCallRecords,
				Turns:     turn + 1,
				Usage:     totalUsage,
				Duration:  time.Since(startTime),
				AgentName: agent.name,
				SessionID: sessionID,
			}
			fireOnConversationEnd(ctx, agent, *result)
			if cfg.postRunHook != nil {
				cfg.postRunHook(ctx, result)
			}
			return result, nil
		}

		// Has tool calls — add assistant message with tool calls
		assistantMsg := Message{
			Role:      RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		// Rate limiting check
		if agent.config.rateLimit > 0 {
			if time.Since(windowStart) >= time.Minute {
				toolCallsInWindow = 0
				windowStart = time.Now()
			}
			toolCallsInWindow += len(resp.ToolCalls)
			if toolCallsInWindow > agent.config.rateLimit {
				wait := time.Minute - time.Since(windowStart)
				select {
				case <-time.After(wait):
					toolCallsInWindow = len(resp.ToolCalls)
					windowStart = time.Now()
				case <-ctx.Done():
					rootSpan.RecordError(ctx.Err())
					return nil, ctx.Err()
				}
			}
		}

		// Execute tool calls
		results := executeToolCalls(ctx, agent, &cfg, toolMap, resp.ToolCalls, sessionID)

		for _, tr := range results {
			toolCallRecords = append(toolCallRecords, tr.record)

			// Check for handoff
			if tr.handoff != nil {
				target := tr.handoff

				// Inherit provider if target has none
				targetAgent := target
				if targetAgent.config.provider == nil {
					cp := targetAgent.clone()
					cp.config.provider = provider
					targetAgent = cp
				}

				// Prepare history for handoff
				handoffMsgs := prepareHandoffHistory(messages, agent.config.handoffHistory)

				// Run the target agent with handoff history
				handoffOpts := append([]RunOption{WithHistory(handoffMsgs)}, opts...)
				handoffResult, err := run(ctx, targetAgent, tr.handoffReason, handoffOpts...)
				if err != nil {
					return nil, err
				}
				handoffResult.HandoffFrom = agent.name

				fireOnConversationEnd(ctx, agent, *handoffResult)
				if cfg.postRunHook != nil {
					cfg.postRunHook(ctx, handoffResult)
				}
				return handoffResult, nil
			}

			// Add tool result message
			messages = append(messages, tr.message)
		}

		// Stream tool call done events
		if cfg.streamFn != nil {
			for _, tr := range results {
				cfg.streamFn(StreamChunk{
					Type:       StreamToolCallDone,
					ToolResult: &ToolResult{ToolCallID: tr.record.Name, Content: tr.record.Result},
				})
			}
		}
	}

	// Exceeded max turns
	partial := &RunResult{
		Output:    "",
		Messages:  messages,
		ToolCalls: toolCallRecords,
		Turns:     maxTurns,
		Usage:     totalUsage,
		Duration:  time.Since(startTime),
		AgentName: agent.name,
		SessionID: sessionID,
	}
	fireOnConversationEnd(ctx, agent, *partial)
	return nil, &MaxTurnsError{
		Agent:    agent.name,
		MaxTurns: maxTurns,
		Partial:  partial,
	}
}

// toolCallResult is the internal result of a single tool execution.
type toolCallResult struct {
	record        ToolCallRecord
	message       Message
	handoff       *Agent // non-nil if this was a handoff
	handoffReason string
}

// executeToolCalls runs all tool calls from a single LLM response,
// respecting the configured concurrency mode.
func executeToolCalls(
	ctx context.Context,
	agent *Agent,
	cfg *runConfig,
	toolMap map[string]Tool,
	calls []ToolCall,
	sessionID string,
) []toolCallResult {
	p := agent.config.toolExecution.parallelism()

	if p == 1 || len(calls) == 1 {
		// Sequential execution
		results := make([]toolCallResult, len(calls))
		for i, call := range calls {
			results[i] = executeSingleTool(ctx, agent, cfg, toolMap, call, sessionID)
		}
		return results
	}

	// Parallel execution
	results := make([]toolCallResult, len(calls))
	var wg sync.WaitGroup

	if p == 0 {
		// Unlimited parallelism
		for i, call := range calls {
			wg.Add(1)
			go func(idx int, c ToolCall) {
				defer wg.Done()
				results[idx] = executeSingleTool(ctx, agent, cfg, toolMap, c, sessionID)
			}(i, call)
		}
	} else {
		// Limited parallelism via semaphore
		sem := make(chan struct{}, p)
		for i, call := range calls {
			wg.Add(1)
			go func(idx int, c ToolCall) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				results[idx] = executeSingleTool(ctx, agent, cfg, toolMap, c, sessionID)
			}(i, call)
		}
	}

	wg.Wait()
	return results
}

// executeSingleTool handles permission checks, approval, and execution
// of a single tool call.
func executeSingleTool(
	ctx context.Context,
	agent *Agent,
	cfg *runConfig,
	toolMap map[string]Tool,
	call ToolCall,
	sessionID string,
) toolCallResult {
	startTime := time.Now()

	// Tool tracing
	toolTracer := agent.config.tracer
	if toolTracer == nil {
		toolTracer = defaultTracer{}
	}
	ctx, toolSpan := toolTracer.StartSpan(ctx, "daneel.tool."+call.Name,
		Attr{Key: "tool.name", Value: call.Name},
	)
	defer toolSpan.End()

	// Check if it's a handoff
	if isHandoffTool(call.Name) {
		targetName := handoffTargetName(call.Name)

		// Check handoff permissions
		if reason, ok := agent.perms.checkHandoff(targetName); !ok {
			return permissionDeniedResult(agent, call, reason, startTime)
		}

		target := findHandoffTarget(agent.config.handoffs, targetName)
		if target == nil {
			return errorResult(call, fmt.Sprintf("handoff target %q not found", targetName), startTime)
		}

		return toolCallResult{
			record: ToolCallRecord{
				Name:      call.Name,
				Arguments: call.Arguments,
				Result:    "handoff",
				Duration:  time.Since(startTime),
				Permitted: true,
			},
			handoff:       target,
			handoffReason: parseHandoffArgs(call.Arguments),
		}
	}

	// Check tool permissions
	if reason, ok := agent.perms.checkTool(call.Name); !ok {
		if agent.config.strictPermissions {
			return toolCallResult{
				record: ToolCallRecord{
					Name:      call.Name,
					Arguments: call.Arguments,
					Result:    reason,
					IsError:   true,
					Duration:  time.Since(startTime),
					Permitted: false,
				},
				message: ToolResult{
					ToolCallID: call.ID,
					Content:    fmt.Sprintf("Permission denied: %s", reason),
					IsError:    true,
				}.ToMessage(),
			}
		}
		return permissionDeniedResult(agent, call, reason, startTime)
	}

	// Find tool
	tool, ok := toolMap[call.Name]
	if !ok {
		return errorResult(call, fmt.Sprintf("tool %q not found", call.Name), startTime)
	}

	// Check approval if required
	if tool.requireApproval {
		if cfg.approver == nil {
			return errorResult(call, "tool requires approval but no approver configured", startTime)
		}

		approved, err := cfg.approver.Approve(ctx, ApprovalRequest{
			Agent:     agent.name,
			Tool:      call.Name,
			Args:      call.Arguments,
			SessionID: sessionID,
		})
		if err != nil {
			return errorResult(call, fmt.Sprintf("approval error: %v", err), startTime)
		}
		if !approved {
			return errorResult(call, "tool call denied by approver", startTime)
		}
	}

	// Apply default timeout if tool has none
	toolCtx := ctx
	if tool.timeout == 0 && agent.config.defaultToolTimeout > 0 {
		var cancel context.CancelFunc
		toolCtx, cancel = context.WithTimeout(ctx, agent.config.defaultToolTimeout)
		defer cancel()
	}

	// Execute tool
	result, err := tool.Run(toolCtx, call.Arguments)
	duration := time.Since(startTime)

	if err != nil {
		return toolCallResult{
			record: ToolCallRecord{
				Name:      call.Name,
				Arguments: call.Arguments,
				Result:    err.Error(),
				IsError:   true,
				Duration:  duration,
				Permitted: true,
			},
			message: ToolResult{
				ToolCallID: call.ID,
				Content:    err.Error(),
				IsError:    true,
			}.ToMessage(),
		}
	}

	return toolCallResult{
		record: ToolCallRecord{
			Name:      call.Name,
			Arguments: call.Arguments,
			Result:    result,
			Duration:  duration,
			Permitted: true,
		},
		message: ToolResult{
			ToolCallID: call.ID,
			Content:    result,
		}.ToMessage(),
	}
}

func permissionDeniedResult(agent *Agent, call ToolCall, reason string, start time.Time) toolCallResult {
	msg := fmt.Sprintf("You don't have permission to use tool %q: %s", call.Name, reason)
	return toolCallResult{
		record: ToolCallRecord{
			Name:      call.Name,
			Arguments: call.Arguments,
			Result:    msg,
			IsError:   true,
			Duration:  time.Since(start),
			Permitted: false,
		},
		message: ToolResult{
			ToolCallID: call.ID,
			Content:    msg,
			IsError:    true,
		}.ToMessage(),
	}
}

func errorResult(call ToolCall, msg string, start time.Time) toolCallResult {
	return toolCallResult{
		record: ToolCallRecord{
			Name:      call.Name,
			Arguments: call.Arguments,
			Result:    msg,
			IsError:   true,
			Duration:  time.Since(start),
			Permitted: true,
		},
		message: ToolResult{
			ToolCallID: call.ID,
			Content:    msg,
			IsError:    true,
		}.ToMessage(),
	}
}

// truncateMessages applies context window management. For now,
// implements the sliding window heuristic. Full implementations
// of ContextSummarize and exact token counting will be in context_mgmt.go.
func truncateMessages(msgs []Message, strategy ContextStrategy) []Message {
	if strategy == ContextError {
		return msgs // caller handles overflow via provider error
	}

	// Estimate tokens: ~4 chars per token for English
	const maxTokens = 100_000 // conservative default
	var totalChars int
	for _, m := range msgs {
		totalChars += len(m.Content)
		for _, tc := range m.ToolCalls {
			totalChars += len(tc.Arguments)
		}
	}

	estimatedTokens := totalChars / 4
	if estimatedTokens <= maxTokens {
		return msgs // fits within window
	}

	// Sliding window: keep system prompt (first msg if system) + last N that fit
	if len(msgs) < 3 {
		return msgs
	}

	var preserved []Message
	if msgs[0].Role == RoleSystem {
		preserved = append(preserved, msgs[0])
		msgs = msgs[1:]
	}

	// Keep removing oldest messages until we fit
	for estimatedTokens > maxTokens && len(msgs) > 1 {
		removed := msgs[0]
		removedChars := len(removed.Content)
		for _, tc := range removed.ToolCalls {
			removedChars += len(tc.Arguments)
		}
		estimatedTokens -= removedChars / 4
		msgs = msgs[1:]
	}

	return append(preserved, msgs...)
}

// fireOnConversationEnd invokes all registered conversation-end callbacks.
func fireOnConversationEnd(ctx context.Context, agent *Agent, result RunResult) {
	for _, fn := range agent.config.onConversationEnd {
		fn(ctx, result)
	}
}
