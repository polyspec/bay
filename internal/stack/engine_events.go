package stack

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
)

// containerEvents are the Docker events after which a container's address,
// or whether it runs, may have changed: a container starting, ending or being
// removed, and joining or leaving a network.
var containerEvents = []string{
	"--filter", "type=container", "--filter", "type=network",
	"--filter", "event=start", "--filter", "event=die", "--filter", "event=destroy",
	"--filter", "event=connect", "--filter", "event=disconnect",
}

// WatchDockerEvents calls changed for each Docker event after which a
// container's address may differ, and blocks until ctx ends or the stream
// does, for example when the daemon restarts. ready is called once the
// stream runs, so a caller that reads the state then misses no change made
// after it.
func WatchDockerEvents(ctx context.Context, ready, changed func()) error {
	bin := engineBin(DockerEngine)
	if bin == "" {
		return fmt.Errorf("docker is not installed")
	}
	args := append([]string{"events", "--format", "{{.Type}} {{.Action}}"}, containerEvents...)
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	ready()
	lines := bufio.NewScanner(out)
	for lines.Scan() {
		changed()
	}
	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("docker events: %w", err)
	}
	return ctx.Err()
}
