package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/polyspec/bay/internal/stack"
)

// The agent runs the machine's job queue: one job at a time, oldest first, each
// in a bay process of its own whose output is the job's log. It queues the jobs
// of the schedules it follows, tells approved peers about its jobs over the
// link, and on macOS says in a notification when a job here or on an approved
// peer ends.

const (
	jobPoll       = 2 * time.Second
	jobReportPoll = 5 * time.Second
	peerJobsPoll  = 15 * time.Second
	// jobOverrun is how long past its limit a job's process may take to stop
	// the job and record it before the agent ends the process.
	jobOverrun = 5 * time.Minute
)

func serveJobs(ctx context.Context, dir string) {
	r := &jobRunner{dir: dir}
	go r.report(ctx)
	go r.followPeers(ctx)
	r.recover()
	t := time.NewTicker(jobPoll)
	defer t.Stop()
	lastMinute := time.Now().Truncate(time.Minute)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if minute := time.Now().Truncate(time.Minute); minute.After(lastMinute) {
			lastMinute = minute
			r.queueScheduled(minute)
		}
		if !r.busy.Load() {
			r.startNext()
		}
	}
}

type jobRunner struct {
	dir  string
	busy atomic.Bool
}

// recover takes over the job a previous agent started. A job runs in a process
// of its own, which outlives the agent: an update or a restart of the agent
// leaves it running, and this agent waits for it as the previous one would
// have. A job whose process is gone without a result is recorded failed.
func (r *jobRunner) recover() {
	jobs, err := stack.Jobs(r.dir)
	if err != nil {
		log.Printf("jobs: %v", err)
		return
	}
	for _, j := range jobs {
		if j.State != stack.JobRunning {
			continue
		}
		if j.PID > 0 && processRunsJob(j.PID, j.ID) {
			log.Printf("job %s: taking over its run, process %d", j.ID, j.PID)
			r.busy.Store(true)
			go r.adopt(j)
			continue
		}
		stack.FinishJob(r.dir, j.ID, stack.JobFailed, nil, "its run ended without recording a result", 0)
		log.Printf("job %s: its run ended without a result", j.ID)
	}
}

// adopt waits for a job's process this agent did not start.
func (r *jobRunner) adopt(j stack.Job) {
	defer r.busy.Store(false)
	limit := time.Now().Add(j.Timeout + jobOverrun)
	if j.Started != nil {
		limit = j.Started.Add(j.Timeout + jobOverrun)
	}
	for processRunsJob(j.PID, j.ID) {
		if time.Now().After(limit) {
			log.Printf("job %s: its process did not end %s past the job's limit; ending it", j.ID, jobOverrun)
			syscall.Kill(j.PID, syscall.SIGTERM)
			limit = time.Now().Add(2 * time.Minute)
			j.Timeout = 0
		}
		time.Sleep(jobPoll)
	}
	r.ended(j.ID, nil)
}

// processRunsJob reports whether the process is the run of this job, so a
// process number used again by another program is not taken for it.
func processRunsJob(pid int, id string) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	var cmdline string
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil {
		cmdline = strings.ReplaceAll(string(b), "\x00", " ")
	} else if out, err := exec.Command("/bin/ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output(); err == nil {
		cmdline = string(out)
	}
	return strings.Contains(cmdline, "job run "+id)
}

// ended records a job whose run ended without recording a result, says how it
// ended and prunes the queue.
func (r *jobRunner) ended(id string, runErr error) {
	ended, _ := stack.GetJob(r.dir, id)
	if !ended.Done() {
		reason := "the run ended without recording a result"
		if runErr != nil {
			reason += " (" + runErr.Error() + ")"
		}
		ended, _ = stack.FinishJob(r.dir, id, stack.JobFailed, nil, reason, 0)
	}
	log.Printf("job %s: %s in %s", ended.ID, ended.State, ended.Duration())
	notify("", ended)
	if err := stack.PruneJobs(r.dir); err != nil {
		log.Printf("jobs: %v", err)
	}
}

func (r *jobRunner) queueScheduled(minute time.Time) {
	list, err := stack.Schedules(r.dir)
	if err != nil || len(list) == 0 {
		return
	}
	m := stack.NewMachine(r.dir)
	for _, p := range list {
		cfg, err := stack.LoadNamed(m, p.File, p.Project)
		if err != nil {
			log.Printf("schedule %s: %v", p.Project, err)
			continue
		}
		for _, s := range cfg.Jobs() {
			sc, err := stack.ParseSchedule(s.Schedule)
			if s.Schedule == "" || err != nil || !sc.Matches(minute) {
				continue
			}
			j, _, err := stack.QueueJob(r.dir, cfg, s.Name, "schedule", "")
			switch {
			case errors.Is(err, stack.ErrScheduledPending):
				log.Printf("schedule %s %s: the last job has not ended; not queued again", cfg.Name, s.Name)
			case err != nil:
				log.Printf("schedule %s %s: %v", cfg.Name, s.Name, err)
			default:
				log.Printf("schedule %s %s: queued job %s", cfg.Name, s.Name, j.ID)
			}
		}
	}
}

// bayBinary is the bay installed beside this agent.
func bayBinary() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	bin := filepath.Join(filepath.Dir(self), "bay")
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("no bay beside %s to run jobs with", self)
	}
	return bin, nil
}

func (r *jobRunner) startNext() {
	j, ok, err := stack.NextJob(r.dir)
	if err != nil {
		log.Printf("jobs: %v", err)
		return
	}
	if !ok {
		return
	}
	bin, err := bayBinary()
	if err != nil {
		stack.FinishJob(r.dir, j.ID, stack.JobFailed, nil, err.Error(), 0)
		return
	}
	out, err := os.OpenFile(stack.JobLogPath(r.dir, j.ID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		stack.FinishJob(r.dir, j.ID, stack.JobFailed, nil, err.Error(), 0)
		return
	}
	cmd := exec.Command(bin, "-state", r.dir, "job", "run", j.ID)
	cmd.Stdout, cmd.Stderr = out, out
	// The run is a process group of its own, so what ends the agent, launchd
	// stopping its job or a signal to its group, does not end the job.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		out.Close()
		stack.FinishJob(r.dir, j.ID, stack.JobFailed, nil, err.Error(), 0)
		return
	}
	r.busy.Store(true)
	stack.UpdateJob(r.dir, j.ID, func(job *stack.Job) error { job.PID = cmd.Process.Pid; return nil })
	log.Printf("job %s: running %s %s", j.ID, j.Project, j.Service)
	go func() {
		defer r.busy.Store(false)
		defer out.Close()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		limit := time.NewTimer(j.Timeout + jobOverrun)
		defer limit.Stop()
		var err error
		select {
		case err = <-done:
		case <-limit.C:
			log.Printf("job %s: its process did not end %s past the job's limit; ending it", j.ID, jobOverrun)
			cmd.Process.Signal(syscall.SIGTERM)
			select {
			case err = <-done:
			case <-time.After(2 * time.Minute):
				cmd.Process.Kill()
				err = <-done
			}
		}
		r.ended(j.ID, err)
	}()
}

// report writes this machine's jobs where the link serves them.
func (r *jobRunner) report(ctx context.Context) {
	t := time.NewTicker(jobReportPoll)
	defer t.Stop()
	failed := false
	for {
		rep, err := stack.LocalJobReport(r.dir)
		if err == nil {
			err = stack.WriteJobReport(r.dir, rep)
		}
		if err != nil && !failed {
			log.Printf("jobs report: %v", err)
		}
		failed = err != nil
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// followPeers reads the jobs of each approved peer and says when one ends.
// Only macOS shows a notification, so only there are peers followed.
//
// A peer may be out of reach for a while; its jobs run on there and keep
// their results, and a job that ended meanwhile is announced when the link is
// back. What was announced is kept in <state>/jobs/announced.json, so an agent
// that restarted announces what it missed and nothing twice. The first reading
// of a peer is what was there already and is not announced.
func (r *jobRunner) followPeers(ctx context.Context) {
	if runtime.GOOS != "darwin" {
		return
	}
	announced := stack.LoadAnnounced(r.dir)
	t := time.NewTicker(peerJobsPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		peers, err := stack.Peers(r.dir)
		if err != nil {
			continue
		}
		changed := false
		for _, p := range peers {
			rep, err := stack.FetchPeerJobs(r.dir, p, 5*time.Second)
			if err != nil {
				continue // out of reach: read again next round
			}
			ended := map[string]bool{}
			for _, j := range rep.Jobs {
				if j.Done() {
					ended[j.ID] = true
				}
			}
			seen, known := announced[p.Fingerprint]
			if known {
				for _, j := range rep.Jobs {
					if j.Done() && !seen[j.ID] {
						notify(p.Name, j)
					}
				}
			}
			if !known || len(ended) != len(seen) || !sameKeys(ended, seen) {
				announced[p.Fingerprint] = ended
				changed = true
			}
		}
		if changed {
			if err := stack.SaveAnnounced(r.dir, announced); err != nil {
				log.Printf("jobs: %v", err)
			}
		}
	}
}

func sameKeys(a, b map[string]bool) bool {
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// notify says that a job ended, in a macOS notification.
func notify(machine string, j stack.Job) {
	if runtime.GOOS != "darwin" {
		return
	}
	title := "bay: " + j.Project + " " + j.Service + " " + j.State
	if machine != "" {
		title = "bay on " + machine + ": " + j.Project + " " + j.Service + " " + j.State
	}
	body := "in " + j.Duration().String()
	if j.ExitCode != nil && *j.ExitCode != 0 {
		body += fmt.Sprintf(", exit %d", *j.ExitCode)
	}
	if j.Reason != "" {
		body += ": " + j.Reason
	}
	script := fmt.Sprintf("display notification %q with title %q", body, title)
	if err := exec.Command("/usr/bin/osascript", "-e", script).Run(); err != nil {
		log.Printf("notification: %v", err)
	}
}
