package ws

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/Rafiki81/daneel"
)

// Server is a WebSocket server that connects clients directly to a daneel Agent.
type Server struct {
	agent        *daneel.Agent
	path         string
	authFn       func(*http.Request) bool
	onConnect    func(sessionID string)
	onDisconnect func(sessionID string)
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithPath sets the URL path the server listens on (default: "/chat").
func WithPath(path string) ServerOption {
	return func(s *Server) { s.path = path }
}

// WithAuth sets an authentication function called for each new connection.
// Return false to reject the connection with 401.
func WithAuth(fn func(*http.Request) bool) ServerOption {
	return func(s *Server) { s.authFn = fn }
}

// WithOnConnect registers a callback invoked when a new client connects.
func WithOnConnect(fn func(sessionID string)) ServerOption {
	return func(s *Server) { s.onConnect = fn }
}

// WithOnDisconnect registers a callback invoked when a client disconnects.
func WithOnDisconnect(fn func(sessionID string)) ServerOption {
	return func(s *Server) { s.onDisconnect = fn }
}

// NewServer creates a WebSocket server for the given agent.
func NewServer(agent *daneel.Agent, opts ...ServerOption) *Server {
	s := &Server{agent: agent, path: "/chat"}
	for _, o := range opts {
		o(s)
	}
	return s
}

// ServeHTTP implements http.Handler. Mount on an existing mux with
// mux.Handle("/chat", server).
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return
	}
	if s.authFn != nil && !s.authFn(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	accept := acceptKey(key)

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	rawConn, rw, err := hj.Hijack()
	if err != nil {
		http.Error(w, "hijack failed", http.StatusInternalServerError)
		return
	}

	// Flush any pending buffered data, then send the upgrade response.
	if rw.Writer.Buffered() > 0 {
		_ = rw.Writer.Flush()
	}
	if err := writeUpgradeResponse(rawConn, accept); err != nil {
		rawConn.Close()
		return
	}

	sessionID := daneel.NewSessionID()
	conn := newConnection(rawConn, sessionID, s.agent, s)

	if s.onConnect != nil {
		s.onConnect(sessionID)
	}

	go func() {
		conn.serve(r.Context())
		if s.onDisconnect != nil {
			s.onDisconnect(sessionID)
		}
	}()
}

// Handler returns the server as an http.Handler for mounting on a mux.
func (s *Server) Handler() http.Handler { return s }

// ListenAndServe starts a standalone HTTP server on addr that serves WebSocket
// connections. Blocks until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	mux := http.NewServeMux()
	mux.Handle(s.path, s)
	httpServer := &http.Server{Handler: mux}

	go func() {
		<-ctx.Done()
		_ = httpServer.Shutdown(context.Background())
	}()

	return httpServer.Serve(bufioListener{ln})
}

// bufioListener wraps a net.Listener with bufio so we can supply rw to conn.
// For standalone mode we use http.Server directly without needing bufio.
type bufioListener struct{ net.Listener }

func (l bufioListener) Accept() (net.Conn, error) {
	return l.Listener.Accept()
}
