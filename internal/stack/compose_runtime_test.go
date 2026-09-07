package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeRuntimeRequiredInterpolation(t *testing.T) {
	_, err := Load(write(t, "services:\n  web:\n    image: nginx\n    user: '${BAY_TEST_MISSING:?required}'\n"))
	if err == nil {
		t.Fatal("missing required runtime user was silently ignored")
	}
}

func TestComposeRuntimeRejectsUnsafeDeclarations(t *testing.T) {
	for _, body := range []string{
		"    depends_on: {missing: {condition: service_started}}\n",
		"    depends_on: {web: {condition: service_started}}\n",
		"    healthcheck: {test: [CMD, true], unsupported: true}\n",
		"    depends_on: {web: {condition: unknown}}\n",
		"    volumes: [undeclared:/data]\n",
	} {
		t.Run(strings.TrimSpace(body), func(t *testing.T) {
			if _, err := Load(write(t, "services:\n  web:\n    image: nginx\n"+body)); err == nil {
				t.Fatal("unsafe declaration silently accepted")
			}
		})
	}
}

func TestComposeRuntimeNormalizesBindSource(t *testing.T) {
	path := write(t, "services:\n  web:\n    image: nginx\n    volumes: ['./config:/run/config:ro']\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(path), "config") + ":/run/config:ro"
	if cfg.Services["web"].Volumes[0] != want {
		t.Fatal("bind source does not use Compose directory")
	}
}

func TestComposeRuntimeEnvironmentPrecedence(t *testing.T) {
	path := write(t, "name: '${CTL_PROJECT}'\nservices:\n  web:\n    image: nginx\n")
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), ".env"), []byte("CTL_PROJECT=from-file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CTL_PROJECT", "from-shell")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "from-shell" {
		t.Fatal("shell environment did not override project .env")
	}
}

func TestComposeRuntimeInterpolationAndExternalVolume(t *testing.T) {
	values := map[string]string{"VALUE": "set", "EMPTY": ""}
	for _, test := range []struct{ in, want string }{
		{"$VALUE/${VALUE}/$$VALUE", "set/set/$VALUE"},
		{"${MISSING:-${VALUE}}", "set"}, {"${EMPTY:-default}", "default"},
		{"${EMPTY-default}", ""}, {"${VALUE:+other}", "other"},
		{"${EMPTY+other}", "other"}, {"${MISSING+other}", ""},
	} {
		got, err := interpolate(test.in, values, 0)
		if err != nil || got != test.want {
			t.Fatalf("interpolation %q=%q err=%v", test.in, got, err)
		}
	}
	for _, input := range []string{"${MISSING?never-echo-this-secret}", "${EMPTY:?never-echo-this-secret}", "${VALUE/unimplemented}", "${VALUE"} {
		_, err := interpolate(input, values, 0)
		if err == nil || strings.Contains(err.Error(), "never-echo") {
			t.Fatal("invalid interpolation accepted or leaked diagnostic content")
		}
	}
	path := write(t, `services:
  db:
    image: postgres
    volumes: [data:/var/lib/postgresql]
volumes:
  data: {external: true, name: existing-postgres}
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Services["db"].Volumes[0] != "existing-postgres:/var/lib/postgresql" || cfg.Services["db"].ExternalVolumes[0] != "existing-postgres" {
		t.Fatal("external volume name changed")
	}
}

func TestComposeRuntimeRejectsUnsupportedDependencyAndHealth(t *testing.T) {
	for _, body := range []string{
		"    depends_on: {db: {condition: service_started, restart: true}}\n",
		"    depends_on: {db: {condition: service_started, required: false}}\n",
		"    depends_on: {db: {condition: service_healthy}}\n",
		"    healthcheck: {test: [CMD, true], timeout: 0s}\n",
		"    healthcheck: {test: [CMD, true], retries: 0}\n",
		"    healthcheck: {test: [CMD-SHELL, true, extra]}\n",
	} {
		if _, err := Load(write(t, "services:\n  web:\n    image: nginx\n"+body+"  db:\n    image: postgres\n")); err == nil {
			t.Fatalf("accepted unsupported lifecycle: %s", body)
		}
	}
}
