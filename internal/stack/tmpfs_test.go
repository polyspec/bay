package stack

import (
	"context"
	"strings"
	"testing"
)

func TestTmpfsIsMountedAsTheFileSays(t *testing.T) {
	cfg, err := Load(write(t, "services:\n  db:\n    image: postgres\n    tmpfs:\n      - /var/lib/postgresql/data\n      - /run:size=64m\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Services["db"]
	args := strings.Join(serviceArguments(cfg.Name, s, ""), " ")
	for _, want := range []string{"--tmpfs /var/lib/postgresql/data", "--tmpfs /run:size=64m"} {
		if !strings.Contains(args, want) {
			t.Errorf("arguments lack %q: %s", want, args)
		}
	}
	one, err := Load(write(t, "services:\n  db:\n    image: postgres\n    tmpfs: /data\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := one.Services["db"].Tmpfs; len(got) != 1 || got[0] != "/data" {
		t.Errorf("a single path read as %v", got)
	}
	if _, err := Load(write(t, "services:\n  db:\n    image: postgres\n    tmpfs: data\n")); err == nil {
		t.Error("a relative tmpfs path was accepted")
	}
}

func TestAppleRejectsTmpfsOptionsByName(t *testing.T) {
	e := &serviceEngine{engine: AppleEngine, timeout: 1}
	e.command = func(_ context.Context, _ ...string) ([]byte, error) { return []byte("[]"), nil }
	err := e.preflight("p", []*Service{{Name: "db", ContainerName: "p-db", Tmpfs: []string{"/run:size=64m"}}})
	if err == nil || !strings.Contains(err.Error(), "tmpfs") {
		t.Fatalf("err = %v, want the tmpfs options named", err)
	}
}
