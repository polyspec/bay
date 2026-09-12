package stack

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// A project is registered before any work is done for it, because the window
// draws from the registry: a project registered after the work is invisible
// while a long run is under way and invisible for good when it fails.
func TestUpRegistersTheProjectBeforeAnyWork(t *testing.T) {
	rt := ownedMachine(t)
	cfg := oneService(t)
	// The engine is unreachable, so the first step that touches it fails. What
	// matters is that the project is already recorded when it does.
	t.Setenv("PATH", "")
	t.Setenv("CONTAINER_BIN", "/nonexistent/container")
	t.Setenv("DOCKER_BIN", "")
	if _, err := rt.Up(cfg); err == nil {
		t.Fatal("up succeeded with no engine")
	}
	groups, err := rt.Machine.Groups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("the project is not registered after a failed up: %v", groups)
	}
}

// Every step that can take more than a moment names the service and the step
// before it runs. Reporting after it finishes tells nobody where a command is
// stuck, so the order is what is checked: the words have to be on the screen
// while the step is still running.
func TestEachSlowStepIsAnnouncedBeforeItRuns(t *testing.T) {
	var events []string
	e := &serviceEngine{
		dir:      t.TempDir(),
		timeout:  5 * time.Second,
		engine:   AppleEngine,
		progress: func(line string) { events = append(events, "said: "+line) },
	}
	e.command = func(ctx context.Context, args ...string) ([]byte, error) {
		events = append(events, "ran: "+strings.Join(args, " "))
		switch args[0] {
		case "image":
			if args[1] == "inspect" {
				// Not found, so the pull runs and has to be announced first.
				return nil, errors.New("container image not found")
			}
		}
		return []byte("[]"), nil
	}
	svc := &Service{Name: "db", ContainerName: "p-db", Image: "mysql:8.4"}
	_, _ = e.image(context.Background(), svc.Image)

	pull, said := -1, -1
	for i, ev := range events {
		if said < 0 && strings.HasPrefix(ev, "said: pulling mysql:8.4") {
			said = i
		}
		if pull < 0 && strings.HasPrefix(ev, "ran: image pull") {
			pull = i
		}
	}
	if said < 0 {
		t.Fatalf("the pull was never announced:\n%s", strings.Join(events, "\n"))
	}
	if pull < 0 {
		t.Fatalf("the pull never ran:\n%s", strings.Join(events, "\n"))
	}
	if said > pull {
		t.Errorf("the pull was announced after it ran:\n%s", strings.Join(events, "\n"))
	}
}

// A healthcheck wait says how long it is prepared to wait, so a command sitting
// in one is not mistaken for a command that has hung.
func TestTheHealthcheckWaitStatesItsBound(t *testing.T) {
	var said []string
	e := &serviceEngine{
		dir:      t.TempDir(),
		timeout:  time.Second,
		engine:   AppleEngine,
		progress: func(line string) { said = append(said, line) },
	}
	e.command = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte("[]"), nil
	}
	svc := &Service{Name: "db", ContainerName: "p-db", Healthcheck: &Healthcheck{
		Test: []string{"CMD", "true"}, Interval: 2 * time.Second,
		Timeout: 3 * time.Second, Retries: 30,
	}}
	_ = e.healthy(svc)
	joined := strings.Join(said, "\n")
	if !strings.Contains(joined, "db: waiting for its healthcheck") {
		t.Fatalf("the wait was not announced:\n%s", joined)
	}
	if !strings.Contains(joined, "up to ") {
		t.Errorf("the wait does not say how long it will wait:\n%s", joined)
	}
}
