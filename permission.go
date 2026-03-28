package daneel

import "strings"

// PermissionRule represents a single allow or deny directive.
type PermissionRule struct {
	kind    permKind   // allow or deny
	target  permTarget // tools or handoffs
	pattern string     // exact name or prefix with trailing "*"
}

type permKind int

const (
	permAllow permKind = iota
	permDeny
)

type permTarget int

const (
	permTools permTarget = iota
	permHandoffs
)

// AllowTools creates permission rules that whitelist the given tool names.
// Patterns ending with "*" match any tool whose name starts with the prefix.
//
//	daneel.AllowTools("read", "search", "mcp.github.*")
func AllowTools(patterns ...string) []PermissionRule {
	rules := make([]PermissionRule, len(patterns))
	for i, p := range patterns {
		rules[i] = PermissionRule{kind: permAllow, target: permTools, pattern: p}
	}
	return rules
}

// DenyTools creates permission rules that blacklist the given tool names.
//
//	daneel.DenyTools("exec", "delete")
func DenyTools(patterns ...string) []PermissionRule {
	rules := make([]PermissionRule, len(patterns))
	for i, p := range patterns {
		rules[i] = PermissionRule{kind: permDeny, target: permTools, pattern: p}
	}
	return rules
}

// AllowHandoffs creates permission rules that whitelist the given handoff targets.
//
//	daneel.AllowHandoffs("coder", "researcher")
func AllowHandoffs(patterns ...string) []PermissionRule {
	rules := make([]PermissionRule, len(patterns))
	for i, p := range patterns {
		rules[i] = PermissionRule{kind: permAllow, target: permHandoffs, pattern: p}
	}
	return rules
}

// permissionSet is the compiled set of permission rules, used by the Runner
// to check each tool call.
type permissionSet struct {
	allowTools    []string
	denyTools     []string
	allowHandoffs []string
}

func compilePermissions(rules []PermissionRule) permissionSet {
	var ps permissionSet
	for _, r := range rules {
		switch {
		case r.kind == permAllow && r.target == permTools:
			ps.allowTools = append(ps.allowTools, r.pattern)
		case r.kind == permDeny && r.target == permTools:
			ps.denyTools = append(ps.denyTools, r.pattern)
		case r.kind == permAllow && r.target == permHandoffs:
			ps.allowHandoffs = append(ps.allowHandoffs, r.pattern)
		}
	}
	return ps
}

// checkTool returns ("", true) if the tool is allowed, or (reason, false) if denied.
//
// Resolution order:
//  1. Check tool is NOT in DenyTools
//  2. If AllowTools defined, check tool IS in AllowTools
//  3. Otherwise, allowed by default
func (ps permissionSet) checkTool(name string) (string, bool) {
	for _, pattern := range ps.denyTools {
		if matchPattern(pattern, name) {
			return "tool in deny list", false
		}
	}
	if len(ps.allowTools) > 0 {
		for _, pattern := range ps.allowTools {
			if matchPattern(pattern, name) {
				return "", true
			}
		}
		return "tool not in allow list", false
	}
	return "", true
}

// checkHandoff returns ("", true) if the handoff target is allowed.
func (ps permissionSet) checkHandoff(name string) (string, bool) {
	if len(ps.allowHandoffs) == 0 {
		return "", true // no restriction
	}
	for _, pattern := range ps.allowHandoffs {
		if matchPattern(pattern, name) {
			return "", true
		}
	}
	return "handoff target not in allow list", false
}

// matchPattern checks if name matches pattern. Patterns ending with "*"
// use prefix matching; all others require an exact match.
func matchPattern(pattern, name string) bool {
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(name, prefix)
	}
	return pattern == name
}
