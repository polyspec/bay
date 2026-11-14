package stack

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScheduleMatchesLikeCron(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, c := range []struct {
		spec, at string
		want     bool
	}{
		{"0 3 * * *", "2026-11-14 03:00", true},
		{"0 3 * * *", "2026-11-14 03:01", false},
		{"*/15 * * * *", "2026-11-14 10:45", true},
		{"*/15 * * * *", "2026-11-14 10:46", false},
		{"0 9-17/4 * * 1-5", "2026-11-13 13:00", true},  // a Friday
		{"0 9-17/4 * * 1-5", "2026-11-14 13:00", false}, // a Saturday
		{"0 0 1 * 0", "2026-11-15 00:00", true},         // day of month or day of week
		{"0 0 * * 7", "2026-11-15 00:00", true},         // 7 is Sunday
		{"@daily", "2026-11-14 00:00", true},
	} {
		sc, err := ParseSchedule(c.spec)
		if err != nil {
			t.Fatalf("%s: %v", c.spec, err)
		}
		if got := sc.Matches(at(c.at)); got != c.want {
			t.Errorf("%s at %s: %v, want %v", c.spec, c.at, got, c.want)
		}
	}
	for _, bad := range []string{"", "* * * *", "60 * * * *", "* * * * 8", "*/0 * * * *", "a * * * *", "5-1 * * * *"} {
		if _, err := ParseSchedule(bad); err == nil {
			t.Errorf("%q: accepted", bad)
		}
	}
	sc, _ := ParseSchedule("30 2 * * *")
	next, ok := sc.Next(at("2026-11-14 02:30"))
	if !ok || !next.Equal(at("2026-11-15 02:30")) {
		t.Errorf("next after a run: %v %v", next, ok)
	}
}

func writeCompose(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestJobServicesAreLeftToTheQueue(t *testing.T) {
	cfg, err := Load(writeCompose(t, `name: ci
services:
  db:
    image: postgres:18
    expose: ["5432"]
  test:
    image: debian:trixie
    command: ["make", "test"]
    depends_on: [db]
    x-bay:
      job: true
      schedule: "0 3 * * *"
      timeout: 30m
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(cfg.Serving()); got != 1 || cfg.Serving()[0].Name != "db" {
		t.Fatalf("serving: %v", cfg.Serving())
	}
	test := cfg.Services["test"]
	if !test.Job || test.Schedule != "0 3 * * *" || test.JobTimeout != 30*time.Minute {
		t.Fatalf("job: %+v", test)
	}
	if _, err := selectServing(cfg, []string{"test"}); err == nil || !strings.Contains(err.Error(), "bay job add test") {
		t.Fatalf("start of a job: %v", err)
	}
	for name, body := range map[string]string{
		"depends on a job": "services:\n  a:\n    image: x\n    depends_on: [b]\n  b:\n    image: x\n    x-bay: {job: true}\n",
		"routed job":       "services:\n  a:\n    image: x\n    expose: [\"80\"]\n    x-bay: {job: true, domains: [a.test]}\n",
		"schedule no job":  "services:\n  a:\n    image: x\n    x-bay: {schedule: \"@daily\"}\n",
		"bad schedule":     "services:\n  a:\n    image: x\n    x-bay: {job: true, schedule: \"61 * * * *\"}\n",
		"long timeout":     "services:\n  a:\n    image: x\n    x-bay: {job: true, timeout: 25h}\n",
	} {
		if _, err := Load(writeCompose(t, body)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestQueueRunsOneJobAtATimeInOrder(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(writeCompose(t, "name: q\nservices:\n  a:\n    image: x\n    x-bay: {job: true}\n  b:\n    image: x\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := QueueJob(dir, cfg, "b", "request", ""); err == nil {
		t.Fatal("a service that is not a job was queued")
	}
	first, _, err := QueueJob(dir, cfg, "a", "request", "")
	if err != nil {
		t.Fatal(err)
	}
	second, _, _ := QueueJob(dir, cfg, "a", "request", "")
	if _, _, err := QueueJob(dir, cfg, "a", "schedule", ""); !errors.Is(err, ErrScheduledPending) {
		t.Fatalf("a schedule while its job waits: %v", err)
	}
	got, ok, err := NextJob(dir)
	if err != nil || !ok || got.ID != first.ID || got.State != JobRunning {
		t.Fatalf("next: %+v %v %v", got, ok, err)
	}
	if _, ok, _ := NextJob(dir); ok {
		t.Fatal("a second job started while one runs")
	}
	exit := 3
	done, err := FinishJob(dir, first.ID, JobFailed, &exit, "", 42<<20)
	if err != nil || !done.Done() || *done.ExitCode != 3 || done.PeakMemory != 42<<20 {
		t.Fatalf("finish: %+v %v", done, err)
	}
	got, ok, _ = NextJob(dir)
	if !ok || got.ID != second.ID {
		t.Fatalf("next after the first ended: %+v %v", got, ok)
	}
	if j, err := GetJob(dir, second.ID[len(second.ID)-6:]); err != nil || j.ID != second.ID {
		t.Fatalf("by the end of its ID: %v %v", j.ID, err)
	}
	queued, _, _ := QueueJob(dir, cfg, "a", "request", "")
	if c, err := CancelJob(dir, queued.ID); err != nil || c.State != JobCancelled {
		t.Fatalf("cancel queued: %+v %v", c, err)
	}
	keyed, added, _ := QueueJob(dir, cfg, "a", "request", "abc123")
	again, addedAgain, _ := QueueJob(dir, cfg, "a", "request", "abc123")
	if !added || addedAgain || again.ID != keyed.ID {
		t.Fatalf("a key queued twice: %v %v %s %s", added, addedAgain, keyed.ID, again.ID)
	}
}

func TestSchedulesAreFollowedOncePerProject(t *testing.T) {
	dir := t.TempDir()
	p := ScheduledProject{Project: "ci", File: "/x/compose.yaml"}
	if changed, err := SetSchedule(dir, p, true); err != nil || !changed {
		t.Fatalf("follow: %v %v", changed, err)
	}
	if changed, _ := SetSchedule(dir, p, true); changed {
		t.Fatal("following twice changed the list")
	}
	moved := ScheduledProject{Project: "ci", File: "/y/compose.yaml"}
	if changed, _ := SetSchedule(dir, moved, true); !changed {
		t.Fatal("a moved file was not recorded")
	}
	if list, _ := Schedules(dir); len(list) != 1 || list[0].File != moved.File {
		t.Fatalf("list: %+v", list)
	}
	if changed, _ := SetSchedule(dir, moved, false); !changed {
		t.Fatal("unfollow changed nothing")
	}
	if list, _ := Schedules(dir); len(list) != 0 {
		t.Fatalf("after unfollow: %+v", list)
	}
}

func TestAJobTakesDownWhatAnEarlierRunLeft(t *testing.T) {
	cfg, err := Load(writeCompose(t, `name: ci
services:
  db:
    image: postgres:18
    expose: ["5432"]
  check:
    image: debian:trixie
    x-bay: {job: true}
`))
	if err != nil {
		t.Fatal(err)
	}
	in := func(service, state string) Instance {
		return Instance{Name: "ci-" + service, State: state, Labels: map[string]string{LabelService: service}}
	}
	for _, c := range []struct {
		name    string
		before  []Instance
		serving bool
	}{
		{"nothing", nil, false},
		{"a job's container left running", []Instance{in("check", "running")}, false},
		{"a service no longer declared", []Instance{in("build", "running")}, false},
		{"a service left stopped", []Instance{in("db", "exited")}, false},
		{"a service a person keeps up", []Instance{in("build", "exited"), in("db", "running")}, true},
	} {
		if got := servedBeforeJob(cfg, c.before); got != c.serving {
			t.Errorf("%s: served %v, want %v", c.name, got, c.serving)
		}
	}
}
