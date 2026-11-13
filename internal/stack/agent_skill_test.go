package stack

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func fakeHome(t *testing.T, roots ...string) string {
	t.Helper()
	home := t.TempDir()
	for _, r := range roots {
		os.MkdirAll(filepath.Join(home, r), 0o755)
	}
	old := agentHome
	agentHome = func() (string, error) { return home, nil }
	t.Cleanup(func() { agentHome = old })
	return home
}

// The skill is written only under an agent directory that exists, once, and
// again when its content differs.
func TestSkillsAreWrittenWhereAgentsAre(t *testing.T) {
	home := fakeHome(t, ".claude")
	items := SkillItems()
	if len(items) != 1 || items[0].OK {
		t.Fatalf("items = %+v", items)
	}
	wrote, err := EnsureSkills()
	if err != nil || len(wrote) != 1 {
		t.Fatalf("wrote %v, %v", wrote, err)
	}
	path := filepath.Join(home, ".claude", "skills", "bay", "SKILL.md")
	if got, _ := os.ReadFile(path); !bytes.Equal(got, SkillContent()) {
		t.Fatal("the skill differs from what the binary renders")
	}
	if _, err := os.Stat(filepath.Join(home, ".agents")); err == nil {
		t.Fatal("wrote ~/.agents, which did not exist")
	}
	if wrote, _ := EnsureSkills(); len(wrote) != 0 || !SkillItems()[0].OK {
		t.Fatalf("a current skill was written again: %v", wrote)
	}
	os.WriteFile(path, []byte("old"), 0o644)
	if SkillItems()[0].OK {
		t.Fatal("an out-of-date skill is reported current")
	}
}

func TestNoAgentDirectoryMeansNoSkill(t *testing.T) {
	fakeHome(t)
	if items := SkillItems(); len(items) != 0 {
		t.Fatalf("items = %+v", items)
	}
}

// The MCP registration is current only when it runs this executable with mcp;
// a machine without claude has no such part.
func TestTheMCPRegistrationFollowsTheExecutable(t *testing.T) {
	home := fakeHome(t)
	oldClaude, oldSelf := claudeBinary, mcpExecutable
	t.Cleanup(func() { claudeBinary, mcpExecutable = oldClaude, oldSelf })
	claudeBinary = func() (string, bool) { return "", false }
	if items := MCPItems(); len(items) != 0 {
		t.Fatalf("without claude: %+v", items)
	}
	claudeBinary = func() (string, bool) { return "/x/claude", true }
	mcpExecutable = func() (string, error) { return "/opt/homebrew/bin/bay", nil }
	write := func(command string) {
		os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"mcpServers":{"bay":{"type":"stdio","command":"`+command+`","args":["mcp"],"env":{}}}}`), 0o644)
	}
	write("/opt/homebrew/bin/bay")
	if items := MCPItems(); len(items) != 1 || !items[0].OK {
		t.Fatalf("current registration: %+v", items)
	}
	write("bay")
	if MCPItems()[0].OK {
		t.Fatal("a registration of another executable is reported current")
	}
}
