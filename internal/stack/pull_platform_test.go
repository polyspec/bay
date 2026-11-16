package stack

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// pullRecorder records each engine command; an image inspect fails as missing
// until the image is pulled.
type pullRecorder struct {
	calls  [][]string
	pulled bool
}

func (p *pullRecorder) command(_ context.Context, args ...string) ([]byte, error) {
	p.calls = append(p.calls, append([]string{}, args...))
	if len(args) > 1 && args[0] == "image" && args[1] == "inspect" && !p.pulled {
		return nil, errors.New("image not found")
	}
	if len(args) > 1 && args[0] == "image" && args[1] == "pull" {
		p.pulled = true
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	return []byte(`[{"Id":"` + digest + `","configuration":{"descriptor":{"digest":"` + digest + `"}}}]`), nil
}

func (p *pullRecorder) pulls() [][]string {
	var out [][]string
	for _, c := range p.calls {
		if len(c) > 1 && c[0] == "image" && c[1] == "pull" {
			out = append(out, c)
		}
	}
	return out
}

func TestPullFetchesOnlyThePlatformItRuns(t *testing.T) {
	host := "linux/" + runtime.GOARCH
	cases := []struct {
		name    string
		engine  string
		service Service
		want    []string
	}{
		{"apple, no platform key", AppleEngine, Service{Image: "postgres:18"},
			[]string{"image", "pull", "--platform", host, "postgres:18"}},
		{"apple, platform key", AppleEngine, Service{Image: "postgres:18", Platform: host + "/v8"},
			[]string{"image", "pull", "--platform", host + "/v8", "postgres:18"}},
		{"docker, no platform key", DockerEngine, Service{Image: "postgres:18"},
			[]string{"image", "pull", "postgres:18"}},
		{"docker, platform key", DockerEngine, Service{Image: "postgres:18", Platform: host},
			[]string{"image", "pull", "--platform", host, "postgres:18"}},
	}
	for _, tc := range cases {
		t.Run(tc.name+", up", func(t *testing.T) {
			p := &pullRecorder{}
			e := &serviceEngine{command: p.command, engine: tc.engine}
			if _, err := e.image(context.Background(), &tc.service); err != nil {
				t.Fatal(err)
			}
			if got := p.pulls(); len(got) != 1 || !reflect.DeepEqual(got[0], tc.want) {
				t.Fatalf("pull commands %q, want one %q", got, tc.want)
			}
		})
		t.Run(tc.name+", pull", func(t *testing.T) {
			p := &pullRecorder{}
			e := &serviceEngine{command: p.command, engine: tc.engine}
			if err := pullImages(e, []*Service{&tc.service, &tc.service}); err != nil {
				t.Fatal(err)
			}
			if got := p.pulls(); len(got) != 1 || !reflect.DeepEqual(got[0], tc.want) {
				t.Fatalf("pull commands %q, want one %q", got, tc.want)
			}
		})
	}
}

func TestPlatformKey(t *testing.T) {
	host := "linux/" + runtime.GOARCH
	for _, value := range []string{host, host + "/v8"} {
		cfg, err := Load(write(t, "name: app\nservices:\n  db:\n    image: postgres:18\n    platform: "+value+"\n"))
		if err != nil {
			t.Fatalf("platform %s: %v", value, err)
		}
		if got := cfg.Services["db"].Platform; got != value {
			t.Errorf("platform %s: loaded %q", value, got)
		}
	}
	other := "linux/amd64"
	if runtime.GOARCH == "amd64" {
		other = "linux/arm64"
	}
	for _, value := range []string{other, "windows/" + runtime.GOARCH, runtime.GOARCH, ""} {
		_, err := Load(write(t, "name: app\nservices:\n  db:\n    image: postgres:18\n    platform: \""+value+"\"\n"))
		if err == nil || !strings.Contains(err.Error(), "platform") {
			t.Errorf("platform %q: error %v, want a rejection that names platform", value, err)
		}
	}
}
