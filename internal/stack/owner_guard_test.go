package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A command that does not own the machine setup has to stop before it changes
// anything, not after. Refusing halfway leaves containers removed, volumes
// created and the project registered, and the caller is told the command
// failed for a machine it has already altered.
//
// ownedElsewhere puts a state directory in the position of not being the owner.
func ownedElsewhere(t *testing.T) *Runtime {
	t.Helper()
	owner := t.TempDir()
	dir := t.TempDir()
	machineOwner = func() (string, bool) { return owner, true }
	t.Cleanup(func() { machineOwner = readMachineOwner })
	return &Runtime{Machine: &Machine{Dir: dir}, Addr: "127.0.0.1:5354"}
}

// A configuration naming one service, written where the runtime will read it.
func oneService(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yaml")
	body := "services:\n  web:\n    image: nginx\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestUpStopsBeforeChangingAnythingWhenItOwnsNothing(t *testing.T) {
	rt := ownedElsewhere(t)
	cfg := oneService(t)
	if _, err := rt.Up(cfg); err == nil {
		t.Fatal("up ran against a machine this state directory does not own")
	} else if !strings.Contains(err.Error(), "belongs to the state directory") {
		t.Fatalf("up failed for another reason: %v", err)
	}
	// Nothing of the project may be recorded: the command stopped before it
	// touched the machine or its own registry.
	if groups, err := rt.Machine.Groups(); err == nil && len(groups) != 0 {
		t.Errorf("up registered %d project(s) before stopping", len(groups))
	}
}

func TestDownStopsBeforeRemovingAnythingWhenItOwnsNothing(t *testing.T) {
	rt := ownedElsewhere(t)
	if _, err := rt.Down(oneService(t)); err == nil {
		t.Fatal("down ran against a machine this state directory does not own")
	} else if !strings.Contains(err.Error(), "belongs to the state directory") {
		t.Fatalf("down failed for another reason: %v", err)
	}
}

func TestServiceActionsStopWhenTheyOwnNothing(t *testing.T) {
	for name, run := range map[string]func(*Runtime, *Config) (SyncResult, error){
		"start":   func(r *Runtime, c *Config) (SyncResult, error) { return r.StartServices(c, nil) },
		"stop":    func(r *Runtime, c *Config) (SyncResult, error) { return r.StopServices(c, nil) },
		"restart": func(r *Runtime, c *Config) (SyncResult, error) { return r.RestartServices(c, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			rt := ownedElsewhere(t)
			if _, err := run(rt, oneService(t)); err == nil {
				t.Fatalf("%s ran against a machine this state directory does not own", name)
			} else if !strings.Contains(err.Error(), "belongs to the state directory") {
				t.Fatalf("%s failed for another reason: %v", name, err)
			}
		})
	}
}
