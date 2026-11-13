package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/polyspec/bay/internal/stack"
)

// registeredElsewhere registers the fixture project under another Compose
// path, the state a read command must leave as it is.
func registeredElsewhere(t *testing.T, m *stack.Machine) map[string]fileState {
	t.Helper()
	if err := m.Register(stack.GroupRef{Name: "selected", StackPath: "/elsewhere/compose.yaml"}); err != nil {
		t.Fatal(err)
	}
	return stateFiles(t, m.Dir)
}

// Registration happens only in up. status reads the Compose file of the
// working directory and leaves the registry as it is, also when the project
// is registered under another path.
func TestStatusDoesNotRegisterTheWorkingDirectoryProject(t *testing.T) {
	m, _ := queryFixture(t)
	before := registeredElsewhere(t, m)
	for _, args := range [][]string{nil, {"--json"}} {
		if _, err := captureQuery(t, func() error { return status(m, args) }); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, stateFiles(t, m.Dir)) {
			t.Fatalf("status %v changed the registry", args)
		}
	}
}

// install applies the machine setup and registers no project. The fixture has
// no bayd, so install stops with that error before any setup step.
func TestInstallDoesNotRegisterTheWorkingDirectoryProject(t *testing.T) {
	m, _ := queryFixture(t)
	before := registeredElsewhere(t, m)
	_, err := captureQuery(t, func() error { return install(m, nil) })
	if err == nil || !strings.Contains(err.Error(), "bayd") {
		t.Fatalf("install err = %v, want the missing bayd", err)
	}
	if !reflect.DeepEqual(before, stateFiles(t, m.Dir)) {
		t.Fatal("install changed the registry")
	}
}
