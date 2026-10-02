package stack

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"
)

// This exercises the existing readiness contract without changing the shared
// proxy, DNS registration, or any existing project container.
func TestReadinessActualRuntime(t *testing.T) {
	if os.Getenv("BAY_SERVICE_E2E") != "1" {
		t.Skip("set BAY_SERVICE_E2E=1 for isolated service containers")
	}
	before, err := List()
	if err != nil {
		t.Fatal(err)
	}
	group := fmt.Sprintf("ctl-ready-%d", time.Now().UnixNano())
	svc := &Service{
		Name: "web", ContainerName: group + "-web", Image: "docker.io/library/node:26.8.1-trixie-slim",
		Command: []string{"node", "-e", "const t=setInterval(()=>{if(require('fs').existsSync('/tmp/release')){clearInterval(t);require('http').createServer((q,s)=>s.end('ready')).listen(8080)}},50)"},
		Port:    8080, Network: ProxyNetwork,
	}
	t.Cleanup(func() {
		in, found, err := Lookup(svc.ContainerName)
		if err != nil {
			t.Error(err)
		} else if found {
			if err := owns(group, svc, in); err != nil {
				t.Error(err)
			} else if err := Remove(svc.ContainerName); err != nil {
				t.Error(err)
			}
		}
		after, err := List()
		if err != nil {
			t.Error(err)
			return
		}
		actual := map[string]Instance{}
		for _, in := range after {
			actual[in.Name] = in
		}
		for _, in := range before {
			if !reflect.DeepEqual(in, actual[in.Name]) {
				t.Errorf("preexisting container changed: %s", in.Name)
			}
		}
	})
	if err := StartService(group, svc); err != nil {
		t.Fatal(err)
	}
	instances, err := Instances()
	if err != nil {
		t.Fatal(err)
	}
	var current ServiceInstance
	for _, in := range instances {
		if in.Container == svc.ContainerName {
			current = in
		}
	}
	if !current.Running() || current.Ready() {
		t.Fatal("running process must remain unready until it listens")
	}
	// Without the file the declared port is unknown, so nothing is measured.
	status := serviceFromInstance(current, nil)
	if status.State != "running" || status.Accepting != nil || !status.Live() {
		t.Fatalf("unready process status without its file = %+v", status)
	}
	cfg, err := Load(write(t, fmt.Sprintf("name: %s\nservices:\n  web:\n    image: node\n    expose: ['8080']\n", group)))
	if err != nil {
		t.Fatal(err)
	}
	machine := NewMachine(t.TempDir())
	byContainer := map[string]ServiceInstance{svc.ContainerName: current}
	configured := groupStatus(machine, cfg.Ref(), byContainer, nil)
	// With the file the declared port is measured beside the engine's state.
	if len(configured.Services) != 1 || configured.Services[0].State != "running" || !configured.Services[0].Closed() || !configured.Services[0].Live() {
		t.Fatalf("configured unready process status = %+v", configured)
	}
	pending, err := WaitReady([]string{svc.ContainerName}, 100*time.Millisecond)
	if err != nil || len(pending) != 1 || pending[0] != svc.ContainerName {
		t.Fatalf("unready wait = %v, %v", pending, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := serviceCommand(ctx, "exec", svc.ContainerName, "touch", "/tmp/release"); err != nil {
		t.Fatal(err)
	}
	if pending, err := WaitReady([]string{svc.ContainerName}, 10*time.Second); err != nil || len(pending) != 0 {
		t.Fatalf("listening process remains pending: %v, %v", pending, err)
	}
	if status := serviceFromInstance(current, nil); status.State != "running" || !status.Live() {
		t.Fatalf("listening process status = %+v", status)
	}
	if configured := groupStatus(machine, cfg.Ref(), byContainer, nil); len(configured.Services) != 1 || configured.Services[0].Closed() || !configured.Services[0].Live() {
		t.Fatalf("configured listening process status = %+v", configured)
	}
}
