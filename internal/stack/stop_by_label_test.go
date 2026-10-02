package stack

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// labelledEngine is an engine whose container list is fixed, recording every
// command it is asked to run.
func labelledEngine(t *testing.T, containers []Instance) (*serviceEngine, *[]string) {
	t.Helper()
	var ran []string
	raw := make([]map[string]any, 0, len(containers))
	for _, c := range containers {
		raw = append(raw, map[string]any{
			"configuration": map[string]any{"id": c.Name, "labels": c.Labels},
			"status":        map[string]any{"state": c.State, "startedDate": c.Started},
		})
	}
	list, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	e := &serviceEngine{timeout: 5 * time.Second, engine: AppleEngine}
	e.command = func(ctx context.Context, args ...string) ([]byte, error) {
		if args[0] == "ls" {
			return list, nil
		}
		ran = append(ran, strings.Join(args, " "))
		return nil, nil
	}
	return e, &ran
}

func serviceOf(group, service, state, started string) Instance {
	return Instance{
		Name:    group + "-" + service,
		State:   state,
		Started: started,
		Labels:  map[string]string{LabelRole: roleService, LabelGroup: group, LabelService: service},
	}
}

// What to stop is read from the containers. The window's Stop failed with
// "stat …/compose.yaml: no such file or directory" while the project's
// container was running; no file is involved here at all.
func TestAProjectIsRemovedFromItsContainersWithoutItsFile(t *testing.T) {
	e, ran := labelledEngine(t, []Instance{
		serviceOf("crudui", "comparison", "running", "2026-10-03T06:00:00Z"),
		serviceOf("other", "web", "running", "2026-10-03T06:00:00Z"),
	})
	if err := e.removeProject("crudui"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(*ran, "\n") != "rm --force crudui-comparison" {
		t.Errorf("ran:\n%s\nwant only the project's container removed", strings.Join(*ran, "\n"))
	}
}

// A service removed from the file after it started still carries the project's
// label. A down driven by the file left it running.
func TestAContainerTheFileNoLongerNamesIsRemovedToo(t *testing.T) {
	e, ran := labelledEngine(t, []Instance{
		serviceOf("p", "web", "running", "2026-10-03T06:00:01Z"),
		serviceOf("p", "dropped", "running", "2026-10-03T06:00:02Z"),
	})
	if err := e.removeProject("p"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(*ran, "\n"), "rm --force p-dropped") {
		t.Errorf("the container no file names was left: %v", *ran)
	}
}

// Services start in dependency order, so the reverse of the start order stops a
// service before the services it depends on.
func TestContainersStopInTheReverseOfTheOrderTheyStarted(t *testing.T) {
	e, ran := labelledEngine(t, []Instance{
		serviceOf("p", "db", "running", "2026-10-03T06:00:01Z"),
		serviceOf("p", "app", "running", "2026-10-03T06:00:09Z"),
		serviceOf("p", "cache", "running", "2026-10-03T06:00:04Z"),
	})
	if err := e.stopProject("p", nil); err != nil {
		t.Fatal(err)
	}
	want := "stop p-app\nstop p-cache\nstop p-db"
	if got := strings.Join(*ran, "\n"); got != want {
		t.Errorf("stopped in the order:\n%s\nwant:\n%s", got, want)
	}
}

// Stopping named services stops those and nothing else, and a container that is
// already stopped is not asked to stop again.
func TestStoppingNamedServicesStopsOnlyThoseThatRun(t *testing.T) {
	e, ran := labelledEngine(t, []Instance{
		serviceOf("p", "db", "running", "2026-10-03T06:00:01Z"),
		serviceOf("p", "app", "stopped", "2026-10-03T06:00:09Z"),
		serviceOf("p", "web", "running", "2026-10-03T06:00:05Z"),
	})
	if err := e.stopProject("p", []string{"app", "web"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*ran, "\n"); got != "stop p-web" {
		t.Errorf("ran %q, want only the running named service stopped", got)
	}
}

// A proxy or another role sharing the group label is not a service of the
// project and is not removed with it.
func TestOnlyServiceContainersAreRemoved(t *testing.T) {
	proxy := serviceOf("p", "edge", "running", "2026-10-03T06:00:00Z")
	proxy.Labels[LabelRole] = roleProxy
	e, ran := labelledEngine(t, []Instance{proxy, serviceOf("p", "web", "running", "2026-10-03T06:00:01Z")})
	if err := e.removeProject("p"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*ran, "\n"); got != "rm --force p-web" {
		t.Errorf("ran %q, want only the service removed", got)
	}
}
