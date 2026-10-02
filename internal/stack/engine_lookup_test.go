package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A program macOS starts outside a terminal receives this search path. The
// window launched from Finder found no engine on it while container answered
// from a terminal.
const launchedPath = "/usr/bin:/bin:/usr/sbin:/sbin"

func TestAnEngineInAnInstalledDirectoryIsFoundWithoutTheSearchPath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "container")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", launchedPath)
	t.Setenv("CONTAINER_BIN", "")
	old := installedDirs
	installedDirs = []string{filepath.Join(dir, "missing"), dir}
	t.Cleanup(func() { installedDirs = old })

	if got := engineBin(AppleEngine); got != bin {
		t.Errorf("engineBin = %q, want %q", got, bin)
	}
}

func TestTheVariableIsAskedBeforeAnyDirectory(t *testing.T) {
	t.Setenv("CONTAINER_BIN", "/named/container")
	if got := engineBin(AppleEngine); got != "/named/container" {
		t.Errorf("engineBin = %q, want the named program", got)
	}
}

// The agent's job and the lookup read one list, so the two cannot disagree on
// where an engine is installed.
func TestTheAgentsSearchPathHoldsEveryInstalledDirectory(t *testing.T) {
	path := agentSearchPath()
	for _, d := range installedDirs {
		if !strings.Contains(path, d) {
			t.Errorf("the agent's search path %q lacks %s", path, d)
		}
	}
}
