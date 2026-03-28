package daneel

import "sync"

// Registry is an introspection catalog that tracks registered agents,
// platforms, and tools. Designed for a future CLI layer.
//
//	reg := daneel.NewRegistry()
//	reg.RegisterAgent(supportAgent)
//	reg.RegisterPlatform("twitter", twitter.Tools(token))
//	reg.Agents()     // []AgentInfo
//	reg.Platforms()  // []PlatformInfo
//	reg.Tools()      // []ToolInfo
type Registry struct {
	mu        sync.RWMutex
	agents    []AgentInfo
	platforms []PlatformInfo
}

// AgentInfo describes a registered agent for introspection.
type AgentInfo struct {
	Name         string
	Instructions string
	Tools        []ToolInfo
	Handoffs     []string
	MaxTurns     int
}

// PlatformInfo describes a registered platform tool pack.
type PlatformInfo struct {
	Name  string
	Tools []ToolInfo
}

// ToolInfo describes a single tool for introspection.
type ToolInfo struct {
	Name        string
	Description string
	Schema      string // JSON Schema as string
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// RegisterAgent adds an agent to the registry.
func (r *Registry) RegisterAgent(agent *Agent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tools := make([]ToolInfo, len(agent.config.tools))
	for i, t := range agent.config.tools {
		tools[i] = ToolInfo{
			Name:        t.Name,
			Description: t.Description,
			Schema:      string(t.Schema),
		}
	}

	handoffs := make([]string, len(agent.config.handoffs))
	for i, h := range agent.config.handoffs {
		handoffs[i] = h.Name()
	}

	r.agents = append(r.agents, AgentInfo{
		Name:         agent.Name(),
		Instructions: agent.config.instructions,
		Tools:        tools,
		Handoffs:     handoffs,
		MaxTurns:     agent.config.maxTurns,
	})
}

// RegisterPlatform adds a platform tool pack to the registry.
func (r *Registry) RegisterPlatform(name string, tools []Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	infos := make([]ToolInfo, len(tools))
	for i, t := range tools {
		infos[i] = ToolInfo{
			Name:        t.Name,
			Description: t.Description,
			Schema:      string(t.Schema),
		}
	}

	r.platforms = append(r.platforms, PlatformInfo{
		Name:  name,
		Tools: infos,
	})
}

// Agents returns all registered agents.
func (r *Registry) Agents() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]AgentInfo(nil), r.agents...)
}

// Platforms returns all registered platforms.
func (r *Registry) Platforms() []PlatformInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]PlatformInfo(nil), r.platforms...)
}

// Tools returns all tools from all registered agents and platforms.
func (r *Registry) Tools() []ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool)
	var all []ToolInfo

	for _, a := range r.agents {
		for _, t := range a.Tools {
			if !seen[t.Name] {
				seen[t.Name] = true
				all = append(all, t)
			}
		}
	}
	for _, p := range r.platforms {
		for _, t := range p.Tools {
			if !seen[t.Name] {
				seen[t.Name] = true
				all = append(all, t)
			}
		}
	}

	return all
}

// FindAgent returns the AgentInfo for the given name, or nil.
func (r *Registry) FindAgent(name string) *AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, a := range r.agents {
		if a.Name == name {
			return &a
		}
	}
	return nil
}

// FindTool returns the ToolInfo for the given name, or nil.
func (r *Registry) FindTool(name string) *ToolInfo {
	for _, t := range r.Tools() {
		if t.Name == name {
			return &t
		}
	}
	return nil
}
