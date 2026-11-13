package stack

import (
	"os/exec"
	"testing"
)

// warmExecutable runs a program a test has just written once, with no
// arguments, and waits for it to end. macOS checks an executable the first time
// it runs, and while the system is checking many new executables that check has
// taken longer than the engine read bound (measured: 44 and 80 seconds for a
// two-line script, 0 seconds for its second run). A test that times its
// engine calls would otherwise measure that check instead of the code.
func warmExecutable(t *testing.T, path string) {
	t.Helper()
	cmd := exec.Command(path)
	cmd.Env = append(cmd.Environ(), "BAY_STUB_WARMUP=1")
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("running %s: %v", path, err)
		}
	}
}
