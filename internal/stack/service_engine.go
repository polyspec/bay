package stack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type serviceEngine struct {
	command func(context.Context, ...string) ([]byte, error)
	dir     string
	timeout time.Duration
	// engine is the engine used to create this project's containers.
	engine string
	// progress reports each step before it begins. A step that can take more
	// than a moment says so first, so a command stopped in one is readable from
	// what it has already printed.
	progress func(string)
}

// say reports one step. It is called before the step runs, never after.
func (e *serviceEngine) say(format string, args ...any) {
	if e.progress != nil {
		e.progress(fmt.Sprintf(format, args...))
	}
}

func newServiceEngine(dir string) *serviceEngine {
	engine := ServiceEngine()
	// The network must exist before the first service is created.
	_ = ensureNetwork(engine)
	return &serviceEngine{
		command: func(ctx context.Context, args ...string) ([]byte, error) {
			return serviceCommand(ctx, engine, args...)
		},
		dir:     dir,
		timeout: 10 * time.Minute,
		engine:  engine,
	}
}

func serviceCommand(ctx context.Context, engine string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, engineBin(engine), args...)
	if args[0] == "create" {
		var diagnostic creationDiagnostic
		cmd.Stdout, cmd.Stderr = io.Discard, &diagnostic
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("%s", creationFailure(diagnostic.text.String(), args))
		}
		return nil, nil
	}
	// Process output belongs to explicit logs, never startup errors or completion state.
	if args[0] == "start" || args[0] == "exec" {
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
		err := cmd.Run()
		if err != nil {
			return nil, fmt.Errorf("container %s failed: %w", args[0], err)
		}
		return nil, nil
	}
	out, err := cmd.Output()
	if err != nil {
		var exited *exec.ExitError
		if errors.As(err, &exited) {
			return nil, fmt.Errorf("container %s: %s", args[0], strings.TrimSpace(string(exited.Stderr)))
		}
	}
	return out, err
}

// creationFailure reports a refused creation: the category, then what the
// runtime said. The runtime names the thing it refused over and the category on
// its own names nothing, so a missing bind source has to arrive as the path
// that is missing rather than as a rejected creation.
//
// Environment values are passed on the command line, so a runtime that echoes
// an argument echoes a value with it. Every pair this command passed is
// redacted first.
func creationFailure(said string, args []string) string {
	d := creationDiagnostic{}
	d.text.WriteString(said)
	category := d.category()
	said = strings.TrimSpace(redactEnvironment(said, args))
	if said == "" {
		return fmt.Sprintf("container create failed (%s)", category)
	}
	return fmt.Sprintf("container create failed (%s): %s", category, said)
}

// redactEnvironment replaces every KEY=VALUE this command passed with the key
// and a placeholder. The pair is redacted rather than the bare value, because a
// value is only ever echoed as part of the argument it came from and a short
// value would otherwise match ordinary words.
func redactEnvironment(said string, args []string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] != "--env" {
			continue
		}
		key, _, found := strings.Cut(args[i+1], "=")
		if !found || key == "" {
			continue
		}
		said = strings.ReplaceAll(said, args[i+1], key+"=<redacted>")
	}
	return said
}

// creationDiagnostic retains a bounded prefix of a refused creation so it can
// be classified and reported.
type creationDiagnostic struct{ text strings.Builder }

func (d *creationDiagnostic) Write(data []byte) (int, error) {
	n := len(data)
	if remaining := 4096 - d.text.Len(); remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		d.text.Write(data)
	}
	return n, nil
}

func (d *creationDiagnostic) category() string {
	message := strings.ToLower(d.text.String())
	for _, candidate := range []struct{ text, category string }{
		{"not found", "resource not found"},
		{"does not exist", "resource not found"},
		{"permission denied", "permission denied"},
		{"no space left", "insufficient storage"},
		{"invalid", "invalid configuration"},
	} {
		if strings.Contains(message, candidate.text) {
			return candidate.category
		}
	}
	return "runtime rejected creation"
}

func (e *serviceEngine) list(ctx context.Context) ([]Instance, error) {
	out, err := e.command(ctx, "ls", "--all", "--format", "json")
	if err != nil {
		return nil, err
	}
	return decodeInstances(out)
}
func (e *serviceEngine) lookup(ctx context.Context, name string) (Instance, bool, error) {
	list, err := e.list(ctx)
	if err != nil {
		return Instance{}, false, err
	}
	for _, in := range list {
		if in.Name == name {
			return in, true, nil
		}
	}
	return Instance{}, false, nil
}
func owns(group string, s *Service, in Instance) error {
	if in.Labels[LabelRole] != roleService || in.Labels[LabelGroup] != group || in.Labels[LabelService] != s.Name {
		return fmt.Errorf("container %q is not owned by project %q service %q", s.ContainerName, group, s.Name)
	}
	return nil
}
func (e *serviceEngine) preflight(group string, services []*Service) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	list, err := e.list(ctx)
	if err != nil {
		return err
	}
	for _, s := range services {
		for _, in := range list {
			if in.Name == s.ContainerName {
				if err := owns(group, s, in); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (e *serviceEngine) image(ctx context.Context, reference string) (string, error) {
	out, err := e.command(ctx, "image", "inspect", reference)
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "not found") {
			return "", err
		}
		e.say("pulling %s", reference)
		if _, err = e.command(ctx, "image", "pull", reference); err != nil {
			return "", err
		}
		out, err = e.command(ctx, "image", "inspect", reference)
	}
	if err != nil {
		return "", err
	}
	var images []struct {
		Configuration struct {
			Descriptor struct {
				Digest string `json:"digest"`
			} `json:"descriptor"`
		} `json:"configuration"`
	}
	if err = json.Unmarshal(out, &images); err != nil {
		return "", err
	}
	if len(images) != 1 || !strings.HasPrefix(images[0].Configuration.Descriptor.Digest, "sha256:") {
		return "", fmt.Errorf("image %s has no resolved digest", reference)
	}
	return images[0].Configuration.Descriptor.Digest, nil
}
func serviceFingerprint(group string, s *Service, digest string) string {
	encoded, _ := json.Marshal(struct {
		Group        string
		Service      *Service
		Image        string
		Dependencies map[string]string
	}{group, s, digest, s.dependencyIdentity})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func (e *serviceEngine) reconcile(group string, s *Service, restart bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()
	in, found, err := e.lookup(ctx, s.ContainerName)
	if err != nil {
		return err
	}
	if found {
		if err := owns(group, s, in); err != nil {
			return err
		}
		if in.Labels[LabelConfig] == "" && !restart {
			return fmt.Errorf("service %s has no recorded configuration identity; explicitly restart it", s.Name)
		}
	}
	if s.OneShot && e.dir == "" {
		return fmt.Errorf("initializer %s requires machine completion state", s.Name)
	}
	e.say("%s: reading %s", s.Name, s.Image)
	digest, err := e.image(ctx, s.Image)
	if err != nil {
		return err
	}
	fingerprint := serviceFingerprint(group, s, digest)
	same := found && in.Labels[LabelConfig] == fingerprint && in.ImageDigest == digest
	if same && !restart {
		if s.OneShot {
			if in.State == "stopped" && e.completed(in, fingerprint) {
				return nil
			}
			return fmt.Errorf("initializer %s has no matching successful completion; explicitly restart it", s.Name)
		}
		if in.State == "running" {
			return nil
		}
		e.say("%s: starting it", s.Name)
		if _, err = e.command(ctx, "start", s.ContainerName); err != nil {
			return err
		}
		return e.waitRunning(ctx, s.ContainerName)
	}
	if found {
		if _, err = e.command(ctx, "rm", "--force", s.ContainerName); err != nil {
			return err
		}
	}
	// Create does not execute the process. Inspect its actual image before
	// starting, since a local tag need not have a name@digest cache alias.
	e.say("%s: creating its container", s.Name)
	if _, err = e.command(ctx, serviceArguments(group, s, fingerprint)...); err != nil {
		return fmt.Errorf("service %s create: %w", s.Name, err)
	}
	created, ok, err := e.lookup(ctx, s.ContainerName)
	if err != nil {
		return fmt.Errorf("service %s inspect created container: %w", s.Name, err)
	}
	if !ok || owns(group, s, created) != nil || created.State != "stopped" || created.Created == "" || created.Labels[LabelConfig] != fingerprint || created.ImageDigest != digest {
		return fmt.Errorf("service %s created container identity differs; process was not started", s.Name)
	}
	start := []string{"start"}
	if s.OneShot {
		start = append(start, "--attach")
	}
	start = append(start, s.ContainerName)
	if _, err = e.command(ctx, start...); err != nil {
		if s.OneShot && ctx.Err() != nil {
			cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
			current, ok, check := e.lookup(cleanup, s.ContainerName)
			if check == nil && ok && owns(group, s, current) == nil && current.Labels[LabelConfig] == fingerprint && current.State == "running" {
				_, check = e.command(cleanup, "stop", s.ContainerName)
			}
			stop()
			if check != nil {
				return fmt.Errorf("initializer %s timed out and stopping it failed: %w", s.Name, check)
			}
		}
		return failureWithLog(fmt.Sprintf("service %s start: %v", s.Name, err),
			e.serviceLog(s.ContainerName))
	}
	if s.OneShot {
		after, ok, err := e.lookup(ctx, s.ContainerName)
		if err != nil {
			return err
		}
		if !ok || owns(group, s, after) != nil || after.State != "stopped" || after.Created != created.Created || after.Started == "" || after.Labels[LabelConfig] != fingerprint || after.ImageDigest != digest {
			return fmt.Errorf("initializer %s has no stable stopped identity", s.Name)
		}
		return e.record(after, fingerprint)
	}
	return e.waitRunning(ctx, s.ContainerName)
}

func (e *serviceEngine) waitRunning(ctx context.Context, name string) error {
	deadline, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	for {
		in, ok, err := e.lookup(deadline, name)
		if err != nil {
			return err
		}
		if ok && in.State == "running" && in.IPv4 != "" {
			return nil
		}
		if ok && in.State == "stopped" {
			return failureWithLog(fmt.Sprintf("service %s stopped before becoming ready", name),
				e.serviceLog(name))
		}
		if err := sleepContext(deadline, 100*time.Millisecond); err != nil {
			return fmt.Errorf("service %s did not start: %w", name, err)
		}
	}
}
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type completion struct{ Name, Created, Started, Fingerprint string }

func (e *serviceEngine) completionPath(name string) string {
	sum := sha256.Sum256([]byte(name))
	return filepath.Join(e.dir, "completions", hex.EncodeToString(sum[:])+".json")
}
func (e *serviceEngine) completed(in Instance, fingerprint string) bool {
	if e.dir == "" {
		return false
	}
	data, err := os.ReadFile(e.completionPath(in.Name))
	if err != nil {
		return false
	}
	var saved completion
	if json.Unmarshal(data, &saved) != nil {
		return false
	}
	return saved == (completion{in.Name, in.Created, in.Started, fingerprint})
}
func (e *serviceEngine) record(in Instance, fingerprint string) error {
	target := e.completionPath(in.Name)
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(completion{in.Name, in.Created, in.Started, fingerprint})
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(target), "completion-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), target)
}

func (e *serviceEngine) healthy(s *Service) error {
	h := s.Healthcheck
	if h == nil {
		return fmt.Errorf("service %s has no healthcheck", s.Name)
	}
	bound := h.StartPeriod + time.Duration(h.Retries)*(h.Interval+h.Timeout) + time.Second
	e.say("%s: waiting for its healthcheck, up to %s", s.Name, bound)
	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()
	started := time.Now()
	failures := 0
	for {
		in, ok, err := e.lookup(ctx, s.ContainerName)
		if err != nil {
			return err
		}
		if !ok || in.State != "running" {
			return failureWithLog(fmt.Sprintf("service %s stopped during healthcheck", s.Name),
				e.serviceLog(s.ContainerName))
		}
		args := []string{"exec", s.ContainerName}
		if h.Test[0] == "CMD-SHELL" {
			args = append(args, "/bin/sh", "-c", h.Test[1])
		} else {
			args = append(args, h.Test[1:]...)
		}
		probe, stop := context.WithTimeout(ctx, h.Timeout)
		_, err = e.command(probe, args...)
		stop()
		if err == nil {
			return nil
		}
		if time.Since(started) >= h.StartPeriod {
			failures++
		}
		if failures >= h.Retries {
			return failureWithLog(fmt.Sprintf("service %s failed its healthcheck after %d attempts", s.Name, failures),
				e.serviceLog(s.ContainerName))
		}
		if err = sleepContext(ctx, h.Interval); err != nil {
			return failureWithLog(fmt.Sprintf("service %s healthcheck timed out", s.Name),
				e.serviceLog(s.ContainerName))
		}
	}
}

func (e *serviceEngine) start(cfg *Config, targets []*Service, restart bool) error {
	return e.locked(cfg.Name, func() error { return e.startLocked(cfg, targets, restart) })
}

func (e *serviceEngine) locked(group string, fn func() error) error {
	if e.dir == "" {
		return fn()
	}
	dir := filepath.Join(e.dir, "locks")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(group))
	file, err := os.OpenFile(filepath.Join(dir, hex.EncodeToString(sum[:])+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return err
		}
		if err = sleepContext(ctx, 20*time.Millisecond); err != nil {
			return fmt.Errorf("project %s lifecycle lock: %w", group, err)
		}
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	return fn()
}

func (e *serviceEngine) startLocked(cfg *Config, targets []*Service, restart bool) error {
	ordered, err := dependencyOrder(cfg, targets)
	if err != nil {
		return err
	}
	if err = e.preflight(cfg.Name, ordered); err != nil {
		return err
	}
	if err = e.prepareVolumes(cfg.Name, ordered, true); err != nil {
		return err
	}
	selected := map[string]bool{}
	for _, s := range targets {
		selected[s.Name] = true
	}
	for _, s := range ordered {
		if s.OneShot {
			copy := *s
			copy.dependencyIdentity = map[string]string{}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			for name := range s.DependsOn {
				in, ok, lookupErr := e.lookup(ctx, cfg.Services[name].ContainerName)
				if lookupErr != nil || !ok {
					cancel()
					return fmt.Errorf("initializer %s dependency %s is unavailable", s.Name, name)
				}
				copy.dependencyIdentity[name] = in.Labels[LabelConfig] + ":" + in.ImageDigest + ":" + in.Created
			}
			cancel()
			s = &copy
		}
		if err = e.reconcile(cfg.Name, s, restart && selected[s.Name]); err != nil {
			return err
		}
		if s.Healthcheck != nil {
			if err = e.healthy(s); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *serviceEngine) stop(cfg *Config, targets []*Service, remove bool) error {
	return e.locked(cfg.Name, func() error {
		if err := e.preflight(cfg.Name, targets); err != nil {
			return err
		}
		order, err := dependencyOrder(cfg, targets)
		if err != nil {
			return err
		}
		selected := map[string]bool{}
		for _, s := range targets {
			selected[s.Name] = true
		}
		ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
		defer cancel()
		for i := len(order) - 1; i >= 0; i-- {
			s := order[i]
			if !selected[s.Name] {
				continue
			}
			in, ok, err := e.lookup(ctx, s.ContainerName)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			if err = owns(cfg.Name, s, in); err != nil {
				return err
			}
			if remove {
				_, err = e.command(ctx, "rm", "--force", s.ContainerName)
			} else if in.State == "running" {
				_, err = e.command(ctx, "stop", s.ContainerName)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}
