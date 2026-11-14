package stack

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// TestMain lets the test binary act as `bay logs -f`: with LOGS_FOLLOW_HELPER set it runs the log command with follow.
func TestMain(m *testing.M) {
	if os.Getenv("LOGS_FOLLOW_HELPER") == "1" {
		cmd := exec.Command(os.Getenv("CONTAINER_BIN"))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stdout
		_ = runLogs(cmd, true)
		os.Exit(0)
	}
	// With DOCKER_EVENTS_HELPER set the test binary acts as `docker events`:
	// it records its arguments and prints three events.
	if path := os.Getenv("DOCKER_EVENTS_HELPER"); path != "" {
		os.WriteFile(path, []byte(strings.Join(os.Args[1:], " ")), 0o644)
		fmt.Println("container start")
		fmt.Println("container die")
		fmt.Println("network connect")
		os.Exit(0)
	}
	// No test writes the agent skill into the home directory of whoever runs
	// the tests.
	home, err := os.MkdirTemp("", "bay-home")
	if err != nil {
		panic(err)
	}
	agentHome = func() (string, error) { return home, nil }
	claudeBinary = func() (string, bool) { return "", false }
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

// A follower of Logs ends when bay receives SIGTERM, instead of staying as an orphan of process 1.
func TestLogsFollowerEndsWithBay(t *testing.T) {
	fake := t.TempDir() + "/container"
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho $$\nexec sleep 3600\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "LOGS_FOLLOW_HELPER=1", "CONTAINER_BIN="+fake)
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 32)
	n, _ := out.Read(buf)
	pid, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	if err != nil {
		t.Fatalf("the follower wrote no process: %q", buf[:n])
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	_, _ = io.Copy(io.Discard, out)
	_ = cmd.Wait()
	// The copy above ends when every holder of the pipe, this process and its follower, has closed it.
	if syscall.Kill(pid, 0) == nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("the follower %d still runs after bay ended", pid)
	}
}
