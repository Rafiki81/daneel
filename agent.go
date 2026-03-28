package daneel

import "context"

// Agent is an immutable configuration for an LLM-powered agent.
// Create with New() and functional options. All .With*() methods
// return a new Agent (copy-on-modify), so it's safe to share and
// derive agents across goroutines.
type Agent struct {
	name   string
	config agentConfig
	perms  permissionSet // compiled permissions (cached)
}

// New creates a new Agent with the given name and options.
//
//	agent := daneel.New("support",
//	    daneel.WithInstructions("You handle customer support"),
//	    daneel.WithModel("gpt-4o"),
//	    daneel.WithTools(searchTool, replyTool),
//	)
func New(name string, opts ...AgentOption) *Agent {
	cfg := agentConfig{
		maxTurns:        25,
		contextStrategy: ContextSlidingWindow,
		toolExecution:   Sequential,
		handoffHistory:  FullHistory,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Agent{
		name:   name,
		config: cfg,
		perms:  compilePermissions(cfg.permissions),
	}
}

// Name returns the agent's name.
func (a *Agent) Name() string { return a.name }

// Instructions returns the agent's system prompt.
func (a *Agent) Instructions() string { return a.config.instructions }

// Tools returns the agent's configured tools.
func (a *Agent) Tools() []Tool { return a.config.tools }

// Provider returns the agent's LLM provider (may be nil if using
// convenience shortcuts that are resolved at Run time).
func (a *Agent) Provider() Provider { return a.config.provider }

// clone returns a deep-enough copy of the agent for copy-on-modify.
func (a *Agent) clone() *Agent {
	cp := *a
	cp.config.tools = append([]Tool(nil), a.config.tools...)
	cp.config.handoffs = append([]*Agent(nil), a.config.handoffs...)
	cp.config.permissions = append([]PermissionRule(nil), a.config.permissions...)
	cp.config.inputGuards = append([]InputGuard(nil), a.config.inputGuards...)
	cp.config.outputGuards = append([]OutputGuard(nil), a.config.outputGuards...)
	cp.config.contextFuncs = append([]func(ctx context.Context) (string, error)(nil), a.config.contextFuncs...)
	return &cp
}

// WithName returns a new Agent with a different name.
func (a *Agent) WithName(name string) *Agent {
	cp := a.clone()
	cp.name = name
	return cp
}

// WithExtraTools returns a new Agent with additional tools appended.
func (a *Agent) WithExtraTools(tools ...Tool) *Agent {
	cp := a.clone()
	cp.config.tools = append(cp.config.tools, tools...)
	return cp
}

// WithExtraInstructions returns a new Agent with additional instructions
// appended to the existing ones.
func (a *Agent) WithExtraInstructions(extra string) *Agent {
	cp := a.clone()
	if cp.config.instructions != "" {
		cp.config.instructions += "\n\n" + extra
	} else {
		cp.config.instructions = extra
	}
	return cp
}

// WithProvider returns a new Agent with a different provider.
// Useful for testing with mock.New().
func (a *Agent) WithProvider(p Provider) *Agent {
	cp := a.clone()
	cp.config.provider = p
	return cp
}

// buildSystemPrompt constructs the full system prompt by combining
// static instructions and dynamic context functions. Called by the
// Runner at the start of each Run().
func (a *Agent) buildSystemPrompt(ctx context.Context) (string, error) {
	prompt := a.config.instructions

	for _, fn := range a.config.contextFuncs {
		extra, err := fn(ctx)
		if err != nil {
			return "", err
		}
		if extra != "" {
			if prompt != "" {
				prompt += "\n\n"
			}
			prompt += extra
		}
	}

	return prompt, nil
}
