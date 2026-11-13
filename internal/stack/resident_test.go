package stack

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseLaunchdPID(t *testing.T) {
	out := "system/dev.bay.relay = {\n\tactive count = 1\n\tpath = /Library/LaunchDaemons/dev.bay.relay.plist\n\tstate = running\n\n\tprogram = /opt/homebrew/bin/bayd\n\tpid = 36487\n\timmediate reason = speculative\n}\n"
	if got := parseLaunchdPID(out); got != 36487 {
		t.Fatalf("pid = %d, want 36487", got)
	}
	if got := parseLaunchdPID("system/dev.bay.relay = {\n\tstate = not running\n}\n"); got != 0 {
		t.Fatalf("pid of a stopped job = %d, want 0", got)
	}
}

func TestResidentStale(t *testing.T) {
	changed := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	restore := func(s func(int) (time.Time, error), f func(string) (time.Time, error)) {
		processStart, fileChanged = s, f
	}
	defer restore(processStart, fileChanged)
	fileChanged = func(string) (time.Time, error) { return changed, nil }

	for _, c := range []struct {
		name    string
		started time.Time
		err     error
		want    bool
	}{
		{"started before the file was replaced", changed.Add(-3 * time.Hour), nil, true},
		{"started after", changed.Add(time.Minute), nil, false},
		{"started within the clock's coarseness", changed.Add(-time.Second), nil, false},
		{"unreadable process", time.Time{}, errors.New("gone"), false},
	} {
		processStart = func(int) (time.Time, error) { return c.started, c.err }
		if got := residentStale(42, "/bin/bayd"); got != c.want {
			t.Errorf("%s: stale = %v, want %v", c.name, got, c.want)
		}
	}
	if residentStale(0, "/bin/bayd") {
		t.Error("a job with no process is not stale; whether it runs is checked elsewhere")
	}
}

// TestResidentStaleThisProcess reads the real clocks: this test's process
// started before a file written now, and after one written before it ran.
func TestResidentStaleThisProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin")
	time.Sleep(3 * time.Second)
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !residentStale(os.Getpid(), path) {
		t.Fatal("a process started before its executable was written is stale")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if residentStale(os.Getpid(), exe) {
		t.Fatal("a process started from its executable as it is is not stale")
	}
}
