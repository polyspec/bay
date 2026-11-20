package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A project names the engine that creates its containers. A named engine that
// is on this machine is returned as named; one that is not is an error, because
// creating the containers on another engine would move the project without the
// file saying so.
func TestAProjectNamesItsEngine(t *testing.T) {
	dir := t.TempDir()
	docker := filepath.Join(dir, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_BIN", docker)
	if got, err := ServiceEngineOn(DockerEngine); err != nil || got != DockerEngine {
		t.Fatalf("a named engine present returned %q, %v; want %s", got, err, DockerEngine)
	}

	// A project that names no engine runs on the first engine present.
	t.Setenv("CONTAINER_BIN", filepath.Join(dir, "container"))
	os.WriteFile(filepath.Join(dir, "container"), []byte("#!/bin/sh\n"), 0o700)
	if got, err := ServiceEngineOn(""); err != nil || got != AppleEngine {
		t.Fatalf("an unnamed engine returned %q, %v; want the first present", got, err)
	}

	// A named engine that is not on this machine is an error naming it.
	t.Setenv("DOCKER_BIN", "")
	t.Setenv("CONTAINER_BIN", "")
	t.Setenv("PATH", launchedPath)
	old := installedDirs
	installedDirs = nil
	t.Cleanup(func() { installedDirs = old })
	_, err := ServiceEngineOn(DockerEngine)
	if err == nil || !strings.Contains(err.Error(), "docker is not on this machine") {
		t.Fatalf("a missing engine returned %v; want an error naming it", err)
	}
}

// The engine key is read from the Compose file and refuses a value that names
// no engine.
func TestTheEngineKeyIsReadAndRefusesAnUnknownEngine(t *testing.T) {
	cfg, err := Load(write(t, "x-bay:\n  engine: docker\nservices:\n  web:\n    image: nginx\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Engine != DockerEngine {
		t.Errorf("engine is %q, want %q", cfg.Engine, DockerEngine)
	}
	_, err = Load(write(t, "x-bay:\n  engine: podman\nservices:\n  web:\n    image: nginx\n"))
	if err == nil || !strings.Contains(err.Error(), "x-bay.engine") {
		t.Fatalf("an unknown engine returned %v; want an error naming the key", err)
	}
}

// engineScript writes a program that answers one engine's list commands and
// logs every command it is given. It stands in for an engine command in tests
// that read or remove containers through the engine itself.
func engineScript(t *testing.T, name, log string, list, inspect string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, name)
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> '" + log + "'\n" +
		"if [ \"$1\" = ls ] || { [ \"$1\" = container ] && [ \"$2\" = ls ]; }; then printf '%s' '" + list + "'; exit 0; fi\n" +
		"if [ \"$1\" = inspect ]; then printf '%s' '" + inspect + "'; exit 0; fi\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return bin
}

const dockerInspectLabeled = `[{
  "Id": "abc1", "Created": "2026-11-20T09:00:00Z", "Name": "/proj-web",
  "Image": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
  "State": {"Status": "running", "StartedAt": "2026-11-20T09:00:01Z"},
  "Config": {"Labels": {"bay.role": "service", "bay.group": "proj", "bay.service": "web"}},
  "NetworkSettings": {"Networks": {"bay": {"IPAddress": "172.20.0.3", "Gateway": "172.20.0.1"}}}
}]`

const appleListLabeled = `[{"configuration":{"id":"proj-db","labels":{"bay.role":"service","bay.group":"proj","bay.service":"db"},
  "image":{"descriptor":{"digest":"sha256:abc"}}},
  "status":{"state":"running","startedDate":"2026-11-20T09:00:01Z","networks":[{"ipv4Address":"192.168.64.9/24","ipv4Gateway":"192.168.64.1"}]}}]`

// scriptBothEngines stands in for both engine commands, each answering one
// container of one project, and returns each engine's command log, Docker's
// first. Each engine's command is named by its own variable; the other
// engine's variable is left empty so the engine it names is absent.
func scriptBothEngines(t *testing.T) []string {
	t.Helper()
	var logs []string
	for _, engine := range []string{DockerEngine, AppleEngine} {
		logs = append(logs, filepath.Join(t.TempDir(), engine+".log"))
	}
	vars := map[string]string{DockerEngine: "DOCKER_BIN", AppleEngine: "CONTAINER_BIN"}
	for i, engine := range []string{DockerEngine, AppleEngine} {
		list, inspect := appleListLabeled, ""
		if engine == DockerEngine {
			list, inspect = "abc1\n", dockerInspectLabeled
		}
		t.Setenv(vars[engine], engineScript(t, engine, logs[i], list, inspect))
	}
	return logs
}

// A project that switched engines has the containers on the engine it left
// removed, and the containers on the engine it runs on left alone. A domain is
// claimed once per machine, so a container left on the old engine holds the
// project's domains and keeps its routes published there.
func TestAProjectThatSwitchesEnginesLosesItsOldContainers(t *testing.T) {
	logs := scriptBothEngines(t)

	rt := &Runtime{Machine: &Machine{}}
	if err := rt.takeDownElsewhere("proj", AppleEngine); err != nil {
		t.Fatal(err)
	}
	dockerLog, _ := os.ReadFile(logs[0])
	if got := string(dockerLog); !strings.Contains(got, "stop proj-web") || !strings.Contains(got, "rm --force proj-web") {
		t.Errorf("the docker engine was asked %q; want the old engine's container stopped and removed", got)
	}
	appleLog, _ := os.ReadFile(logs[1])
	if got := strings.TrimSpace(string(appleLog)); got != "" {
		t.Errorf("the apple engine was asked %q; want nothing on the engine the project runs on", got)
	}
}

// What a project has is read from every engine, so a cleanup after an
// interrupted command sees containers a project left on another engine too.
func TestAProjectsContainersAreReadFromEveryEngine(t *testing.T) {
	scriptBothEngines(t)

	rt := &Runtime{Machine: &Machine{}}
	have, err := rt.ProjectContainers("proj")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"proj-web", "proj-db"} {
		if _, ok := have[name]; !ok {
			t.Errorf("the merged list lacks %s: %v", name, have)
		}
	}
}
