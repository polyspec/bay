package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/polyspec/bay/internal/stack"
)

// The test binary also serves as the fixture executables. No shell script or
// actual container, trust, or launchd operation is run by these tests.
func TestMain(m *testing.M) {
	if os.Getenv("BAY_QUERY_FIXTURE") == "1" {
		args := os.Args[1:]
		switch filepath.Base(os.Args[0]) {
		case "container":
			if reflect.DeepEqual(args, []string{"ls", "--all", "--format", "json"}) {
				fmt.Print("[]")
				os.Exit(0)
			}
		case "security":
			if len(args) > 0 && args[0] == "verify-cert" {
				os.Exit(1)
			}
		case "launchctl":
			if len(args) > 0 && args[0] == "print" {
				os.Exit(1)
			}
		case "plutil":
			if len(args) > 0 && args[0] == "-extract" {
				os.Exit(1)
			}
		}
		os.Exit(99)
	}
	os.Exit(m.Run())
}

type fileState struct {
	Body     string
	Mode     os.FileMode
	Modified time.Time
}

func stateFiles(t *testing.T, root string) map[string]fileState {
	t.Helper()
	out := map[string]fileState{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		out[rel] = fileState{string(data), info.Mode(), info.ModTime()}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func queryFixture(t *testing.T) (*stack.Machine, string) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"container", "security", "launchctl", "plutil"} {
		if err = os.Symlink(executable, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	t.Setenv("BAY_QUERY_FIXTURE", "1")
	t.Setenv("CONTAINER_BIN", filepath.Join(bin, "container"))
	project := filepath.Join(root, "compose.yaml")
	if err = os.WriteFile(project, []byte("name: selected\nx-bay: {domain: selected.test}\nservices:\n  web: {image: example:test}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	previous := *file
	*file = project
	t.Cleanup(func() { *file = previous })
	return stack.NewMachine(filepath.Join(root, "state")), project
}

func captureQuery(t *testing.T, action func() error) (string, error) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = out
	defer func() { os.Stdout = previous; out.Close() }()
	err = action()
	body, readErr := os.ReadFile(out.Name())
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(body), err
}

func TestStatusLeavesMissingStateAndSelectedProjectUnregistered(t *testing.T) {
	m, _ := queryFixture(t)
	output, err := captureQuery(t, func() error { return status(m, []string{"--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(m.Dir); !os.IsNotExist(err) {
		t.Fatal("status created machine state")
	}
	var snapshot stack.Snapshot
	if json.Unmarshal([]byte(output), &snapshot) != nil {
		t.Fatal("invalid status JSON")
	}
	if len(snapshot.Groups) != 0 || snapshot.Machine.CATrusted || len(snapshot.Machine.Pending) == 0 {
		t.Fatal("missing setup or unregistered project was reported incorrectly")
	}
	if snapshot.Certificates.Authority.ReadError == "" {
		t.Fatal("missing public certificate was not reported in status JSON")
	}
}

func TestStatusReportsUnreadablePublicCertificate(t *testing.T) {
	m, _ := queryFixture(t)
	if err := os.Mkdir(m.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.Dir, "ca.crt"), []byte("not a certificate"), 0644); err != nil {
		t.Fatal(err)
	}
	before := stateFiles(t, m.Dir)
	output, err := captureQuery(t, func() error { return status(m, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "authority") || !strings.Contains(output, "unreadable:") {
		t.Fatal("status did not report the unreadable public certificate")
	}
	output, err = captureQuery(t, func() error { return status(m, []string{"--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	var snapshot stack.Snapshot
	if err := json.Unmarshal([]byte(output), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Certificates.Authority.ReadError == "" || snapshot.Machine.CATrusted {
		t.Fatal("status JSON did not report the unreadable public certificate")
	}
	if !reflect.DeepEqual(before, stateFiles(t, m.Dir)) {
		t.Fatal("status changed existing machine state")
	}
}

func TestDoctorLeavesMissingStateAbsent(t *testing.T) {
	m, _ := queryFixture(t)
	output, err := captureQuery(t, func() error { return doctor(m) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(m.Dir); !os.IsNotExist(err) {
		t.Fatal("doctor created machine state")
	}
	if !strings.Contains(output, "selected.test") || !strings.Contains(output, "create local certificate authority") || strings.Contains(output, "nothing to do") {
		t.Fatal("doctor did not report missing setup for selected Compose")
	}
}

func TestQueriesPreserveRegistrationAndReadNoPrivateKey(t *testing.T) {
	m, project := queryFixture(t)
	if _, err := stack.LoadOrCreateCA(m.Dir); err != nil {
		t.Fatal(err)
	}
	if err := m.Register(stack.GroupRef{Name: "selected", StackPath: project, Domains: []string{"previous.test"}}); err != nil {
		t.Fatal(err)
	}
	// A public diagnostic must work even when no usable signing key is present.
	if err := os.WriteFile(filepath.Join(m.Dir, "ca.key"), []byte("not a private key"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, keyState := range []string{"malformed", "absent"} {
		t.Run(keyState, func(t *testing.T) {
			if keyState == "absent" {
				if err := os.Remove(filepath.Join(m.Dir, "ca.key")); err != nil {
					t.Fatal(err)
				}
			}
			before := stateFiles(t, m.Dir)
			for _, action := range []func() error{func() error { return status(m, []string{"--json"}) }, func() error { return doctor(m) }} {
				if _, err := captureQuery(t, action); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, stateFiles(t, m.Dir)) {
					t.Fatal("query changed state bytes, mode or modification time")
				}
			}
		})
	}
}
