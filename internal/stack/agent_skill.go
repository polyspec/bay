package stack

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/polyspec/bay/internal/contract"
)

// The agent skill is part of the machine setup: an agent on this machine
// learns the tool from it without being told. It is written where agents read
// the user's skills, ~/.claude/skills for Claude Code and ~/.agents/skills for
// the agents that read the shared directory, and only under a directory that
// exists, because one that does not belongs to an agent this user does not run.
// It is rewritten when its content differs from what this binary renders, so
// a new version of the tool brings its skill along.

// agentHome is the home directory skills are written under. Tests replace it.
var agentHome = os.UserHomeDir

// agentRoots are the directories, relative to the home directory, of the
// agents whose skill directories are written.
var agentRoots = []string{".claude", ".agents"}

// SkillContent is the skill this binary renders.
func SkillContent() []byte {
	var b bytes.Buffer
	contract.Skill(&b)
	return b.Bytes()
}

// SkillPaths returns the SKILL.md of each agent directory that exists.
func SkillPaths() []string {
	home, err := agentHome()
	if err != nil {
		return nil
	}
	var paths []string
	for _, root := range agentRoots {
		if info, err := os.Stat(filepath.Join(home, root)); err == nil && info.IsDir() {
			paths = append(paths, filepath.Join(home, root, "skills", contract.SkillName, "SKILL.md"))
		}
	}
	return paths
}

// SkillItems are the skills as setup parts. A machine with no agent directory
// has none.
func SkillItems() []SetupItem {
	want := SkillContent()
	var items []SetupItem
	for _, p := range SkillPaths() {
		got, err := os.ReadFile(p)
		items = append(items, SetupItem{
			Name: "agent skill " + p, OK: err == nil && bytes.Equal(got, want),
			SetupStep: SetupStep{Text: "write the agent skill " + p}, Agent: true,
		})
	}
	return items
}

// EnsureSkills writes each skill that differs from what this binary renders and
// returns the paths it wrote.
func EnsureSkills() ([]string, error) {
	want := SkillContent()
	var wrote []string
	for _, p := range SkillPaths() {
		if got, err := os.ReadFile(p); err == nil && bytes.Equal(got, want) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return wrote, err
		}
		tmp := p + ".bay"
		if err := os.WriteFile(tmp, want, 0o644); err != nil {
			return wrote, err
		}
		if err := os.Rename(tmp, p); err != nil {
			os.Remove(tmp)
			return wrote, err
		}
		wrote = append(wrote, p)
	}
	return wrote, nil
}
