package stack

import (
	"reflect"
	"testing"
)

func TestReadinessSelectionExcludesCompletedDependencies(t *testing.T) {
	cfg, err := Load(write(t, `services:
  db:
    image: postgres
    expose: ["5432"]
  initialize:
    image: application
  web:
    image: application
    depends_on:
      initialize: {condition: service_completed_successfully}
`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{cfg.Services["db"].ContainerName, cfg.Services["web"].ContainerName}
	if got := containerNames(cfg.Sorted()); !reflect.DeepEqual(got, want) {
		t.Fatalf("TCP readiness targets = %v, want %v", got, want)
	}
	if got := containerNames([]*Service{cfg.Services["initialize"]}); len(got) != 0 {
		t.Fatalf("completed initializer requires TCP readiness: %v", got)
	}
}

func TestEmptyReadinessSelectionDoesNotReportConnections(t *testing.T) {
	var messages []string
	r := Runtime{Progress: func(message string) { messages = append(messages, message) }}
	r.reportReady(nil)
	if len(messages) != 0 {
		t.Fatalf("empty readiness selection performed a lookup or reported connections: %v", messages)
	}
}
