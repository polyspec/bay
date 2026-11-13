package stack

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// JobOutcome is how a job's container ended.
type JobOutcome struct {
	ExitCode   int
	PeakMemory uint64
}

// RunJob runs one job service of a project to completion: it brings the
// project's other services up as up does, runs the job's container with its
// output written to out, removes the container, and takes the project down
// again when it was not running before. Ending ctx stops the container, waiting
// its stop grace period, and still takes the project down.
func (r *Runtime) RunJob(ctx context.Context, cfg *Config, name string, out io.Writer) (JobOutcome, error) {
	s, ok := cfg.Services[strings.ToLower(name)]
	if !ok || !s.Job {
		return JobOutcome{}, fmt.Errorf("project %s has no job %q", cfg.Name, name)
	}
	before, err := r.ProjectContainers(cfg.Name)
	if err != nil {
		return JobOutcome{}, err
	}
	wasRunning := len(before) > 0
	defer func() {
		if !wasRunning {
			if _, err := r.Down(cfg.Name); err != nil {
				fmt.Fprintf(out, "bay: taking %s down after the job: %v\n", cfg.Name, err)
			}
		}
	}()
	if _, err := r.Up(cfg); err != nil {
		return JobOutcome{}, err
	}
	if err := ctx.Err(); err != nil {
		return JobOutcome{}, err
	}
	return r.engine().runJob(ctx, cfg.Name, s, out)
}

func (e *serviceEngine) runJob(ctx context.Context, group string, s *Service, out io.Writer) (JobOutcome, error) {
	prep, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()
	if err := e.removeJobContainer(prep, group, s); err != nil {
		return JobOutcome{}, err
	}
	_, fingerprint, launched, err := e.identity(prep, group, s)
	if err != nil {
		return JobOutcome{}, err
	}
	e.say("%s: creating its container", s.Name)
	if _, err := e.command(prep, e.createArgumentsWith(group, s, fingerprint, launched)...); err != nil {
		return JobOutcome{}, fmt.Errorf("job %s create: %w", s.Name, err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		if err := e.removeJobContainer(cleanup, group, s); err != nil {
			fmt.Fprintf(out, "bay: removing the job's container: %v\n", err)
		}
	}()

	e.say("%s: running", s.Name)
	attach := exec.Command(engineBin(e.engine), "start", "--attach", s.ContainerName)
	attach.Stdout, attach.Stderr = out, out
	if err := attach.Start(); err != nil {
		return JobOutcome{}, fmt.Errorf("job %s start: %w", s.Name, err)
	}
	ended := make(chan error, 1)
	go func() { ended <- attach.Wait() }()
	peak := newPeakReader(e.engine, s.ContainerName)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var stopped error
	for {
		select {
		case err := <-ended:
			peak.read()
			outcome := JobOutcome{PeakMemory: peak.max}
			if err == nil {
				return outcome, stopped
			}
			var exited *exec.ExitError
			if errors.As(err, &exited) && exited.ExitCode() >= 0 {
				outcome.ExitCode = exited.ExitCode()
				return outcome, stopped
			}
			return outcome, fmt.Errorf("job %s: %w", s.Name, err)
		case <-tick.C:
			peak.read()
		case <-ctx.Done():
			// The container is stopped, as a person would stop it, so the job
			// gets its stop signal and grace period; the attached client then
			// returns with the container's status.
			stopped = ctx.Err()
			cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second+graceOf(s))
			if _, err := e.command(cleanup, stopArguments(s.ContainerName, s.StopGrace, s.StopSignal)...); err != nil {
				fmt.Fprintf(out, "bay: stopping the job: %v\n", err)
			}
			stop()
			ctx = context.Background()
		}
	}
}

func graceOf(s *Service) time.Duration {
	if s.StopGrace != nil {
		return time.Duration(*s.StopGrace) * time.Second
	}
	return 10 * time.Second
}

// removeJobContainer removes the job's container when one is left, by an
// earlier run that was stopped hard. A container of that name that another
// project owns is refused, as up refuses it.
func (e *serviceEngine) removeJobContainer(ctx context.Context, group string, s *Service) error {
	in, found, err := e.lookup(ctx, s.ContainerName)
	if err != nil || !found {
		return err
	}
	if err := owns(group, s, in); err != nil {
		return err
	}
	if in.State == "running" {
		if _, err := e.command(ctx, stopArguments(s.ContainerName, s.StopGrace, s.StopSignal)...); err != nil {
			return err
		}
	}
	_, err = e.command(ctx, "rm", s.ContainerName)
	return err
}

// peakReader follows the memory a Docker container uses on this machine, from
// its control group. Where the engine runs containers in a virtual machine,
// there is nothing on this machine to read, and the peak stays zero.
type peakReader struct {
	engine, name string
	path         string
	max          uint64
}

func newPeakReader(engine, name string) *peakReader {
	return &peakReader{engine: engine, name: name}
}

func (p *peakReader) read() {
	if hostSetsUpNames || p.engine != DockerEngine {
		return
	}
	if p.path == "" {
		out, err := runEngineWithin(5*time.Second, DockerEngine, "inspect", "--format", "{{.State.Pid}}", p.name)
		if err != nil {
			return
		}
		pid := strings.TrimSpace(string(out))
		if pid == "" || pid == "0" {
			return
		}
		b, err := os.ReadFile("/proc/" + pid + "/cgroup")
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(b), "\n") {
			if rest, ok := strings.CutPrefix(line, "0::"); ok {
				p.path = filepath.Join("/sys/fs/cgroup", rest)
			}
		}
		if p.path == "" {
			return
		}
	}
	for _, name := range []string{"memory.peak", "memory.current"} {
		b, err := os.ReadFile(filepath.Join(p.path, name))
		if err != nil {
			continue
		}
		if v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64); err == nil && v > p.max {
			p.max = v
		}
	}
}
