package ws

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/daneel-ai/daneel"
)

// ─── WebSocket framing (RFC 6455) ────────────────────────────────────────────

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func acceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

const (
	opcodeText  byte = 0x1
	opcodeClose byte = 0x8
	opcodePing  byte = 0x9
	opcodePong  byte = 0xA
)

func readFrame(r io.Reader) (opcode byte, payload []byte, err error) {
	header := make([]byte, 2)
	if _, err = io.ReadFull(r, header); err != nil {
		return
	}
	opcode = header[0] & 0x0F
	masked := (header[1] & 0x80) != 0
	payloadLen := uint64(header[1] & 0x7F)

	switch payloadLen {
	case 126:
		ext := make([]byte, 2)
		if _, err = io.ReadFull(r, ext); err != nil {
			return
		}
		payloadLen = uint64(binary.BigEndian.Uint16(ext))
	case 127:
		ext := make([]byte, 8)
		if _, err = io.ReadFull(r, ext); err != nil {
			return
		}
		payloadLen = binary.BigEndian.Uint64(ext)
	}

	var maskKey [4]byte
	if masked {
		if _, err = io.ReadFull(r, maskKey[:]); err != nil {
			return
		}
	}

	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err = io.ReadFull(r, payload); err != nil {
			return
		}
		if masked {
			for i := range payload {
				payload[i] ^= maskKey[i%4]
			}
		}
	}
	return
}

func writeFrame(w io.Writer, opcode byte, payload []byte) error {
	n := len(payload)
	var buf []byte
	buf = append(buf, 0x80|opcode)
	switch {
	case n < 126:
		buf = append(buf, byte(n))
	case n < 65536:
		le := make([]byte, 2)
		binary.BigEndian.PutUint16(le, uint16(n))
		buf = append(buf, 126)
		buf = append(buf, le...)
	default:
		le := make([]byte, 8)
		binary.BigEndian.PutUint64(le, uint64(n))
		buf = append(buf, 127)
		buf = append(buf, le...)
	}
	buf = append(buf, payload...)
	_, err := w.Write(buf)
	return err
}

// ─── connection ───────────────────────────────────────────────────────────────

// connection manages a single WebSocket client connection.
type connection struct {
	conn      net.Conn
	sessionID string
	agent     *daneel.Agent
	server    *Server
	mu        sync.Mutex
}

func newConnection(conn net.Conn, sessionID string, agent *daneel.Agent, s *Server) *connection {
	return &connection{conn: conn, sessionID: sessionID, agent: agent, server: s}
}

func (c *connection) send(msg ServerMessage) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return writeFrame(c.conn, opcodeText, b)
}

func (c *connection) serve(ctx context.Context) {
	defer c.conn.Close()
	for {
		opcode, payload, err := readFrame(c.conn)
		if err != nil {
			return
		}
		switch opcode {
		case opcodeClose:
			// echo close frame and exit
			c.mu.Lock()
			_ = writeFrame(c.conn, opcodeClose, []byte{0x03, 0xE8})
			c.mu.Unlock()
			return
		case opcodePing:
			c.mu.Lock()
			_ = writeFrame(c.conn, opcodePong, payload)
			c.mu.Unlock()
		case opcodeText:
			var msg ClientMessage
			if err := json.Unmarshal(payload, &msg); err != nil {
				_ = c.send(ServerMessage{Type: MsgTypeError, Content: "invalid JSON"})
				continue
			}
			if msg.SessionID != "" {
				c.sessionID = msg.SessionID
			}
			go c.handleMessage(ctx, msg)
		}
	}
}

func (c *connection) handleMessage(ctx context.Context, msg ClientMessage) {
	opts := []daneel.RunOption{
		daneel.WithSessionID(c.sessionID),
		daneel.WithStreaming(func(chunk daneel.StreamChunk) {
			switch chunk.Type {
			case daneel.StreamText:
				_ = c.send(ServerMessage{Type: MsgTypeToken, Content: chunk.Text})
			case daneel.StreamToolCallStart:
				if chunk.ToolCall != nil {
					_ = c.send(ServerMessage{
						Type: MsgTypeToolCall,
						Tool: chunk.ToolCall.Name,
						Args: chunk.ToolCall.Arguments,
					})
				}
			case daneel.StreamToolCallDone:
				if chunk.ToolResult != nil {
					_ = c.send(ServerMessage{
						Type:    MsgTypeToolResult,
						Tool:    chunk.ToolResult.Name,
						Content: chunk.ToolResult.Content,
					})
				}
			}
		}),
	}

	result, err := daneel.Run(ctx, c.agent, msg.Content, opts...)
	if err != nil {
		_ = c.send(ServerMessage{Type: MsgTypeError, Content: err.Error()})
		return
	}
	_ = c.send(ServerMessage{
		Type:      MsgTypeDone,
		Content:   result.Output,
		SessionID: result.SessionID,
	})
}

// ─── upgrader helpers ─────────────────────────────────────────────────────────

func isWebSocketUpgrade(headers map[string][]string) bool {
	for _, v := range headers["Upgrade"] {
		if v == "websocket" {
			return true
		}
	}
	return false
}

func writeUpgradeResponse(conn net.Conn, accept string) error {
	_, err := fmt.Fprintf(conn,
		"HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n\r\n",
		accept)
	return err
}
