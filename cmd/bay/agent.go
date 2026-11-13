package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/polyspec/bay/internal/contract"
	"github.com/polyspec/bay/internal/stack"
)

// skill prints the agent skill or writes it where agents read skills.
func skill(args []string) error {
	action := "print"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "print":
		contract.Skill(os.Stdout)
		return nil
	case "install":
		paths := stack.SkillPaths()
		if len(paths) == 0 {
			return fmt.Errorf("no agent directory: neither ~/.claude nor ~/.agents exists; \"bay skill print\" writes the skill to standard output")
		}
		wrote, err := stack.EnsureSkills()
		if err != nil {
			return err
		}
		for _, p := range paths {
			state := "current"
			for _, w := range wrote {
				if w == p {
					state = "written"
				}
			}
			fmt.Printf("%-8s %s\n", state, p)
		}
		return nil
	}
	return usageError{fmt.Sprintf("unknown skill action %q; use print or install", action)}
}

// toolInput is what every tool takes: the arguments after the command name and
// the project directory the command runs in.
type toolInput struct {
	Args []string `json:"args,omitempty" jsonschema:"arguments after the command name, as on the command line"`
	Dir  string   `json:"dir,omitempty" jsonschema:"absolute path of the directory holding the project's Compose file; project commands run there"`
}

// toolOutput is a command's result.
type toolOutput struct {
	ExitStatus int    `json:"exitStatus" jsonschema:"the command's exit status; see brief for what each means"`
	Output     string `json:"output" jsonschema:"what the command printed, standard output then standard error"`
}

// mcpCommand serves the tools, or with "install" registers the server in
// Claude Code.
func mcpCommand(args []string) error {
	if len(args) == 0 {
		return serveMCP()
	}
	if args[0] != "install" {
		return usageError{fmt.Sprintf("unknown mcp action %q; use install, or no argument to serve", args[0])}
	}
	if _, ok := stack.ClaudeInstalled(); !ok {
		return fmt.Errorf("the claude command is not on PATH; register the server with your agent as: bay mcp")
	}
	registered, err := stack.EnsureMCP()
	if err != nil {
		return err
	}
	state := "current"
	if registered {
		state = "registered"
	}
	for _, it := range stack.MCPItems() {
		fmt.Printf("%-10s %s\n", state, it.Name)
	}
	return nil
}

// serveMCP serves every command that is not terminal-only as a tool, and the
// contract and the current state as resources, on standard input and output.
func serveMCP() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	// A client starts the server for each session, so the skill follows the
	// installed binary from the next session after an update.
	if _, err := stack.EnsureSkills(); err != nil {
		fmt.Fprintf(os.Stderr, "bay mcp: updating the agent skill: %v\n", err)
	}
	return newMCPServer(self).Run(context.Background(), &mcp.StdioTransport{})
}

// newMCPServer returns the server whose tools run self.
func newMCPServer(self string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "bay", Version: fmt.Sprintf("schema-%d", contract.SchemaVersion)},
		&mcp.ServerOptions{Instructions: contract.MCPInstructions()})
	for _, c := range contract.Commands() {
		if c.Terminal {
			continue
		}
		mcp.AddTool(server, toolFor(c), runner(self, c))
	}
	resources := []struct {
		name, uri, about string
		args             []string
	}{
		{"brief", "bay://brief", "The usage contract as JSON: commands, access, exit status, keys, rules.", []string{"brief", "--json"}},
		{"schema", "bay://schema", "The Compose file contract as JSON.", []string{"schema", "--json"}},
		{"status", "bay://status", "The current proxies, DNS server and projects as JSON.", []string{"status", "--json"}},
	}
	for _, r := range resources {
		r := r
		server.AddResource(&mcp.Resource{Name: r.name, URI: r.uri, Description: r.about, MIMEType: "application/json"},
			func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				out, status, err := runSelf(ctx, self, "", r.args)
				if err != nil {
					return nil, err
				}
				if status != 0 {
					return nil, fmt.Errorf("bay %s exited %d: %s", strings.Join(r.args, " "), status, out)
				}
				return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{
					{URI: r.uri, MIMEType: "application/json", Text: out},
				}}, nil
			})
	}
	return server
}

// toolFor describes a command as a tool. Its hints follow its access.
func toolFor(c contract.Command) *mcp.Tool {
	about := c.Summary
	if c.Effect != "" {
		about += " " + c.Effect
	}
	if c.Args != "" {
		about += " Arguments: " + c.Args + "."
	}
	if c.Asks != "" {
		about += " Asks a person for " + c.Asks + "; with no one to answer here, ask the user to run it in a terminal."
	}
	destructive := c.Access == contract.Removes
	open := c.Name == "peer"
	return &mcp.Tool{
		Name:        c.Name,
		Title:       "bay " + c.Name,
		Description: about,
		Annotations: &mcp.ToolAnnotations{
			Title:           "bay " + c.Name,
			ReadOnlyHint:    c.Access == contract.Reads,
			DestructiveHint: &destructive,
			IdempotentHint:  c.Access != contract.Removes || c.Name == "down" || c.Name == "uninstall",
			OpenWorldHint:   &open,
		},
	}
}

// runner runs one command as this executable, so a tool does exactly what the
// command line does.
func runner(self string, c contract.Command) mcp.ToolHandlerFor[toolInput, toolOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in toolInput) (*mcp.CallToolResult, toolOutput, error) {
		if c.Name == "logs" {
			for _, a := range in.Args {
				if a == "-f" || a == "--follow" || a == "-follow" {
					return nil, toolOutput{}, fmt.Errorf("logs -f does not end, so a tool cannot return it; use -n N")
				}
			}
		}
		if in.Dir != "" && !filepath.IsAbs(in.Dir) {
			return nil, toolOutput{}, fmt.Errorf("dir must be an absolute path, got %q", in.Dir)
		}
		out, status, err := runSelf(ctx, self, in.Dir, append([]string{c.Name}, in.Args...))
		if err != nil {
			return nil, toolOutput{}, err
		}
		res := toolOutput{ExitStatus: status, Output: out}
		text := out
		if status != 0 {
			text = fmt.Sprintf("%s\n(exit status %d)", strings.TrimRight(out, "\n"), status)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
			IsError: status != 0,
		}, res, nil
	}
}

// runSelf runs this executable with the global flags this process was given
// and args, in dir. Standard input is empty, so a question is answered no.
func runSelf(ctx context.Context, self, dir string, args []string) (string, int, error) {
	cmd := exec.CommandContext(ctx, self, append(globalFlags(), args...)...)
	cmd.Dir = dir
	cmd.Stdin = nil
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return out.String(), exit.ExitCode(), nil
	}
	if err != nil {
		return "", 0, err
	}
	return out.String(), 0, nil
}

// globalFlags are the flags before the command that this process was started
// with, so the server's commands use the same state directory and address.
func globalFlags() []string {
	var flags []string
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "state", "addr", "dnsbin":
			flags = append(flags, "-"+f.Name, f.Value.String())
		}
	})
	return flags
}

// usageError is a command line that names no command or an unknown one. It
// exits with status 2.
type usageError struct{ text string }

func (e usageError) Error() string { return e.text }
