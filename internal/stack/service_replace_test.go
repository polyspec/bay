package stack

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestReplacementStopsBeforeRemoving(t *testing.T) {
	for _, running := range []bool{true, false} {
		t.Run(map[bool]string{true: "running", false: "stopped"}[running], func(t *testing.T) {
			cfg := lifecycleConfig(t)
			e, f := fakeEngine(t)
			service := cfg.Services["web"]
			if err := e.reconcile(cfg.Name, service, false); err != nil {
				t.Fatal(err)
			}
			if !running {
				in := f.instances[service.ContainerName]
				in.State = "stopped"
				f.instances[in.Name] = in
			}
			f.events, f.calls = nil, nil
			if err := e.reconcile(cfg.Name, service, true); err != nil {
				t.Fatal(err)
			}
			want := []string{"rm:app-web", "create:app-web", "start:app-web"}
			if running {
				want = append([]string{"stop:app-web"}, want...)
			}
			if !reflect.DeepEqual(f.events, want) {
				t.Fatalf("replacement events %v, want %v", f.events, want)
			}
			for _, call := range f.calls {
				if call[0] == "rm" && !reflect.DeepEqual(call, []string{"rm", "app-web"}) {
					t.Fatalf("replacement must remove without force: %v", call)
				}
			}
		})
	}
}

func TestReplacementPreservesServiceAfterStopFailure(t *testing.T) {
	cfg := lifecycleConfig(t)
	e, f := fakeEngine(t)
	service := cfg.Services["web"]
	if err := e.reconcile(cfg.Name, service, false); err != nil {
		t.Fatal(err)
	}
	original := f.instances[service.ContainerName].Created
	f.events = nil
	want := errors.New("service stop failed")
	e.command = func(ctx context.Context, args ...string) ([]byte, error) {
		if args[0] == "stop" {
			return nil, want
		}
		return f.command(ctx, args...)
	}
	if err := e.reconcile(cfg.Name, service, true); !errors.Is(err, want) {
		t.Fatalf("replacement error %v, want %v", err, want)
	}
	if len(f.events) != 0 || f.instances[service.ContainerName].Created != original {
		t.Fatalf("failed stop changed the service: %v", f.events)
	}
}
