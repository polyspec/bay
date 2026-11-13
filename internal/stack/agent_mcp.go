package stack

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
)

// The MCP server is part of the machine setup where Claude Code is installed:
// it is registered in the user's Claude Code configuration as "bay", running
// this executable with "mcp". The registration is read from ~/.claude.json and
// changed only through the claude command, which owns that file, and only when
// it differs, so a moved executable is registered again.

// MCPServerName is the name the server is registered under.
const MCPServerName = "bay"

// claudeBinary finds the claude command. Tests replace it.
var claudeBinary = func() (string, bool) {
	p, err := exec.LookPath("claude")
	return p, err == nil
}

// ClaudeInstalled returns the claude command, if it is on PATH.
func ClaudeInstalled() (string, bool) { return claudeBinary() }

// mcpExecutable is the bay executable the registration runs. Tests replace it.
var mcpExecutable = os.Executable

// mcpRegistration is a server's entry in ~/.claude.json.
type mcpRegistration struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// registeredMCP returns the user-scope registration of the server, if any.
func registeredMCP() (mcpRegistration, bool) {
	home, err := agentHome()
	if err != nil {
		return mcpRegistration{}, false
	}
	b, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		return mcpRegistration{}, false
	}
	var cfg struct {
		MCPServers map[string]mcpRegistration `json:"mcpServers"`
	}
	if json.Unmarshal(b, &cfg) != nil {
		return mcpRegistration{}, false
	}
	r, ok := cfg.MCPServers[MCPServerName]
	return r, ok
}

// wantMCP is the registration this executable needs.
func wantMCP() (mcpRegistration, error) {
	self, err := mcpExecutable()
	if err != nil {
		return mcpRegistration{}, err
	}
	return mcpRegistration{Type: "stdio", Command: self, Args: []string{"mcp"}}, nil
}

// MCPItems is the registration as a setup part. A machine without the claude
// command has none.
func MCPItems() []SetupItem {
	if _, ok := claudeBinary(); !ok {
		return nil
	}
	want, err := wantMCP()
	if err != nil {
		return []SetupItem{{Name: "MCP server " + MCPServerName + ": " + err.Error()}}
	}
	got, ok := registeredMCP()
	return []SetupItem{{
		Name:  "MCP server " + MCPServerName + " in Claude Code (" + want.Command + " mcp)",
		OK:    ok && reflect.DeepEqual(got, want),
		Agent: true,
		SetupStep: SetupStep{Text: "register the MCP server: claude mcp add --scope user " +
			MCPServerName + " -- " + want.Command + " mcp"},
	}}
}

// EnsureMCP registers the server when its registration differs and reports
// whether it registered.
func EnsureMCP() (bool, error) {
	claude, ok := claudeBinary()
	if !ok {
		return false, nil
	}
	want, err := wantMCP()
	if err != nil {
		return false, err
	}
	got, registered := registeredMCP()
	if registered && reflect.DeepEqual(got, want) {
		return false, nil
	}
	if registered {
		if out, err := exec.Command(claude, "mcp", "remove", "--scope", "user", MCPServerName).CombinedOutput(); err != nil {
			return false, fmt.Errorf("claude mcp remove: %v: %s", err, out)
		}
	}
	args := append([]string{"mcp", "add", "--scope", "user", MCPServerName, "--", want.Command}, want.Args...)
	if out, err := exec.Command(claude, args...).CombinedOutput(); err != nil {
		return false, fmt.Errorf("claude mcp add: %v: %s", err, out)
	}
	return true, nil
}
