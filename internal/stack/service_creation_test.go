package stack

import (
	"context"
	"strings"
	"testing"
)

func TestChangedImageTagCannotStartUnverifiedContainer(t *testing.T) {
	cfg := lifecycleConfig(t)
	svc := cfg.Services["db"]
	e, fake := fakeEngine(t)
	e.command = func(ctx context.Context, args ...string) ([]byte, error) {
		if args[0] == "create" {
			// The selection was resolved before a concurrent local tag change.
			fake.images[svc.Image] = "sha256:" + strings.Repeat("b", 64)
		}
		return fake.command(ctx, args...)
	}
	if err := e.reconcile(cfg.Name, svc, false); err == nil || !strings.Contains(err.Error(), "identity differs") {
		t.Fatalf("changed image was not rejected before start: %v", err)
	}
	for _, args := range fake.calls {
		if args[0] == "start" {
			t.Fatal("image mismatch executed a process")
		}
	}
}

func TestCreatedContainerMustRetainOwnershipAndConfiguration(t *testing.T) {
	for _, changed := range []string{LabelGroup, LabelService, LabelRole, LabelConfig} {
		t.Run(changed, func(t *testing.T) {
			cfg := lifecycleConfig(t)
			svc := cfg.Services["db"]
			e, fake := fakeEngine(t)
			e.command = func(ctx context.Context, args ...string) ([]byte, error) {
				body, err := fake.command(ctx, args...)
				if args[0] == "create" {
					in := fake.instances[svc.ContainerName]
					in.Labels[changed] = "changed"
					fake.instances[in.Name] = in
				}
				return body, err
			}
			if err := e.reconcile(cfg.Name, svc, false); err == nil {
				t.Fatal("changed created-container ownership or configuration accepted")
			}
			for _, args := range fake.calls {
				if args[0] == "start" {
					t.Fatal("unverified created container executed a process")
				}
			}
		})
	}
}

func TestCreationDiagnosticIsBoundedAndDoesNotExposeValues(t *testing.T) {
	var diagnostic creationDiagnostic
	message := "resource not found: fixture-private-value " + strings.Repeat("x", 10000)
	if n, err := diagnostic.Write([]byte(message)); err != nil || n != len(message) {
		t.Fatalf("diagnostic writer did not consume input: %d %v", n, err)
	}
	if diagnostic.text.Len() != 4096 {
		t.Fatal("creation diagnostic exceeded its bounded storage")
	}
	if got := diagnostic.category(); got != "resource not found" {
		t.Fatalf("creation diagnostic exposed runtime values: %q", got)
	}
}
