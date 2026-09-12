package stack

import (
	"path/filepath"
	"strings"
	"testing"
)

// The machine setup is owned by the state directory the registered agent runs
// with. These cover reading that owner and refusing a command from another one.

func TestTheOwnerIsTheStateDirectoryTheAgentRunsWith(t *testing.T) {
	got, registered := ownerFromArgs([]string{
		"/opt/homebrew/bin/bayd",
		"-domain", "test",
		"-addr", "127.0.0.1:5354",
		"-state", "/Users/someone/.bay",
	})
	if !registered {
		t.Fatal("a registered agent was read as no agent")
	}
	if got != "/Users/someone/.bay" {
		t.Errorf("owner is %q, want the agent's state directory", got)
	}
}

func TestNoAgentMeansNoOwner(t *testing.T) {
	if _, registered := ownerFromArgs(nil); registered {
		t.Error("no registered agent was read as an owner")
	}
}

// An agent registered without -state names no directory. It cannot be trusted
// to speak for one, so it owns nothing and the next install takes over.
func TestAnAgentWithNoStateArgumentOwnsNothing(t *testing.T) {
	if _, registered := ownerFromArgs([]string{
		"/opt/homebrew/bin/bayd", "-domain", "test", "-addr", "127.0.0.1:5354",
	}); registered {
		t.Error("an agent that names no state directory was read as an owner")
	}
}

func TestTheOwnerMayChangeTheMachineSetup(t *testing.T) {
	dir := t.TempDir()
	if err := ownershipHeldBy(dir, dir, true); err != nil {
		t.Errorf("the owner was refused: %v", err)
	}
}

func TestAnotherStateDirectoryMayNotChangeTheMachineSetup(t *testing.T) {
	owner, other := t.TempDir(), t.TempDir()
	err := ownershipHeldBy(other, owner, true)
	if err == nil {
		t.Fatal("a state directory that owns nothing was allowed to change the machine setup")
	}
	// The message has to name the owner, because the reader's next step is to
	// run the command from there or to take ownership deliberately.
	if !strings.Contains(err.Error(), owner) {
		t.Errorf("the refusal does not name the owner: %v", err)
	}
}

func TestAnUnownedMachineIsTakenByTheCaller(t *testing.T) {
	if err := ownershipHeldBy(t.TempDir(), "", false); err != nil {
		t.Errorf("a machine with no owner refused the caller: %v", err)
	}
}

// A path that reaches the same directory through a symbolic link or a trailing
// separator is the same owner. The argument is written by hand often enough
// that comparing the text alone would refuse the owner.
func TestTheSameDirectoryWrittenTwoWaysIsOneOwner(t *testing.T) {
	dir := t.TempDir()
	if err := ownershipHeldBy(dir+string(filepath.Separator), dir, true); err != nil {
		t.Errorf("the same directory with a trailing separator was refused: %v", err)
	}
}
