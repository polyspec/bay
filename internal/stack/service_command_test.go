package stack

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// TestServiceCommandNamesTheEndOfItsContext ends a command with its context and
// requires the error to name the command, the context error and the elapsed
// time instead of the empty standard error of the ended process.
func TestServiceCommandNamesTheEndOfItsContext(t *testing.T) {
	fake := t.TempDir() + "/container"
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexec sleep 3600\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTAINER_BIN", fake)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := serviceCommand(ctx, AppleEngine, "volume", "create", "project-data")
	if err == nil {
		t.Fatal("an ended command returned no error")
	}
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "container volume create: context deadline exceeded after ") {
		t.Fatalf("error = %q, want the command, the context error and the elapsed time", err)
	}
}
