package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/Rafiki81/daneel"
)

// WSConnector implements daneel.Connector, making a WebSocket server available
// as a platform channel for use with the Bridge.
//
// Incoming WebSocket text frames are forwarded to Messages().
// Call Send(ctx, sessionID, content) to push a response back to a specific client.
type WSConnector struct {
	addr  string
	path  string
	msgs  chan daneel.IncomingMessage
	conns sync.Map // sessionID → *connWriter
	ln    net.Listener
}

type connWriter struct {
	conn net.Conn
	mu   sync.Mutex
}

func (cw *connWriter) send(data []byte) error {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	return writeFrame(cw.conn, opcodeText, data)
}

// NewConnector creates a WSConnector that listens on addr at the given path.
func NewConnector(addr, path string) *WSConnector {
	if path == "" {
		path = "/chat"
	}
	return &WSConnector{
		addr: addr,
		path: path,
		msgs: make(chan daneel.IncomingMessage, 64),
	}
}

// Start begins accepting WebSocket connections. Implements daneel.Connector.
func (c *WSConnector) Start(ctx context.Context) error {
	var err error
	c.ln, err = net.Listen("tcp", c.addr)
	if err != nil {
		return fmt.Errorf("ws connector: listen %q: %w", c.addr, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(c.path, func(w http.ResponseWriter, r *http.Request) {
		c.handleUpgrade(w, r)
	})
	srv := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	go func() { _ = srv.Serve(c.ln) }()
	return nil
}

func (c *WSConnector) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket required", http.StatusBadRequest)
		return
	}
	accept := acceptKey(r.Header.Get("Sec-WebSocket-Key"))

	hj, ok := w.(http.Hijacker)
	if !ok {
		return
	}
	rawConn, rw, err := hj.Hijack()
	if err != nil {
		return
	}
	if rw.Writer.Buffered() > 0 {
		_ = rw.Writer.Flush()
	}
	if err := writeUpgradeResponse(rawConn, accept); err != nil {
		rawConn.Close()
		return
	}

	sessionID := daneel.NewSessionID()
	cw := &connWriter{conn: rawConn}
	c.conns.Store(sessionID, cw)
	go c.serveConn(r.Context(), rawConn, sessionID, cw)
}

func (c *WSConnector) serveConn(ctx context.Context, conn net.Conn, sessionID string, cw *connWriter) {
	defer func() {
		conn.Close()
		c.conns.Delete(sessionID)
	}()
	for {
		opcode, payload, err := readFrame(conn)
		if err != nil {
			return
		}
		switch opcode {
		case opcodeClose:
			cw.mu.Lock()
			_ = writeFrame(conn, opcodeClose, []byte{0x03, 0xE8})
			cw.mu.Unlock()
			return
		case opcodePing:
			cw.mu.Lock()
			_ = writeFrame(conn, opcodePong, payload)
			cw.mu.Unlock()
		case opcodeText:
			select {
			case c.msgs <- daneel.IncomingMessage{
				Platform: "websocket",
				From:     sessionID,
				Content:  string(payload),
				Channel:  sessionID,
			}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// Send encodes content as a ServerMessage and writes it to the client identified by sessionID.
func (c *WSConnector) Send(_ context.Context, to string, content string) error {
	v, ok := c.conns.Load(to)
	if !ok {
		return fmt.Errorf("ws connector: session %q not connected", to)
	}
	b, err := json.Marshal(ServerMessage{Type: MsgTypeDone, Content: content})
	if err != nil {
		return err
	}
	return v.(*connWriter).send(b)
}

// Messages returns the channel of incoming WebSocket messages.
func (c *WSConnector) Messages() <-chan daneel.IncomingMessage { return c.msgs }

// Stop closes the listener.
func (c *WSConnector) Stop() error {
	if c.ln != nil {
		return c.ln.Close()
	}
	return nil
}
