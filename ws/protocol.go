// Package ws provides a WebSocket server for real-time agent interactions.
// It implements both a stand-alone server (ws.NewServer) and a
// daneel.Connector (ws.NewConnector) for use with the Bridge.
package ws

import "encoding/json"

// MsgType identifies the kind of a WebSocket message.
type MsgType string

const (
	MsgTypeMessage    MsgType = "message"     // client sends a chat message
	MsgTypeToken      MsgType = "token"       // server streams a text token
	MsgTypeToolCall   MsgType = "tool_call"   // server notifies a tool invocation
	MsgTypeToolResult MsgType = "tool_result" // server notifies a tool result
	MsgTypeDone       MsgType = "done"        // server signals run complete
	MsgTypeError      MsgType = "error"       // server signals an error
	MsgTypePing       MsgType = "ping"
	MsgTypePong       MsgType = "pong"
)

// ClientMessage is a message sent from the browser/client to the server.
type ClientMessage struct {
	Type      MsgType `json:"type"`
	Content   string  `json:"content"`
	SessionID string  `json:"session_id,omitempty"`
}

// ServerMessage is a message sent from the server to the browser/client.
type ServerMessage struct {
	Type      MsgType         `json:"type"`
	Content   string          `json:"content,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	Args      json.RawMessage `json:"args,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
}
