package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReloadProxyOnlyExecutesReload(t *testing.T) {
	for _, failed := range []bool{false, true} {
		name := "success"
		if failed {
			name = "original nginx error"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			calls := filepath.Join(dir, "calls")
			binary := filepath.Join(dir, "container")
			const diagnostic = "nginx: [emerg] fixture configuration rejected"
			body := `#!/bin/sh
printf '%s\n' "$*" >> "$BAY_RELOAD_CALLS"
if [ "$1" = exec ] && [ "$BAY_RELOAD_FAIL" = 1 ]; then
    printf '%s\n' 'nginx: [emerg] fixture configuration rejected' >&2
    exit 1
fi
`
			if err := os.WriteFile(binary, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("CONTAINER_BIN", binary)
			t.Setenv("DOCKER_BIN", filepath.Join(dir, "no-docker"))
			t.Setenv("BAY_RELOAD_CALLS", calls)
			t.Setenv("BAY_RELOAD_FAIL", "0")
			if failed {
				t.Setenv("BAY_RELOAD_FAIL", "1")
			}
			err := ReloadProxy(AppleEngine)
			if failed {
				want := "container exec bay-edge nginx -s reload: " + diagnostic
				if err == nil || err.Error() != want {
					t.Errorf("reload did not return the original nginx diagnostic: %v", err)
				}
			} else if err != nil {
				t.Errorf("successful reload failed: %v", err)
			}
			got, err := os.ReadFile(calls)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "exec bay-edge nginx -s reload\n" {
				t.Errorf("reload ran unexpected commands: %q", strings.TrimSpace(string(got)))
			}
		})
	}
}
