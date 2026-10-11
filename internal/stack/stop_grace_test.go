package stack

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// graceProject has one service with stop_grace_period and one without it.
func graceProject(t *testing.T, grace string) *Config {
	t.Helper()
	cfg, err := Load(write(t, "name: app\nservices:\n  web:\n    image: platform\n    stop_grace_period: "+grace+
		"\n  cache:\n    image: platform\n"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// stopCalls returns the arguments of every stop the fake engine received.
func stopCalls(f *fakeServices) [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out [][]string
	for _, c := range f.calls {
		if c[0] == "stop" {
			out = append(out, c)
		}
	}
	return out
}

// callIndex returns the position of the first call equal to want, or -1.
func callIndex(f *fakeServices, want []string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, c := range f.calls {
		if reflect.DeepEqual(c, want) {
			return i
		}
	}
	return -1
}

func TestStopGraceRefusesAnInvalidValue(t *testing.T) {
	for _, value := range []string{"soon", "1500ms", "1.5s", "-5s", "30", `""`} {
		t.Run(value, func(t *testing.T) {
			_, err := Load(write(t, "services:\n  web:\n    image: nginx\n    stop_grace_period: "+value+"\n"))
			if err == nil {
				t.Fatalf("stop_grace_period %s was accepted", value)
			}
			unquoted := strings.Trim(value, `"`)
			if !strings.Contains(err.Error(), `"web"`) || !strings.Contains(err.Error(), unquoted) {
				t.Fatalf("err = %v, want it to name service web and value %s", err, value)
			}
		})
	}
}

func TestStopGraceIsPassedAsWholeSeconds(t *testing.T) {
	for value, seconds := range map[string]string{"0s": "0", "30s": "30", "1m30s": "90"} {
		t.Run(value, func(t *testing.T) {
			cfg := graceProject(t, value)
			e, f := fakeEngine(t)
			web := cfg.Services["web"]
			if err := e.reconcile(cfg.Name, web, false); err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			f.calls = nil
			f.mu.Unlock()
			if err := e.reconcile(cfg.Name, web, true); err != nil {
				t.Fatal(err)
			}
			want := []string{"stop", "--time", seconds, web.ContainerName}
			if got := stopCalls(f); !reflect.DeepEqual(got, [][]string{want}) {
				t.Fatalf("stop calls = %q, want %q", got, want)
			}
		})
	}
}

func TestStopWithoutGraceKeepsTheDefault(t *testing.T) {
	cfg := graceProject(t, "30s")
	e, f := fakeEngine(t)
	cache := cfg.Services["cache"]
	if err := e.reconcile(cfg.Name, cache, false); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.calls = nil
	f.mu.Unlock()
	if err := e.reconcile(cfg.Name, cache, true); err != nil {
		t.Fatal(err)
	}
	want := []string{"stop", cache.ContainerName}
	if got := stopCalls(f); !reflect.DeepEqual(got, [][]string{want}) {
		t.Fatalf("stop calls = %q, want %q", got, want)
	}
}

func TestStopServicesPassesTheRecordedGrace(t *testing.T) {
	cfg := graceProject(t, "30s")
	e, f := fakeEngine(t)
	for _, s := range cfg.Sorted() {
		if err := e.reconcile(cfg.Name, s, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.stopProject(cfg.Name, nil); err != nil {
		t.Fatal(err)
	}
	web := []string{"stop", "--time", "30", cfg.Services["web"].ContainerName}
	cache := []string{"stop", cfg.Services["cache"].ContainerName}
	if callIndex(f, web) < 0 || callIndex(f, cache) < 0 {
		t.Fatalf("stop calls = %q, want %q and %q", stopCalls(f), web, cache)
	}
}

func TestDownStopsEveryRunningContainerBeforeRemoving(t *testing.T) {
	cfg := graceProject(t, "30s")
	e, f := fakeEngine(t)
	for _, s := range cfg.Sorted() {
		if err := e.reconcile(cfg.Name, s, false); err != nil {
			t.Fatal(err)
		}
	}
	f.mu.Lock()
	f.calls = nil
	f.mu.Unlock()
	if err := e.removeProject(cfg.Name); err != nil {
		t.Fatal(err)
	}
	web := cfg.Services["web"].ContainerName
	stop := callIndex(f, []string{"stop", "--time", "30", web})
	remove := callIndex(f, []string{"rm", "--force", web})
	if stop < 0 || remove < 0 || stop > remove {
		t.Fatalf("calls = %q, want the graceful stop of %s before its removal", f.calls, web)
	}
	// A service without the key gets the runtime default: a plain stop, not a kill.
	cache := cfg.Services["cache"].ContainerName
	plain := callIndex(f, []string{"stop", cache})
	cacheRemove := callIndex(f, []string{"rm", "--force", cache})
	if plain < 0 || cacheRemove < 0 || plain > cacheRemove {
		t.Fatalf("calls = %q, want a plain stop of %s before its removal", f.calls, cache)
	}
}

func TestDownKeepsTheContainerWhenItsStopFails(t *testing.T) {
	cfg := graceProject(t, "30s")
	e, f := fakeEngine(t)
	web := cfg.Services["web"]
	if err := e.reconcile(cfg.Name, web, false); err != nil {
		t.Fatal(err)
	}
	want := errors.New("stop refused")
	e.command = func(ctx context.Context, args ...string) ([]byte, error) {
		if args[0] == "stop" {
			return nil, want
		}
		return f.command(ctx, args...)
	}
	if err := e.removeProject(cfg.Name); !errors.Is(err, want) {
		t.Fatalf("down error %v, want %v", err, want)
	}
	if _, ok := f.instances[web.ContainerName]; !ok {
		t.Fatal("down removed a container whose stop failed")
	}
}

func TestUnsetGraceIsNotInTheConfigIdentity(t *testing.T) {
	cfg := graceProject(t, "30s")
	encoded, err := json.Marshal(cfg.Services["cache"])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "StopGrace") {
		t.Fatalf("service without stop_grace_period encodes a grace field: %s", encoded)
	}
}
