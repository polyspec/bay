package stack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeServices struct {
	mu             sync.Mutex
	instances      map[string]Instance
	images         map[string]string
	calls          [][]string
	events         []string
	healthFailures int
	failInit       bool
	blockInit      bool
	sequence       int
	// stopLag is the number of list reads after an attached start returns
	// that still report the container running.
	stopLag int
	lagging string
	// attached changes the container when an attached start returns.
	attached func(*Instance)
}

func fakeEngine(t *testing.T) (*serviceEngine, *fakeServices) {
	t.Helper()
	f := &fakeServices{instances: map[string]Instance{}, images: map[string]string{}}
	return &serviceEngine{
		command: f.command, dir: t.TempDir(), timeout: time.Second, engine: AppleEngine,
		subscribeName: func(context.Context, string, []netip.Addr) (*nameSubscription, error) {
			events := make(chan nameEvent, 1)
			events <- nameEvent{address: netip.MustParseAddr("192.0.2.1"), added: true}
			close(events)
			return &nameSubscription{events: events, close: func() error { return nil }}, nil
		},
	}, f
}
func (f *fakeServices) command(ctx context.Context, args ...string) ([]byte, error) {
	// Each fake CLI invocation owns its memory; cross-process flock is not a
	// Go race-detector synchronization primitive.
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string{}, args...))
	switch args[0] {
	case "ls":
		if f.lagging != "" {
			if f.stopLag == 0 {
				in := f.instances[f.lagging]
				in.State = "stopped"
				f.instances[f.lagging] = in
				f.lagging = ""
			} else {
				f.stopLag--
			}
		}
		raw := []any{}
		for _, in := range f.instances {
			raw = append(raw, map[string]any{"configuration": map[string]any{"id": in.Name, "creationDate": in.Created, "labels": in.Labels, "image": map[string]any{"descriptor": map[string]string{"digest": in.ImageDigest}}}, "status": map[string]any{"state": in.State, "startedDate": in.Started, "networks": []any{map[string]string{"ipv4Address": in.IPv4}}}})
		}
		return json.Marshal(raw)
	case "image":
		if args[1] != "inspect" {
			return nil, errors.New("unexpected pull")
		}
		digest := f.images[args[2]]
		if digest == "" {
			digest = "sha256:" + strings.Repeat("a", 64)
		}
		return json.Marshal([]any{map[string]any{"configuration": map[string]any{"descriptor": map[string]string{"digest": digest}}}})
	case "volume":
		return []byte("[]"), nil
	case "rm":
		delete(f.instances, args[len(args)-1])
		f.events = append(f.events, "rm:"+args[len(args)-1])
		return nil, nil
	case "start":
		name := args[len(args)-1]
		in := f.instances[name]
		in.State = "running"
		in.IPv4 = "192.0.2.1/24"
		in.Started += "-start"
		f.instances[name] = in
		f.events = append(f.events, "start:"+name)
		if len(args) == 3 && args[1] == "--attach" {
			if f.blockInit {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			if f.stopLag > 0 {
				f.lagging = name
			} else {
				in.State = "stopped"
			}
			if f.attached != nil {
				f.attached(&in)
			}
			f.instances[name] = in
			if f.failInit {
				return nil, errors.New("exit 7")
			}
		}
		return nil, nil
	case "stop":
		name := args[len(args)-1]
		in := f.instances[name]
		in.State = "stopped"
		f.instances[name] = in
		f.events = append(f.events, "stop:"+name)
		return nil, nil
	case "create":
		in := Instance{Labels: map[string]string{}, State: "stopped", ImageDigest: "sha256:" + strings.Repeat("a", 64)}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--name":
				i++
				in.Name = args[i]
			case "--label":
				i++
				key, value, _ := strings.Cut(args[i], "=")
				in.Labels[key] = value
			}
		}
		for ref, digest := range f.images {
			for _, arg := range args {
				if arg == ref {
					in.ImageDigest = digest
				}
			}
		}
		f.sequence++
		in.Created = fmt.Sprint(f.sequence)
		f.instances[in.Name] = in
		f.events = append(f.events, "create:"+in.Name)
		return nil, nil
	case "exec":
		f.events = append(f.events, "exec:"+args[1])
		if f.healthFailures > 0 {
			f.healthFailures--
			return nil, errors.New("not ready")
		}
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected command %s", args[0])
}

func TestServiceEngineImageChangeAndConcurrentUp(t *testing.T) {
	cfg := lifecycleConfig(t)
	e, f := fakeEngine(t)
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		wg.Go(func() { errors <- e.start(cfg, cfg.Sorted(), false) })
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	runs := 0
	for _, event := range f.events {
		if strings.HasPrefix(event, "create:") {
			runs++
		}
	}
	if runs != 3 {
		t.Fatalf("concurrent up created %d containers", runs)
	}
	f.images["platform"] = "sha256:" + strings.Repeat("b", 64)
	f.events = nil
	if err := e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.events, []string{"exec:app-db", "rm:app-initialize", "create:app-initialize", "start:app-initialize", "stop:app-web", "rm:app-web", "create:app-web", "start:app-web"}) {
		t.Fatalf("image change did not reconcile only selected image: %v", f.events)
	}
	for _, args := range f.calls {
		if args[0] == "create" && strings.Contains(strings.Join(args, " "), "@sha256:") {
			t.Fatal("creation rewrote a local image reference into an unavailable alias")
		}
	}
}

func TestServiceEngineReportsStartAndEveryHealthAttempt(t *testing.T) {
	cfg := lifecycleConfig(t)
	e, f := fakeEngine(t)
	var lines []string
	e.progress = func(line string) { lines = append(lines, line) }
	f.healthFailures = 2
	if err := e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	// A caller that streams output learns the container is up before the health wait, and sees
	// each attempt, so a start period of minutes is never a silent wait.
	want := []string{
		"db: started container app-db",
		"db: waiting for its healthcheck, up to",
		"db: not healthy yet after",
		"db: not healthy yet after",
		"db: healthy after",
		"web: started container app-web",
	}
	var got []string
	for _, line := range lines {
		if strings.Contains(line, "started container") || strings.Contains(line, "health") {
			got = append(got, line)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("progress=%v", lines)
	}
	for i, prefix := range want {
		if !strings.HasPrefix(got[i], prefix) {
			t.Fatalf("progress line %d=%q, want %q", i, got[i], prefix)
		}
	}
	e.progress = nil
	if err := e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
}

func TestServiceEngineStopAndRemoveOwnershipAndOrder(t *testing.T) {
	cfg := lifecycleConfig(t)
	e, f := fakeEngine(t)
	if err := e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	f.events = nil
	if err := e.stop(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.events, []string{"stop:app-web", "stop:app-db"}) {
		t.Fatalf("wrong stop order: %v", f.events)
	}
	f.events = nil
	foreign := f.instances["app-db"]
	foreign.Labels[LabelGroup] = "foreign"
	f.instances["app-db"] = foreign
	if err := e.stop(cfg, cfg.Sorted(), true); err == nil || len(f.events) != 0 {
		t.Fatal("down changed containers before checking every owner")
	}
	foreign.Labels[LabelGroup] = "app"
	f.instances["app-db"] = foreign
	if err := e.stop(cfg, cfg.Sorted(), true); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.events, []string{"rm:app-web", "rm:app-initialize", "rm:app-db"}) {
		t.Fatalf("wrong removal order: %v", f.events)
	}
}

func TestServiceEngineChangedDatabaseInvalidatesInitializer(t *testing.T) {
	for _, change := range []string{"volume", "recreated"} {
		t.Run(change, func(t *testing.T) {
			cfg := lifecycleConfig(t)
			e, f := fakeEngine(t)
			if err := e.start(cfg, cfg.Sorted(), false); err != nil {
				t.Fatal(err)
			}
			if change == "volume" {
				cfg.Services["db"].Volumes = []string{"replacement:/data"}
			} else {
				in := f.instances["app-db"]
				in.Created = "externally-recreated"
				f.instances[in.Name] = in
			}
			f.events = nil
			if err := e.start(cfg, cfg.Sorted(), false); err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, event := range f.events {
				if event == "create:app-initialize" {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("changed database reused old initializer: %v", f.events)
			}
		})
	}
}
func lifecycleConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := Load(write(t, `name: app
services:
  db:
    image: postgres
    healthcheck: {test: [CMD, pg_isready, -h127.0.0.1, -U, postgres], interval: 1ms, timeout: 10ms, retries: 3}
  initialize:
    image: platform
    command: [init]
    depends_on: {db: {condition: service_healthy}}
  web:
    image: platform
    user: '501:20'
    read_only: true
    cap_drop: [ALL]
    command: [serve]
    depends_on: {initialize: {condition: service_completed_successfully}}
`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
func TestServiceEngineDependencyOrderAndUnchangedReuse(t *testing.T) {
	cfg := lifecycleConfig(t)
	e, f := fakeEngine(t)
	f.healthFailures = 2
	if err := e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	want := []string{"create:app-db", "start:app-db", "exec:app-db", "exec:app-db", "exec:app-db", "create:app-initialize", "start:app-initialize", "create:app-web", "start:app-web"}
	if !reflect.DeepEqual(f.events, want) {
		t.Fatalf("events=%v", f.events)
	}
	before := f.instances["app-db"]
	f.events = nil
	if err := e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.events, []string{"exec:app-db"}) || !reflect.DeepEqual(before, f.instances["app-db"]) {
		t.Fatalf("unchanged up restarted containers: %v", f.events)
	}
	args := strings.Join(serviceArguments(cfg.Name, cfg.Services["web"], "digest"), " ")
	for _, part := range []string{"--user 501:20", "--read-only", "--cap-drop ALL"} {
		if !strings.Contains(args, part) {
			t.Fatalf("missing %s", part)
		}
	}
	cfg.Services["web"].Env = map[string]string{"REVISION": "2"}
	f.events = nil
	if err := e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.events, []string{"exec:app-db", "stop:app-web", "rm:app-web", "create:app-web", "start:app-web"}) {
		t.Fatalf("changed web touched unrelated service: %v", f.events)
	}
}
func TestServiceEnginePreflightsEveryContainerOwner(t *testing.T) {
	for _, labels := range []map[string]string{{}, {LabelRole: roleProxy, LabelGroup: "app", LabelService: "web"}, {LabelRole: roleService, LabelGroup: "another", LabelService: "web"}, {LabelRole: roleService, LabelGroup: "app", LabelService: "other"}} {
		cfg := lifecycleConfig(t)
		e, f := fakeEngine(t)
		f.instances["app-web"] = Instance{Name: "app-web", Labels: labels, State: "running"}
		if err := e.start(cfg, cfg.Sorted(), false); err == nil {
			t.Fatal("foreign container accepted")
		}
		if len(f.events) != 0 {
			t.Fatal("earlier service changed before ownership rejection")
		}
	}
}

func TestServiceEngineDoesNotReplaceOwnedContainerWithoutConfigurationEvidence(t *testing.T) {
	cfg := lifecycleConfig(t)
	e, f := fakeEngine(t)
	s := cfg.Services["db"]
	f.instances[s.ContainerName] = Instance{Name: s.ContainerName, State: "running", Labels: map[string]string{LabelRole: roleService, LabelGroup: cfg.Name, LabelService: s.Name}}
	if err := e.start(cfg, cfg.Sorted(), false); err == nil || len(f.events) != 0 {
		t.Fatal("unproven existing configuration was implicitly replaced")
	}
}
func TestServiceEngineFailedDependencyNeverStartsWeb(t *testing.T) {
	for _, failure := range []string{"health", "exit", "timeout"} {
		t.Run(failure, func(t *testing.T) {
			cfg := lifecycleConfig(t)
			e, f := fakeEngine(t)
			switch failure {
			case "health":
				f.healthFailures = 100
			case "exit":
				f.failInit = true
			case "timeout":
				f.blockInit = true
				e.timeout = 20 * time.Millisecond
			}
			if err := e.start(cfg, cfg.Sorted(), false); err == nil {
				t.Fatal("dependency failure was success")
			}
			if _, ok := f.instances["app-web"]; ok {
				t.Fatal("web started after dependency failure")
			}
			if _, err := os.Stat(e.completionPath("app-initialize")); !os.IsNotExist(err) {
				t.Fatal("failed initialization was recorded successful")
			}
		})
	}
}
func TestServiceEngineWaitsForAttachedInitializerToStop(t *testing.T) {
	cfg := lifecycleConfig(t)
	e, f := fakeEngine(t)
	f.stopLag = 3
	if err := e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	if f.lagging != "" || f.stopLag != 0 {
		t.Fatal("initializer completed before the runtime reported it stopped")
	}
	if _, err := os.Stat(e.completionPath("app-initialize")); err != nil {
		t.Fatalf("completed initializer was not recorded: %v", err)
	}
	if _, ok := f.instances["app-web"]; !ok {
		t.Fatal("web did not start after the initializer stopped")
	}
}

func TestServiceEngineNamesEachDifferingStoppedField(t *testing.T) {
	fields := []string{"state", "creation time", "start time", "configuration label", "image digest"}
	for _, c := range []struct {
		field  string
		change func(*Instance)
	}{
		{"state", func(in *Instance) { in.State = "unknown" }},
		{"creation time", func(in *Instance) { in.Created = "replacement" }},
		{"start time", func(in *Instance) { in.Started = "" }},
		{"configuration label", func(in *Instance) {
			in.Labels = map[string]string{LabelRole: roleService, LabelGroup: "app", LabelService: "initialize", LabelConfig: "other"}
		}},
		{"image digest", func(in *Instance) { in.ImageDigest = "sha256:" + strings.Repeat("c", 64) }},
	} {
		t.Run(c.field, func(t *testing.T) {
			cfg := lifecycleConfig(t)
			e, f := fakeEngine(t)
			e.timeout = 300 * time.Millisecond
			f.attached = c.change
			err := e.start(cfg, cfg.Sorted(), false)
			if err == nil {
				t.Fatal("changed initializer identity was accepted")
			}
			for _, field := range fields {
				if named := strings.Contains(err.Error(), field); named != (field == c.field) {
					t.Fatalf("error naming %s is %v: %v", field, named, err)
				}
			}
			if _, err := os.Stat(e.completionPath("app-initialize")); !os.IsNotExist(err) {
				t.Fatal("changed initializer identity was recorded successful")
			}
			if _, ok := f.instances["app-web"]; ok {
				t.Fatal("web started after a changed initializer identity")
			}
		})
	}
}

func TestServiceEngineRejectsUnprovenOrExternallyRestartedCompletion(t *testing.T) {
	for _, change := range []string{"started", "created", "missing", "corrupt"} {
		t.Run(change, func(t *testing.T) {
			cfg := lifecycleConfig(t)
			e, f := fakeEngine(t)
			if err := e.start(cfg, cfg.Sorted(), false); err != nil {
				t.Fatal(err)
			}
			in := f.instances["app-initialize"]
			switch change {
			case "started":
				in.Started = "external-restart"
			case "created":
				in.Created = "replacement"
			case "missing":
				if err := os.Remove(e.completionPath(in.Name)); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := os.WriteFile(e.completionPath(in.Name), []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			f.instances[in.Name] = in
			f.events = nil
			if err := e.start(cfg, cfg.Sorted(), false); err == nil {
				t.Fatal("unproven completion accepted")
			}
			for _, event := range f.events {
				if strings.HasPrefix(event, "create:") || strings.HasPrefix(event, "rm:") {
					t.Fatalf("unproven initializer changed implicitly: %v", f.events)
				}
			}
			if err := e.start(cfg, []*Service{cfg.Services["initialize"]}, true); err != nil {
				t.Fatalf("explicit retry failed: %v", err)
			}
		})
	}
}
