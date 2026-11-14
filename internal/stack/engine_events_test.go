package stack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each event line is one change, the stream is reported open before the
// first, and the filters ask Docker only for the events that move an address.
func TestDockerEventsAreFollowed(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("DOCKER_EVENTS_HELPER", argsFile)
	t.Setenv("DOCKER_BIN", os.Args[0])
	var order []string
	err := WatchDockerEvents(context.Background(),
		func() { order = append(order, "ready") },
		func() { order = append(order, "changed") })
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(order, " "); got != "ready changed changed changed" {
		t.Fatalf("calls %q", got)
	}
	args, _ := os.ReadFile(argsFile)
	for _, want := range []string{"events", "type=container", "event=start", "event=die", "event=destroy", "event=connect", "event=disconnect"} {
		if !strings.Contains(string(args), want) {
			t.Fatalf("docker %s: no %s", args, want)
		}
	}
}
