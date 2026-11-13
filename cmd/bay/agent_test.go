package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/polyspec/bay/internal/contract"
)

// stubSelf writes an executable that prints its working directory and
// arguments and exits with the status named by an argument "exit=N".
func stubSelf(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bay")
	script := `#!/bin/sh
echo "dir=$(pwd -P) args=$*"
for a in "$@"; do case "$a" in exit=*) exit "${a#exit=}";; esac; done
read line && echo "stdin=$line"
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func connect(t *testing.T, self string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := newMCPServer(self).Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// Every command that is not terminal-only is a tool, and its hints follow its
// declared access, so a client asks before a destructive one.
func TestMCPToolsFollowTheContract(t *testing.T) {
	cs := connect(t, stubSelf(t))
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]*mcp.Tool{}
	for _, tool := range res.Tools {
		tools[tool.Name] = tool
	}
	for _, c := range contract.Commands() {
		tool, ok := tools[c.Name]
		if c.Terminal {
			if ok {
				t.Errorf("%s is terminal-only but is a tool", c.Name)
			}
			continue
		}
		if !ok {
			t.Errorf("%s is not a tool", c.Name)
			continue
		}
		a := tool.Annotations
		if a.ReadOnlyHint != (c.Access == contract.Reads) {
			t.Errorf("%s readOnly = %v, access %s", c.Name, a.ReadOnlyHint, c.Access)
		}
		if *a.DestructiveHint != (c.Access == contract.Removes) {
			t.Errorf("%s destructive = %v, access %s", c.Name, *a.DestructiveHint, c.Access)
		}
		if c.Asks != "" && !strings.Contains(tool.Description, "ask the user") {
			t.Errorf("%s asks a person but its description does not say so", c.Name)
		}
	}
}

// A tool runs the command in the given directory with an empty standard input,
// and a non-zero exit is an error result carrying the status.
func TestMCPToolRunsTheCommand(t *testing.T) {
	cs := connect(t, stubSelf(t))
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "status", Arguments: map[string]any{"args": []string{"--json"}, "dir": dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if res.IsError || !strings.Contains(text, "dir="+dir+" args=status --json") || strings.Contains(text, "stdin=") {
		t.Fatalf("result = %v %q", res.IsError, text)
	}

	res, err = cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "check", Arguments: map[string]any{"args": []string{"exit=1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "(exit status 1)") {
		t.Fatalf("a failing command is not an error result: %+v", res)
	}

	res, err = cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "logs", Arguments: map[string]any{"args": []string{"-f", "web"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("logs -f was run; it never returns")
	}
}

// The resources are the commands' JSON output.
func TestMCPResourceIsTheCommandOutput(t *testing.T) {
	cs := connect(t, stubSelf(t))
	res, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "bay://brief"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Contents[0].Text, "args=brief --json") {
		t.Fatalf("brief resource = %q", res.Contents[0].Text)
	}
}

// The committed skill is what this binary renders.
func TestCommittedSkillIsCurrent(t *testing.T) {
	got, err := os.ReadFile("../../skills/bay/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	contract.Skill(&want)
	if !bytes.Equal(got, want.Bytes()) {
		t.Fatal("skills/bay/SKILL.md is not current; run \"make docs-generate\"")
	}
}
