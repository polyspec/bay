package stack

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLIPrerequisitesIncludeNativeBindings(t *testing.T) {
	makefile, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range []string{"events.c", "events.h"} {
		t.Run(binding, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{"cmd/bay", "cmd/bayd", "cmd/baybar", "cmd/bayinit", "internal/stack/initbin", "bin"} {
				if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, "Makefile"), makefile, 0600); err != nil {
				t.Fatal(err)
			}
			earlier := time.Unix(100, 0)
			for _, name := range []string{"cmd/bay/main.go", "internal/stack/events.c", "internal/stack/events.h",
				"cmd/bayinit/main.go", "internal/stack/initbin/bayinit-linux-arm64", "internal/stack/initbin/bayinit-linux-amd64", "bin/bay"} {
				path := filepath.Join(root, name)
				if err := os.WriteFile(path, []byte("fixture-only"), 0600); err != nil {
					t.Fatal(err)
				}
				stamp := earlier
				switch {
				case name == "bin/bay":
					stamp = earlier.Add(2 * time.Second)
				case strings.HasPrefix(name, "internal/stack/initbin/"):
					stamp = earlier.Add(time.Second)
				}
				if err := os.Chtimes(path, stamp, stamp); err != nil {
					t.Fatal(err)
				}
			}
			run := func() error {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "make", "-q", "bin/bay")
				cmd.Dir = root
				return cmd.Run()
			}
			if err := run(); err != nil {
				t.Fatalf("unchanged fixture is not current: %v", err)
			}
			changed := filepath.Join(root, "internal/stack", binding)
			stamp := earlier.Add(3 * time.Second)
			if err := os.Chtimes(changed, stamp, stamp); err != nil {
				t.Fatal(err)
			}
			err := run()
			if status, ok := err.(*exec.ExitError); !ok || status.ExitCode() != 1 {
				t.Fatalf("changed native binding did not require a rebuild: %v", err)
			}
		})
	}
}
