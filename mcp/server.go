package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/Rafiki81/daneel"
)

// Server exposes daneel Tools as an MCP-compatible server.
// It supports both stdio and HTTP transports.
type Server struct {
	name    string
	version string
	tools   []daneel.Tool
	authFn  func(r *http.Request) bool
	mu      sync.RWMutex
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// NewServer creates a new MCP server that exposes the given tools.
func NewServer(name string, tools []daneel.Tool, opts ...ServerOption) *Server {
	s := &Server{
		name:    name,
		version: "1.0.0",
		tools:   tools,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// WithVersion sets the server version reported in capabilities.
func WithVersion(v string) ServerOption {
	return func(s *Server) {
		s.version = v
	}
}

// WithAuth sets an authentication function for HTTP transport.
func WithAuth(fn func(r *http.Request) bool) ServerOption {
	return func(s *Server) {
		s.authFn = fn
	}
}

// ListenStdio runs the MCP server over stdin/stdout (JSON-RPC over stdio).
// Blocks until ctx is cancelled or stdin is closed.
func (s *Server) ListenStdio(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			slog.Error("mcp server: invalid request", "error", err)
			continue
		}

		resp := s.handleRequest(ctx, req)
		data, _ := json.Marshal(resp)
		fmt.Fprintf(os.Stdout, "%s\n", data)
	}

	return scanner.Err()
}

// ListenHTTP runs the MCP server on the given address as an HTTP endpoint.
// Clients send JSON-RPC requests via POST.
func (s *Server) ListenHTTP(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.httpHandler)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		server.Close()
	}()

	err := server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Handler returns an http.Handler for embedding the MCP server
// in an existing HTTP server.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.httpHandler)
}

func (s *Server) httpHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.authFn != nil && !s.authFn(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	resp := s.handleRequest(r.Context(), req)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleRequest dispatches a JSON-RPC request to the appropriate handler.
func (s *Server) handleRequest(ctx context.Context, req jsonRPCRequest) jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	default:
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jsonRPCError{
				Code:    -32601,
				Message: fmt.Sprintf("method not found: %s", req.Method),
			},
		}
	}
}

func (s *Server) handleInitialize(req jsonRPCRequest) jsonRPCResponse {
	result := map[string]any{
		"protocolVersion": "2024-11-05",
		"serverInfo": map[string]string{
			"name":    s.name,
			"version": s.version,
		},
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
	}
	data, _ := json.Marshal(result)
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  json.RawMessage(data),
	}
}

func (s *Server) handleToolsList(req jsonRPCRequest) jsonRPCResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	type serverTool struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"inputSchema"`
	}

	tools := make([]serverTool, len(s.tools))
	for i, t := range s.tools {
		tools[i] = serverTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Schema,
		}
	}

	result := map[string]any{"tools": tools}
	data, _ := json.Marshal(result)
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  json.RawMessage(data),
	}
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) handleToolsCall(ctx context.Context, req jsonRPCRequest) jsonRPCResponse {
	// Parse params
	paramsData, err := json.Marshal(req.Params)
	if err != nil {
		return s.rpcError(req.ID, -32602, "invalid params")
	}

	var params toolCallParams
	if err := json.Unmarshal(paramsData, &params); err != nil {
		return s.rpcError(req.ID, -32602, "invalid params: "+err.Error())
	}

	// Find tool
	s.mu.RLock()
	var tool *daneel.Tool
	for i := range s.tools {
		if s.tools[i].Name == params.Name {
			tool = &s.tools[i]
			break
		}
	}
	s.mu.RUnlock()

	if tool == nil {
		return s.rpcError(req.ID, -32602, fmt.Sprintf("tool not found: %s", params.Name))
	}

	// Execute tool
	result, err := tool.Run(ctx, params.Arguments)
	if err != nil {
		errResult := mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: err.Error()}},
			IsError: true,
		}
		data, _ := json.Marshal(errResult)
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(data),
		}
	}

	okResult := mcpToolResult{
		Content: []mcpContent{{Type: "text", Text: result}},
	}
	data, _ := json.Marshal(okResult)
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  json.RawMessage(data),
	}
}

func (s *Server) rpcError(id int64, code int, msg string) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &jsonRPCError{Code: code, Message: msg},
	}
}

// AddTools dynamically adds more tools to the server.
func (s *Server) AddTools(tools ...daneel.Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools = append(s.tools, tools...)
}
