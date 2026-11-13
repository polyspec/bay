package stack

import (
	"context"
	"errors"
	"testing"
	"time"
)

// An interrupted up must not start a new container, must let one being created
// finish so the cleanup lists it, and the cleanup's own commands must run.
func TestAnInterruptionLetsStartedChangesFinishAndRefusesNewOnes(t *testing.T) {
	defer Resume()
	ctx := context.Background()
	if refused(ctx) {
		t.Fatal("refused before any interruption")
	}
	inflight.Add(1)
	Interrupt()
	if !refused(ctx) {
		t.Fatal("a new change was not refused after the interruption")
	}
	if refused(forCleanup(ctx)) {
		t.Fatal("the cleanup's command was refused")
	}
	if Settle(50 * time.Millisecond) {
		t.Fatal("Settle returned while a change was still running")
	}
	go func() { time.Sleep(20 * time.Millisecond); inflight.Done() }()
	if !Settle(time.Second) {
		t.Fatal("Settle did not return after the change finished")
	}
}

func TestARefusedChangeNamesTheInterruption(t *testing.T) {
	defer Resume()
	Interrupt()
	_, err := serviceCommand(context.Background(), DockerEngine, "create", "--name", "x", "alpine")
	if err == nil || !errors.Is(err, ErrInterrupted) {
		t.Fatalf("err = %v, want ErrInterrupted", err)
	}
}

// A service with no port, such as one that runs once and exits, is not waited
// for: up used to report it as not accepting connections after the wait.
func TestOnlyServicesWithAPortAreWaitedFor(t *testing.T) {
	got := containerNames([]*Service{
		{ContainerName: "p-web", Port: 80},
		{ContainerName: "p-job"},
		{ContainerName: "p-init", Port: 5432, OneShot: true},
	})
	if len(got) != 1 || got[0] != "p-web" {
		t.Fatalf("waited for %v, want [p-web]", got)
	}
}

// The cleanup lists the project's containers after the interruption; the
// listing used to be ended by the interruption it cleans up after.
func TestTheCleanupsReadsAreNotEnded(t *testing.T) {
	defer Resume()
	Interrupt()
	ctx := forCleanup(context.Background())
	if _, err := serviceCommand(ctx, DockerEngine, "version", "--format", "x"); errors.Is(err, context.Canceled) {
		t.Fatalf("the cleanup's read was ended: %v", err)
	}
}
