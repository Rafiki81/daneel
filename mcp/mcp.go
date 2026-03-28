package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/daneel-ai/daneel"
)

// ServerSpec describes how to connect to an MCP server.
type ServerSpec struct {
	Transport string   // "stdio" or "http"
	Command   string   // for stdio: command to run
	Args      []string // for stdio: command args
	URL       string   // for http: endpoint URL
}

// Stdio creates a spec for a stdio MCP server.
func Stdio(command string, args ...string) ServerSpec {
	return ServerSpec{Transport: "stdio", Command: command, Args: args}
}

// HTTP creates a spec for an HTTP MCP server.
func HTTP(url string) ServerSpec {
	return ServerSpec{Transport: "http", URL: url}
}

// Connect starts a stdio MCP server and returns its tools as daneel.Tool.
func Connect(ctx context.Context, command string, args ...string) ([]daneel.Tool, error) {
	c, err := newStdioClient(ctx, command, args...)
	if err != nil {
		return nil, err
	}
	if err := c.initialize(ctx); err != nil {
		c.Close()
		return nil, err
	}
	return c.listTools(ctx)
}

// ConnectHTTP connects to an HTTP MCP server and returns its tools.
func ConnectHTTP(ctx context.Context, url string) ([]daneel.Tool, error) {
	c := newHTTPClient(url)
	if err := c.initialize(ctx); err != nil {
		return nil, err
	}
	return c.listTools(ctx)
}

// ConnectAll connects to multiple MCP servers and merges their tools.
func ConnectAll(ctx context.Context, specs ...ServerSpec) ([]daneel.Tool, error) {
	var all []daneel.Tool
	for _, s := range specs {
		var tools []daneel.Tool
		var err error
		switch s.Transport {
		case "stdio":
			tools, err = Connect(ctx, s.Command, s.Args...)
		case "http":
			tools, err = ConnectHTTP(ctx, s.URL)
		default:
			return nil, fmt.Errorf("mcp: unknown transport %q", s.Transport)
		}
		if err != nil {
			return nil, fmt.Errorf("mcp: connect %s: %w", s.Command+s.URL, err)
		}
		all = append(all, tools...)
	}
	return all, nil
}

// --- JSON-RPC 2.0 ---

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("mcp: rpc error %d: %s", e.Code, e.Message)
}

// mcpTool is the MCP protocol tool definition.
type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema mcpInputSchema `json:"inputSchema"`
}

type mcpInputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]mcpProperty `json:"properties"`
	Required   []string               `json:"required"`
}

type mcpProperty struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// --- transport interface ---

type transport interface {
	send(ctx context.Context, req jsonRPCRequest) (jsonRPCResponse, error)
	Close() error
}

// --- mcpClient ---

type mcpClient struct {
	tr transport
	id atomic.Int64
}

func (c *mcpClient) nextID() int64 {
	return c.id.Add(1)
}

func (c *mcpClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  method,
		Params:  params,
	}
	resp, err := c.tr.send(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp.Result, nil
}

func (c *mcpClient) Close() error {
	return c.tr.Close()
}

func (c *mcpClient) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    "daneel",
			"version": "0.5.0",
		},
	}
	_, err := c.call(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("mcp: initialize: %w", err)
	}
	// Send initialized notification
	notif := jsonRPCRequest{JSONRPC: "2.0", Method: "notifications/initialized"}
	c.tr.send(ctx, notif)
	return nil
}

func (c *mcpClient) listTools(ctx context.Context) ([]daneel.Tool, error) {
	raw, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: tools/list: %w", err)
	}
	var result struct {
		Tools []mcpTool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("mcp: unmarshal tools: %w", err)
	}
	tools := make([]daneel.Tool, 0, len(result.Tools))
	for _, mt := range result.Tools {
		tools = append(tools, c.convertTool(mt))
	}
	return tools, nil
}

func (c *mcpClient) convertTool(mt mcpTool) daneel.Tool {
	name := "mcp." + mt.Name
	schemaBytes := buildSchemaJSON(mt.InputSchema)
	toolName := mt.Name // capture for closure
	return daneel.NewToolRaw(name, mt.Description, schemaBytes,
		func(ctx context.Context, input string) (string, error) {
			var args map[string]any
			if input != "" {
				if err := json.Unmarshal([]byte(input), &args); err != nil {
					return "", fmt.Errorf("mcp: invalid input: %w", err)
				}
			}
			params := map[string]any{"name": toolName, "arguments": args}
			raw, err := c.call(ctx, "tools/call", params)
			if err != nil {
				return "", err
			}
			var res mcpToolResult
			if err := json.Unmarshal(raw, &res); err != nil {
				return string(raw), nil
			}
			if res.IsError && len(res.Content) > 0 {
				return "", fmt.Errorf("mcp tool error: %s", res.Content[0].Text)
			}
			var texts []string
			for _, ct := range res.Content {
				if ct.Type == "text" {
					texts = append(texts, ct.Text)
				}
			}
			return strings.Join(texts, "\n"), nil
		},
	)
}

func buildSchemaJSON(is mcpInputSchema) json.RawMessage {
	schema := map[string]any{"type": "object"}
	if len(is.Properties) > 0 {
		props := make(map[string]any, len(is.Properties))
		for k, v := range is.Properties {
			p := map[string]any{"type": v.Type}
			if v.Description != "" {
				p["description"] = v.Description
			}
			props[k] = p
		}
		schema["properties"] = props
	}
	if len(is.Required) > 0 {
		schema["required"] = is.Required
	}
	b, _ := json.Marshal(schema)
	return json.RawMessage(b)
}

// --- Stdio transport ---

type stdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	mu     sync.Mutex
}

func newStdioClient(ctx context.Context, command string, args ...string) (*mcpClient, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: start %s: %w", command, err)
	}
	tr := &stdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReader(stdout),
	}
	return &mcpClient{tr: tr}, nil
}

func (t *stdioTransport) send(ctx context.Context, req jsonRPCRequest) (jsonRPCResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	b, err := json.Marshal(req)
	if err != nil {
		return jsonRPCResponse{}, fmt.Errorf("mcp: marshal: %w", err)
	}
	b = append(b, '\n')
	if _, err := t.stdin.Write(b); err != nil {
		return jsonRPCResponse{}, fmt.Errorf("mcp: write: %w", err)
	}
	// For notifications (no ID), skip reading response
	if req.ID == 0 {
		return jsonRPCResponse{}, nil
	}
	for {
		line, err := t.reader.ReadBytes('\n')
		if err != nil {
			return jsonRPCResponse{}, fmt.Errorf("mcp: read: %w", err)
		}
		line = trimLine(line)
		if len(line) == 0 {
			continue
		}
		var resp jsonRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue // skip non-JSON lines
		}
		if resp.ID == req.ID {
			return resp, nil
		}
	}
}

func (t *stdioTransport) Close() error {
	t.stdin.Close()
	return t.cmd.Wait()
}

// --- HTTP transport ---

type httpTransport struct {
	url  string
	http *http.Client
	mu   sync.Mutex
}

func newHTTPClient(url string) *mcpClient {
	tr := &httpTransport{url: url, http: http.DefaultClient}
	return &mcpClient{tr: tr}
}

func (t *httpTransport) send(ctx context.Context, req jsonRPCRequest) (jsonRPCResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	b, err := json.Marshal(req)
	if err != nil {
		return jsonRPCResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.url, byteRead(b))
	if err != nil {
		return jsonRPCResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := t.http.Do(httpReq)
	if err != nil {
		return jsonRPCResponse{}, fmt.Errorf("mcp: http: %w", err)
	}
	defer resp.Body.Close()
	if req.ID == 0 {
		return jsonRPCResponse{}, nil
	}
	var rpcResp jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return jsonRPCResponse{}, fmt.Errorf("mcp: decode: %w", err)
	}
	return rpcResp, nil
}

func (t *httpTransport) Close() error { return nil }

// --- helpers ---

func byteRead(b []byte) io.Reader {
	return bytes.NewReader(b)
}

func trimLine(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
