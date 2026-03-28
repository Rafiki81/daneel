package daneel

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/daneel-ai/daneel/content"
)

// ToolDef is the subset of Tool sent to the LLM provider.
// It contains only what the model needs to understand the tool.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"parameters"` // JSON Schema for parameters
}

// Tool is a concrete struct representing an executable tool.
// It is immutable after creation. Use NewTool, NewToolWithContent,
// or NewToolTyped to create instances.
type Tool struct {
	Name            string          // unique identifier
	Description     string          // description for the LLM
	Schema          json.RawMessage // auto-generated JSON Schema
	fn              func(ctx context.Context, args json.RawMessage) (string, error)
	timeout         time.Duration // per-tool timeout (0 = use agent default)
	returnsContent  bool          // true for NewToolWithContent
	requireApproval bool          // true if human approval is required
}

// Run executes the tool with raw JSON arguments. Called by the Runner.
func (t Tool) Run(ctx context.Context, args json.RawMessage) (string, error) {
	if t.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}

	type toolResult struct {
		value string
		err   error
	}
	done := make(chan toolResult, 1)

	go func() {
		v, e := t.fn(ctx, args)
		done <- toolResult{value: v, err: e}
	}()

	select {
	case r := <-done:
		return r.value, r.err
	case <-ctx.Done():
		return "", &ToolTimeoutError{Tool: t.Name, Timeout: t.timeout}
	}
}

// Def returns the ToolDef sent to the provider (name, description, schema only).
func (t Tool) Def() ToolDef {
	return ToolDef{
		Name:        t.Name,
		Description: t.Description,
		Schema:      t.Schema,
	}
}

// ToolOption configures optional Tool behavior.
type ToolOption func(*Tool)

// WithToolTimeout sets a per-tool execution timeout.
func WithToolTimeout(d time.Duration) ToolOption {
	return func(t *Tool) {
		t.timeout = d
	}
}

// WithApprovalRequired marks the tool as requiring human approval before execution.
func WithApprovalRequired() ToolOption {
	return func(t *Tool) {
		t.requireApproval = true
	}
}

// NewTool creates a Tool that returns a text string. The parameter struct T
// is reflected once at creation time to generate a JSON Schema. The handler
// receives a decoded T on each call.
//
//	searchTool := daneel.NewTool("search", "Search the knowledge base",
//	    func(ctx context.Context, p SearchParams) (string, error) {
//	        return kb.Search(p.Query, p.MaxResults)
//	    },
//	)
func NewTool[T any](name, description string, fn func(ctx context.Context, params T) (string, error), opts ...ToolOption) Tool {
	schema := generateSchema[T]()

	t := Tool{
		Name:        name,
		Description: description,
		Schema:      schema,
		fn: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params T
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("tool %q: invalid arguments: %w", name, err)
			}
			return fn(ctx, params)
		},
	}

	for _, opt := range opts {
		opt(&t)
	}
	return t
}

// NewToolWithContent creates a Tool that returns multi-modal content (image,
// audio, etc.). The content is serialized for the LLM automatically.
func NewToolWithContent[T any](name, description string, fn func(ctx context.Context, params T) (content.Content, error), opts ...ToolOption) Tool {
	schema := generateSchema[T]()

	t := Tool{
		Name:           name,
		Description:    description,
		Schema:         schema,
		returnsContent: true,
		fn: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params T
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("tool %q: invalid arguments: %w", name, err)
			}
			c, err := fn(ctx, params)
			if err != nil {
				return "", err
			}
			// Serialize content to JSON for the LLM response
			b, err := json.Marshal(c)
			if err != nil {
				return "", fmt.Errorf("tool %q: failed to marshal content: %w", name, err)
			}
			return string(b), nil
		},
	}

	for _, opt := range opts {
		opt(&t)
	}
	return t
}

// NewToolTyped creates a Tool that returns a typed struct. The output is
// auto-serialized to JSON for the LLM.
func NewToolTyped[In, Out any](name, description string, fn func(ctx context.Context, params In) (Out, error), opts ...ToolOption) Tool {
	schema := generateSchema[In]()

	t := Tool{
		Name:        name,
		Description: description,
		Schema:      schema,
		fn: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params In
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("tool %q: invalid arguments: %w", name, err)
			}
			out, err := fn(ctx, params)
			if err != nil {
				return "", err
			}
			b, err := json.Marshal(out)
			if err != nil {
				return "", fmt.Errorf("tool %q: failed to marshal output: %w", name, err)
			}
			return string(b), nil
		},
	}

	for _, opt := range opts {
		opt(&t)
	}
	return t
}

// NewToolRaw creates a Tool from a raw JSON schema and a raw function.
// This is used by the MCP client to wrap external tools without knowing
// the Go parameter struct at compile time.
func NewToolRaw(name, description string, schema json.RawMessage, fn func(ctx context.Context, args string) (string, error), opts ...ToolOption) Tool {
	t := Tool{
		Name:        name,
		Description: description,
		Schema:      schema,
		fn: func(ctx context.Context, args json.RawMessage) (string, error) {
			return fn(ctx, string(args))
		},
	}
	for _, opt := range opts {
		opt(&t)
	}
	return t
}

// MergeTools concatenates multiple []Tool slices into one.
//
//	daneel.WithTools(daneel.MergeTools(
//	    twitter.Tools(token),
//	    slack.Tools(token),
//	)...)
func MergeTools(toolSets ...[]Tool) []Tool {
	var total int
	for _, ts := range toolSets {
		total += len(ts)
	}
	merged := make([]Tool, 0, total)
	for _, ts := range toolSets {
		merged = append(merged, ts...)
	}
	return merged
}

// ---------- JSON Schema generation via reflection ----------

// generateSchema generates a JSON Schema from struct type T.
// Supports struct tags: json, desc, required, enum, default.
// Reflection runs once at tool creation time, not per call.
func generateSchema[T any]() json.RawMessage {
	var zero T
	t := reflect.TypeOf(zero)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	schema := typeToSchema(t)
	b, _ := json.Marshal(schema)
	return b
}

type jsonSchema struct {
	Type                 string                 `json:"type"`
	Properties           map[string]*jsonSchema `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	Description          string                 `json:"description,omitempty"`
	Enum                 []string               `json:"enum,omitempty"`
	Default              any                    `json:"default,omitempty"`
	Items                *jsonSchema            `json:"items,omitempty"`
	AdditionalProperties *jsonSchema            `json:"additionalProperties,omitempty"`
}

func typeToSchema(t reflect.Type) *jsonSchema {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Struct:
		return structToSchema(t)
	case reflect.Slice:
		return &jsonSchema{
			Type:  "array",
			Items: typeToSchema(t.Elem()),
		}
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return &jsonSchema{Type: "object"}
		}
		return &jsonSchema{
			Type:                 "object",
			AdditionalProperties: typeToSchema(t.Elem()),
		}
	case reflect.String:
		return &jsonSchema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &jsonSchema{Type: "integer"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &jsonSchema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &jsonSchema{Type: "number"}
	case reflect.Bool:
		return &jsonSchema{Type: "boolean"}
	default:
		return &jsonSchema{Type: "object"}
	}
}

func structToSchema(t reflect.Type) *jsonSchema {
	s := &jsonSchema{
		Type:       "object",
		Properties: make(map[string]*jsonSchema),
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}
		name := jsonTag
		if idx := strings.Index(name, ","); idx != -1 {
			name = name[:idx]
		}
		if name == "" {
			name = field.Name
		}

		prop := typeToSchema(field.Type)

		// description or desc tag → description
		if desc := field.Tag.Get("description"); desc != "" {
			prop.Description = desc
		} else if desc := field.Tag.Get("desc"); desc != "" {
			prop.Description = desc
		}

		// enum tag → enum values
		if enum := field.Tag.Get("enum"); enum != "" {
			prop.Enum = strings.Split(enum, ",")
		}

		// default tag → default value
		if def := field.Tag.Get("default"); def != "" {
			prop.Default = def
		}

		s.Properties[name] = prop

		// required logic: non-pointer fields are required by default,
		// unless required:"false" is set explicitly
		isPointer := field.Type.Kind() == reflect.Ptr
		reqTag := field.Tag.Get("required")
		if reqTag == "true" || (reqTag == "" && !isPointer) {
			s.Required = append(s.Required, name)
		}
	}

	return s
}
