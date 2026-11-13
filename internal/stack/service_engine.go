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
	"sort"
	"strings"
	"syscall"
	"time"
)

type serviceEngine struct {
	command       func(context.Context, ...string) ([]byte, error)
	subscribeName nameSubscriber
	dir           string
	timeout       time.Duration
	// engine is the engine used to create this project's containers.
	engine string
	// progress reports each step before it begins. A step that can take more
	// than a moment says so first, so a command stopped in one is readable from
	// what it has already printed. It also reports each started container and
	// the result of each healthcheck attempt, so a long startup gate reports
	// its elapsed time.
	progress func(string)
	// noPull refuses to fetch a missing image.
	noPull bool
}

// say reports one progress line.
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
		subscribeName: nativeNameSubscription,
		command: func(ctx context.Context, args ...string) ([]byte, error) {
			return serviceCommand(ctx, engine, args...)
		},
		dir:     dir,
		timeout: 10 * time.Minute,
		engine:  engine,
	}
}

// changesState names the engine commands that make or remove a container,
// network or volume. Ending one of them ends only the client: the engine
// finishes the change after the client is gone, so a container created that
// way appears after the cleanup listed the project's containers and is left
// behind. An interruption therefore refuses such a command before it starts
// and lets one that started finish.
var changesState = map[string]bool{
	"create": true, "rm": true, "start": true, "stop": true, "kill": true,
	"network": true, "volume": true,
}

func serviceCommand(ctx context.Context, engine string, args ...string) ([]byte, error) {
	if changesState[args[0]] {
		if refused(ctx) {
			return nil, fmt.Errorf("%s %s: %w", engineBin(engine), args[0], ErrInterrupted)
		}
		inflight.Add(1)
		defer inflight.Done()
	} else if ctx.Value(cleanupKey{}) == nil {
		var stop context.CancelFunc
		ctx, stop = interruptible(ctx)
		defer stop()
	}
	cmd := exec.CommandContext(ctx, engineBin(engine), args...)
	begun := time.Now()
	if args[0] == "create" {
		var diagnostic creationDiagnostic
		cmd.Stdout, cmd.Stderr = io.Discard, &diagnostic
		if err := cmd.Run(); err != nil {
			if ended := endedByContext(ctx, begun, args); ended != nil {
				return nil, ended
			}
			return nil, fmt.Errorf("%s", creationFailure(diagnostic.text.String(), args))
		}
		return nil, nil
	}
	// Process output belongs to explicit logs, never startup errors or completion state.
	if args[0] == "start" || args[0] == "exec" {
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
		err := cmd.Run()
		if err != nil {
			if ended := endedByContext(ctx, begun, args); ended != nil {
				return nil, ended
			}
			return nil, fmt.Errorf("container %s failed: %w", args[0], err)
		}
		return nil, nil
	}
	out, err := cmd.Output()
	if err != nil {
		if ended := endedByContext(ctx, begun, args); ended != nil {
			return nil, ended
		}
		var exited *exec.ExitError
		if errors.As(err, &exited) {
			return nil, fmt.Errorf("container %s: %s", args[0], strings.TrimSpace(string(exited.Stderr)))
		}
	}
	return out, err
}

// endedByContext returns the error of a command that its context ended, or nil
// when the context did not end it. The ended process has no standard error of
// its own, so the error names the command, the context error and the time the
// command ran.
func endedByContext(ctx context.Context, begun time.Time, args []string) error {
	if ctx.Err() == nil {
		return nil
	}
	name := args[0]
	if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
		name += " " + args[1]
	}
	return fmt.Errorf("container %s: %w after %s", name, ctx.Err(), time.Since(begun).Round(time.Millisecond))
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
	if e.engine == DockerEngine {
		return e.listDocker(ctx)
	}
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
		if e.engine == AppleEngine && s.Restart != "" && s.Restart != "no" {
			e.say("warning: %s: restart: %s is not applied; Apple container has no restart policy", s.Name, s.Restart)
		}
		if e.engine == AppleEngine {
			for _, t := range s.Tmpfs {
				if strings.Contains(t, ":") {
					return fmt.Errorf("service %s: tmpfs %q: Apple container takes a tmpfs path without mount options", s.Name, t)
				}
			}
		}
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
		if !imageMissing(err) {
			return "", err
		}
		if e.noPull {
			return "", fmt.Errorf("image %s is not in the local store and -no-pull fetches nothing; run \"bay pull\" first", reference)
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
	if e.engine == DockerEngine {
		return dockerImageID(out, reference)
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

// serviceFingerprint identifies a service's configuration. The arguments that
// give the container the trusted certificates are part of it, so a container
// created before the store was written, or with other such arguments, is
// created again.
func serviceFingerprint(group string, s *Service, digest string, trust []string) string {
	encoded, _ := json.Marshal(struct {
		Group        string
		Service      *Service
		Image        string
		Dependencies map[string]string
		Trust        []string `json:",omitempty"`
	}{group, s, digest, s.dependencyIdentity, trust})
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
	// The name a container is reached at is given when it is created, so a
	// container named in another domain is created again.
	if err := ensureServiceCert(e.dir, s); err != nil {
		return err
	}
	launched := e.launchOf(ctx, s)
	identity := append(trustArguments(e.dir, s), certArguments(e.dir, s)...)
	identity = append(identity, "domain="+BackendDomain)
	// A container created before the slice was registered is created again in it.
	if parent := serviceCgroupParent(); parent != "" {
		identity = append(identity, "cgroup-parent="+parent)
	}
	if encoded, err := json.Marshal(launched); err == nil {
		identity = append(identity, "launch="+string(encoded))
	}
	fingerprint := serviceFingerprint(group, s, digest, identity)
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
		return e.started(ctx, s)
	}
	if found {
		if in.State == "running" {
			if _, err = e.command(ctx, stopArguments(s.ContainerName, s.StopGrace, s.StopSignal)...); err != nil {
				return err
			}
		}
		if _, err = e.command(ctx, "rm", s.ContainerName); err != nil {
			return err
		}
	}
	// Create does not execute the process. Inspect its actual image before
	// starting, since a local tag need not have a name@digest cache alias.
	e.say("%s: creating its container", s.Name)
	if _, err = e.command(ctx, e.createArgumentsWith(group, s, fingerprint, launched)...); err != nil {
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
			// The stop waits for the grace period before it kills the container, so
			// the cleanup deadline includes it.
			wait := 10 * time.Second
			if s.StopGrace != nil {
				wait += time.Duration(*s.StopGrace) * time.Second
			}
			cleanup, stop := context.WithTimeout(context.Background(), wait)
			current, ok, check := e.lookup(cleanup, s.ContainerName)
			if check == nil && ok && owns(group, s, current) == nil && current.Labels[LabelConfig] == fingerprint && current.State == "running" {
				_, check = e.command(cleanup, stopArguments(s.ContainerName, s.StopGrace, s.StopSignal)...)
			}
			stop()
			if check != nil {
				return fmt.Errorf("initializer %s timed out and stopping it failed: %w", s.Name, check)
			}
		}
		return e.failureWithLogOf(fmt.Sprintf("service %s start: %v", s.Name, err), s.ContainerName)
	}
	if s.OneShot {
		after, err := e.waitStopped(ctx, s.ContainerName, created.Created)
		if err != nil {
			return fmt.Errorf("initializer %s: %w", s.Name, err)
		}
		if err := stoppedIdentity(group, s, after, created.Created, fingerprint, digest); err != nil {
			return fmt.Errorf("initializer %s has no stable stopped identity: %w", s.Name, err)
		}
		return e.record(after, fingerprint)
	}
	return e.started(ctx, s)
}

// waitStopped waits for the runtime to report an attached one-shot container
// stopped, because the attached start can return before the runtime reports the
// stopped state. It returns the container as last read, which is a replacement
// when the creation time differs.
func (e *serviceEngine) waitStopped(ctx context.Context, name, created string) (Instance, error) {
	var last Instance
	err := e.waitFor(ctx, name, func(in Instance, ok bool) (bool, error) {
		if !ok {
			return false, fmt.Errorf("container %s is missing", name)
		}
		last = in
		return in.State == "stopped" || in.Created != created, nil
	})
	if err != nil && ctx.Err() != nil {
		return last, fmt.Errorf("container %s state is %q after the attached start returned: %w", name, last.State, err)
	}
	return last, err
}

// stoppedIdentity returns one error for each field of a finished one-shot
// container that differs from the container this command created and started.
func stoppedIdentity(group string, s *Service, in Instance, created, fingerprint, digest string) error {
	var differ []error
	if err := owns(group, s, in); err != nil {
		differ = append(differ, err)
	}
	if in.State != "stopped" {
		differ = append(differ, fmt.Errorf("state is %q, not stopped", in.State))
	}
	if in.Created != created {
		differ = append(differ, fmt.Errorf("creation time is %q, not %q", in.Created, created))
	}
	if in.Started == "" {
		differ = append(differ, errors.New("start time is empty"))
	}
	if in.Labels[LabelConfig] != fingerprint {
		differ = append(differ, fmt.Errorf("configuration label is %q, not %q", in.Labels[LabelConfig], fingerprint))
	}
	if in.ImageDigest != digest {
		differ = append(differ, fmt.Errorf("image digest is %q, not %q", in.ImageDigest, digest))
	}
	return errors.Join(differ...)
}

// started waits for the service's container to run and reports it, so a caller
// that streams progress can attach to the container while a healthcheck waits.
func (e *serviceEngine) started(ctx context.Context, s *Service) error {
	ended, err := e.waitRunning(ctx, s.ContainerName, s.Port == 0)
	if err != nil {
		return err
	}
	if ended {
		e.say("%s: ran and ended", s.Name)
		return nil
	}
	e.say("%s: started container %s", s.Name, s.ContainerName)
	return nil
}

// waitRunning waits for a container to run with an address. A service with no
// port that has already ended is done rather than failed: a job that ends at
// once is what it is for. It reports whether the container had ended.
func (e *serviceEngine) waitRunning(ctx context.Context, name string, mayEnd bool) (bool, error) {
	deadline, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	ended := false
	err := e.waitFor(deadline, name, func(in Instance, ok bool) (bool, error) {
		if ok && in.State == "stopped" && mayEnd {
			ended = true
			return true, nil
		}
		if ok && in.State == "stopped" {
			return false, e.failureWithLogOf(fmt.Sprintf("service %s stopped before becoming ready", name), name)
		}
		return ok && in.State == "running" && in.IPv4 != "", nil
	})
	if err != nil && deadline.Err() != nil {
		return false, fmt.Errorf("service %s did not start: %w", name, err)
	}
	return ended, err
}

// waitFor reads the named container every 100 milliseconds until reached
// returns true or an error. It returns ctx's error when ctx ends first.
func (e *serviceEngine) waitFor(ctx context.Context, name string, reached func(Instance, bool) (bool, error)) error {
	for {
		in, ok, err := e.lookup(ctx, name)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if done, err := reached(in, ok); done || err != nil {
			return err
		}
		if err := sleepContext(ctx, 100*time.Millisecond); err != nil {
			return err
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
			return e.failureWithLogOf(fmt.Sprintf("service %s stopped during healthcheck", s.Name), s.ContainerName)
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
			e.say("%s: healthy after %s", s.Name, time.Since(started).Round(time.Second))
			return nil
		}
		if time.Since(started) >= h.StartPeriod {
			failures++
		}
		e.say("%s: not healthy yet after %s (%d of %d attempts)",
			s.Name, time.Since(started).Round(time.Second), failures, h.Retries)
		if failures >= h.Retries {
			return e.failureWithLogOf(fmt.Sprintf("service %s failed its healthcheck after %d attempts", s.Name, failures), s.ContainerName)
		}
		stopped, err := e.waitWhileRunning(ctx, s.ContainerName, h.Interval)
		if stopped {
			return e.failureWithLogOf(fmt.Sprintf("service %s stopped during healthcheck", s.Name), s.ContainerName)
		}
		if err != nil {
			return e.failureWithLogOf(fmt.Sprintf("service %s healthcheck timed out", s.Name), s.ContainerName)
		}
	}
}

// stateCheck is how often a wait between healthcheck attempts looks at the
// container, so a container that stops fails the wait at once rather than at
// the next attempt, an interval later.
var stateCheck = time.Second

// waitWhileRunning waits for d and reports whether the container stopped
// before it passed.
func (e *serviceEngine) waitWhileRunning(ctx context.Context, name string, d time.Duration) (bool, error) {
	end := time.Now().Add(d)
	for {
		left := time.Until(end)
		if left <= 0 {
			return false, nil
		}
		if err := sleepContext(ctx, min(left, stateCheck)); err != nil {
			return false, err
		}
		in, ok, err := e.lookup(ctx, name)
		if err != nil {
			return false, err
		}
		if !ok || in.State != "running" {
			return true, nil
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
		// A service with no port is not connected to, so its name is not waited
		// for; a job among them may already have ended.
		if !s.OneShot && s.Port > 0 && e.engine == AppleEngine {
			if err = e.waitServiceName(s.ContainerName); err != nil {
				return err
			}
		}
	}
	return nil
}

// projectContainers returns the service containers carrying a project's label,
// most recently started first. What to stop is read from the containers rather
// than from the Compose file: the file says what to start, and a project whose
// file is gone, or a service removed from the file after it started, still has
// containers running under the label.
//
// Services start in dependency order, so the most recently started is stopped
// first and a service stops before the services it depends on. The runtime
// reports start times to the second, so containers started in the same second
// keep no defined order between themselves.
func (e *serviceEngine) projectContainers(ctx context.Context, group string) ([]Instance, error) {
	list, err := e.list(ctx)
	if err != nil {
		return nil, err
	}
	var out []Instance
	for _, in := range list {
		if in.Labels[LabelGroup] == group && in.Labels[LabelRole] == roleService {
			out = append(out, in)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Started > out[j].Started })
	return out, nil
}

// removeProject removes every service container of a project.
func (e *serviceEngine) removeProject(group string) error {
	return e.locked(group, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
		defer cancel()
		containers, err := e.projectContainers(ctx, group)
		if err != nil {
			return err
		}
		return e.removeContainers(ctx, containers)
	})
}

// removeContainers removes the given containers in the order given. Every
// running container stops before it is removed, as Compose does. A recorded
// stop signal is passed as --signal and a recorded grace period as --time;
// without them the runtime defaults apply. A failed stop keeps the container.
func (e *serviceEngine) removeContainers(ctx context.Context, containers []Instance) error {
	for _, in := range containers {
		grace, signal, err := stopOf(in)
		if err != nil {
			return err
		}
		if in.State == "running" {
			e.say("%s: stopping it", in.Labels[LabelService])
			if _, err := e.command(ctx, stopArguments(in.Name, grace, signal)...); err != nil {
				return err
			}
		}
		e.say("%s: removing its container", in.Labels[LabelService])
		if _, err := e.command(ctx, "rm", "--force", in.Name); err != nil {
			return err
		}
	}
	return nil
}

// stopProject stops a project's running service containers, or only the named
// services when names are given, and leaves the containers in place.
func (e *serviceEngine) stopProject(group string, names []string) error {
	return e.locked(group, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
		defer cancel()
		containers, err := e.projectContainers(ctx, group)
		if err != nil {
			return err
		}
		wanted := map[string]bool{}
		for _, n := range names {
			wanted[n] = true
		}
		for _, in := range containers {
			service := in.Labels[LabelService]
			if len(names) > 0 && !wanted[service] {
				continue
			}
			if in.State != "running" {
				continue
			}
			e.say("%s: stopping it", service)
			grace, signal, err := stopOf(in)
			if err != nil {
				return err
			}
			if _, err := e.command(ctx, stopArguments(in.Name, grace, signal)...); err != nil {
				return err
			}
		}
		return nil
	})
}
