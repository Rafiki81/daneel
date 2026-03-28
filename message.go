// Package daneel provides a minimal, composable library for orchestrating
// LLM agents with declarative permissions and built-in platform integrations.
//
// Named after R. Daneel Olivaw — the robot who silently orchestrated
// humanity for 20,000 years in Asimov's Foundation/Robot saga.
package daneel

import (
	"encoding/json"

	"github.com/Rafiki81/daneel/content"
)

// Role represents the role of a message participant.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message represents a single message in a conversation.
type Message struct {
	Role         Role              `json:"role"`
	Content      string            `json:"content,omitempty"`
	Name         string            `json:"name,omitempty"`
	ToolCalls    []ToolCall        `json:"tool_calls,omitempty"`
	ToolCallID   string            `json:"tool_call_id,omitempty"`
	ContentParts []content.Content `json:"content_parts,omitempty"` // multi-modal content
}

// ToolCall represents a request from the LLM to invoke a tool.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult represents the result of executing a tool.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

// ToMessage converts a ToolResult into a Message that can be appended
// to the conversation history.
func (r ToolResult) ToMessage() Message {
	content := r.Content
	if r.IsError {
		content = "Error: " + content
	}
	return Message{
		Role:       RoleTool,
		Content:    content,
		Name:       r.Name,
		ToolCallID: r.ToolCallID,
	}
}

// SystemMessage creates a new system message.
func SystemMessage(content string) Message {
	return Message{Role: RoleSystem, Content: content}
}

// UserMessage creates a new user message.
func UserMessage(content string) Message {
	return Message{Role: RoleUser, Content: content}
}

// AssistantMessage creates a new assistant message.
func AssistantMessage(c string) Message {
	return Message{Role: RoleAssistant, Content: c}
}

// MultiModalMessage creates a user message with text and multi-modal content parts.
func MultiModalMessage(text string, parts ...content.Content) Message {
	return Message{
		Role:         RoleUser,
		Content:      text,
		ContentParts: parts,
	}
}
