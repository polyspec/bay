package stack

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
)

// launch is how a service's process is started: the entrypoint that replaces
// the image's, if any, and the arguments after the image.
type launch struct {
	Entrypoint []string `json:",omitempty"`
	Args       []string `json:",omitempty"`
	// Init says the entrypoint is bayinit and Args are the whole process.
	Init bool `json:",omitempty"`
}

// tail is the entrypoint flag, the image and the arguments.
func (l launch) tail(image string) []string {
	var out []string
	if len(l.Entrypoint) > 0 {
		out = append(out, "--entrypoint", l.Entrypoint[0])
	}
	out = append(out, image)
	return append(out, l.Args...)
}

// fileLaunch starts a service as its Compose file says: an entrypoint set in
// the file replaces the image's, its first word as the entrypoint and the rest
// before the command; an empty one clears it; the command replaces the image's.
func fileLaunch(s *Service) launch {
	if !s.EntrypointSet {
		return launch{Args: s.Command}
	}
	if len(s.Entrypoint) == 0 {
		return launch{Entrypoint: []string{""}, Args: s.Command}
	}
	args := append(append([]string{}, s.Entrypoint[1:]...), s.Command...)
	return launch{Entrypoint: s.Entrypoint[:1], Args: args}
}

// launchOf starts a service that trusts this machine with bayinit in front of
// the process the file and the image name. A service bayinit cannot be put in
// front of, because the image's process cannot be read or there is none,
// starts as its file says.
func (e *serviceEngine) launchOf(ctx context.Context, s *Service) launch {
	l := fileLaunch(s)
	if !s.Trust || !initUsable(e.dir) {
		return l
	}
	entrypoint, command, err := e.imageProcess(ctx, s.Image)
	if err != nil {
		return l
	}
	if s.EntrypointSet {
		entrypoint = s.Entrypoint
		command = nil
	}
	if s.Command != nil {
		command = s.Command
	}
	argv := append(append([]string{}, entrypoint...), command...)
	if len(argv) == 0 {
		return l
	}
	return launch{Entrypoint: []string{initContainerPath}, Args: argv, Init: true}
}

// imageProcess reads the entrypoint and command an image declares.
func (e *serviceEngine) imageProcess(ctx context.Context, ref string) ([]string, []string, error) {
	type process struct {
		Entrypoint []string
		Cmd        []string
	}
	if e.engine == DockerEngine {
		out, err := e.command(ctx, "image", "inspect", "--format", "{{json .Config}}", ref)
		if err != nil {
			return nil, nil, err
		}
		var p process
		if err := json.Unmarshal(out, &p); err != nil {
			return nil, nil, err
		}
		return p.Entrypoint, p.Cmd, nil
	}
	out, err := e.command(ctx, "image", "inspect", ref)
	if err != nil {
		return nil, nil, err
	}
	var images []struct {
		Variants []struct {
			Config struct {
				Config process `json:"config"`
			} `json:"config"`
			Platform struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
			} `json:"platform"`
		} `json:"variants"`
	}
	if err := json.Unmarshal(out, &images); err != nil {
		return nil, nil, err
	}
	for _, img := range images {
		for _, v := range img.Variants {
			if v.Platform.OS == "linux" && v.Platform.Architecture == runtime.GOARCH {
				return v.Config.Config.Entrypoint, v.Config.Config.Cmd, nil
			}
		}
	}
	return nil, nil, errNoVariant
}

var errNoVariant = errors.New("the image has no variant for this machine")
