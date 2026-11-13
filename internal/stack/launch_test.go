package stack

import (
	"context"
	"strings"
	"testing"
)

func TestBayinitRunsTheImagesProcess(t *testing.T) {
	defer func(f func() ([]byte, bool)) { initBinary = f }(initBinary)
	initBinary = func() ([]byte, bool) { return []byte("bin"), true }
	dir := t.TempDir()
	if err := ensureInit(dir); err != nil {
		t.Fatal(err)
	}
	e := &serviceEngine{engine: DockerEngine, dir: dir}
	e.command = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte(`{"Entrypoint":["docker-entrypoint.sh"],"Cmd":["postgres"]}`), nil
	}
	cases := []struct {
		name string
		s    Service
		want string
	}{
		{"image", Service{Trust: true}, "--entrypoint /etc/bay/init img docker-entrypoint.sh postgres"},
		{"command", Service{Trust: true, Command: []string{"postgres", "-c", "ssl=on"}},
			"--entrypoint /etc/bay/init img docker-entrypoint.sh postgres -c ssl=on"},
		{"entrypoint", Service{Trust: true, EntrypointSet: true, Entrypoint: []string{"sh", "-c"}, Command: []string{"echo hi"}},
			"--entrypoint /etc/bay/init img sh -c echo hi"},
		{"entrypoint without command", Service{Trust: true, EntrypointSet: true, Entrypoint: []string{"run"}},
			"--entrypoint /etc/bay/init img run"},
		{"no trust", Service{Command: []string{"postgres"}}, "img postgres"},
	}
	for _, c := range cases {
		s := c.s
		s.Image = "img"
		l := e.launchOf(context.Background(), &s)
		if got := strings.Join(l.tail("img"), " "); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
		args := strings.Join(e.createArgumentsWith("p", &s, "f", l), " ")
		if mounted := strings.Contains(args, InitPath(dir)+":/etc/bay/init:ro"); mounted != l.Init {
			t.Errorf("%s: bayinit mounted %v, used %v", c.name, mounted, l.Init)
		}
	}
}

func TestRestartFollowsTheFile(t *testing.T) {
	cases := []struct {
		s    Service
		want string
	}{
		{Service{Port: 80}, "unless-stopped"},
		{Service{}, ""},
		{Service{Port: 80, OneShot: true}, ""},
		{Service{Restart: "no", Port: 80}, ""},
		{Service{Restart: "on-failure:3"}, "on-failure:3"},
		{Service{Restart: "always"}, "always"},
	}
	for _, c := range cases {
		if got := restartPolicy(&c.s); got != c.want {
			t.Errorf("%+v: %q, want %q", c.s, got, c.want)
		}
	}
	if _, err := decodeRestart("sometimes"); err == nil {
		t.Error("an unknown restart policy was accepted")
	}
}

func TestACommandStringSplitsAsAShellWould(t *testing.T) {
	got, err := splitShell(`sh -c 'echo "a b"; exit 3' x\ y`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"sh", "-c", `echo "a b"; exit 3`, "x y"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%q, want %q", got, want)
	}
}
