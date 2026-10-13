package stack

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/polyspec/bay/internal/contract"
)

// signalProject has one service with stop_signal and a grace period, and one
// with neither.
func signalProject(t *testing.T, signal string) *Config {
	t.Helper()
	cfg, err := Load(write(t, "name: app\nservices:\n  web:\n    image: platform\n    stop_grace_period: 30s\n    stop_signal: "+signal+
		"\n  cache:\n    image: platform\n"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// An unknown signal is refused when the file is read, naming the service and the
// value. Names are accepted only in the form the contract lists.
func TestStopSignalRefusesAnUnknownSignal(t *testing.T) {
	for _, value := range []string{"SIGFOO", "sigquit", "QUIT", "15", `""`} {
		t.Run(value, func(t *testing.T) {
			_, err := Load(write(t, "services:\n  web:\n    image: nginx\n    stop_signal: "+value+"\n"))
			if err == nil {
				t.Fatalf("stop_signal %s was accepted", value)
			}
			unquoted := strings.Trim(value, `"`)
			if !strings.Contains(err.Error(), `"web"`) || !strings.Contains(err.Error(), unquoted) {
				t.Fatalf("err = %v, want it to name service web and value %s", err, value)
			}
		})
	}
}

// Every name the contract lists is accepted and kept as written.
func TestStopSignalAcceptsEveryContractName(t *testing.T) {
	names := contract.StopSignals()
	if len(names) == 0 {
		t.Fatal("the contract lists no stop signals")
	}
	for _, name := range names {
		cfg := signalProject(t, name)
		if got := cfg.Services["web"].StopSignal; got != name {
			t.Fatalf("StopSignal = %q, want %q", got, name)
		}
	}
}

// Stop passes the signal as --signal and the grace period as --time.
func TestStopSignalIsPassedToStop(t *testing.T) {
	cfg := signalProject(t, "SIGQUIT")
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
	want := []string{"stop", "--signal", "SIGQUIT", "--time", "30", web.ContainerName}
	if got := stopCalls(f); !reflect.DeepEqual(got, [][]string{want}) {
		t.Fatalf("stop calls = %q, want %q", got, want)
	}
}

// Stop and down read the signal recorded on the container. Down sends it before
// the container is removed.
func TestDownAndStopSendTheRecordedSignal(t *testing.T) {
	cfg := signalProject(t, "SIGQUIT")
	web := cfg.Services["web"].ContainerName
	e, f := fakeEngine(t)
	for _, s := range cfg.Sorted() {
		if err := e.reconcile(cfg.Name, s, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.stopProject(cfg.Name, nil); err != nil {
		t.Fatal(err)
	}
	stop := []string{"stop", "--signal", "SIGQUIT", "--time", "30", web}
	if callIndex(f, stop) < 0 {
		t.Fatalf("stop calls = %q, want %q", stopCalls(f), stop)
	}
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
	signalled := callIndex(f, stop)
	remove := callIndex(f, []string{"rm", "--force", web})
	if signalled < 0 || remove < 0 || signalled > remove {
		t.Fatalf("down calls = %q, want the signalled stop of %s before its removal", f.calls, web)
	}
}

// A service without the key gets no --signal, and its identity gets no field.
func TestUnsetStopSignalIsNotPassedOrEncoded(t *testing.T) {
	cfg := signalProject(t, "SIGQUIT")
	cache := cfg.Services["cache"]
	encoded, err := json.Marshal(cache)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "StopSignal") {
		t.Fatalf("service without stop_signal encodes a signal field: %s", encoded)
	}
	e, f := fakeEngine(t)
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

// The signal is recorded as a label when the container is created.
func TestStopSignalIsRecordedAsALabel(t *testing.T) {
	cfg := signalProject(t, "SIGQUIT")
	args := serviceArguments(cfg.Name, cfg.Services["web"], "fp")
	if !containsPair(args, LabelStopSignal+"=SIGQUIT") {
		t.Fatalf("create arguments %q carry no %s label", args, LabelStopSignal)
	}
	if containsPair(serviceArguments(cfg.Name, cfg.Services["cache"], "fp"), LabelStopSignal+"=SIGQUIT") {
		t.Fatal("a service without stop_signal has the label")
	}
}

// A recorded signal that is not a contract name is an error, and nothing is
// stopped or removed with it.
func TestAnUnknownRecordedSignalIsRefused(t *testing.T) {
	c := serviceOf("p", "web", "running", "2026-10-03T06:00:00Z")
	c.Labels[LabelStopSignal] = "SIGFOO"
	e, ran := labelledEngine(t, []Instance{c})
	if err := e.removeProject("p"); err == nil {
		t.Fatal("a container with an unknown recorded signal was removed")
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q before refusing the signal", *ran)
	}
}

func containsPair(args []string, pair string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--label" && args[i+1] == pair {
			return true
		}
	}
	return false
}
