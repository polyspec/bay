package contract

import (
	"bytes"
	"strings"
	"testing"
)

// Every command declares its access, so no caller has to guess whether a
// command changes the machine, and a command that needs root says it asks.
func TestEveryCommandDeclaresItsAccess(t *testing.T) {
	for _, c := range Commands() {
		switch c.Access {
		case Reads, Changes, Removes:
		default:
			t.Errorf("%s has access %q", c.Name, c.Access)
		}
		if c.Root && c.Asks == "" {
			t.Errorf("%s can require root but does not say it asks", c.Name)
		}
	}
}

// The skill starts with the front matter agents read its name and description
// from, and lists every command.
func TestSkillNamesEveryCommand(t *testing.T) {
	var b bytes.Buffer
	Skill(&b)
	s := b.String()
	if !strings.HasPrefix(s, "---\nname: bay\ndescription: ") {
		t.Fatalf("front matter = %q", s[:60])
	}
	for _, c := range Commands() {
		if !strings.Contains(s, "`bay "+c.Name) {
			t.Errorf("the skill does not list %s", c.Name)
		}
	}
}
