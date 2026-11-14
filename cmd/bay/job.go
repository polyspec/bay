package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/polyspec/bay/internal/stack"
)

// defaultJobWait bounds job wait and job add -wait when -timeout is not given.
const defaultJobWait = 2 * time.Hour

func job(m *stack.Machine, args []string) error {
	if len(args) == 0 {
		return usageError{"job needs a subcommand: add, list, show, logs, wait, cancel, schedule, unschedule or run"}
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return jobAdd(m, rest)
	case "list":
		return jobList(m, rest)
	case "show":
		return jobShow(m, rest)
	case "logs":
		return jobLogs(m, rest)
	case "wait":
		return jobWait(m, rest)
	case "cancel":
		return jobCancel(m, rest)
	case "schedule", "unschedule":
		return jobSchedule(m, sub == "schedule")
	case "run":
		return jobRun(m, rest)
	default:
		return usageError{fmt.Sprintf("unknown job subcommand %q", sub)}
	}
}

func oneID(fs *flag.FlagSet, args []string) (string, error) {
	if err := fs.Parse(args); err != nil {
		return "", usageError{err.Error()}
	}
	if fs.NArg() != 1 {
		return "", usageError{"name one job by its ID"}
	}
	return fs.Arg(0), nil
}

func jobAdd(m *stack.Machine, args []string) error {
	fs := flag.NewFlagSet("job add", flag.ContinueOnError)
	wait := fs.Bool("wait", false, "print the job's output as it runs and exit with its result")
	bound := fs.Duration("timeout", defaultJobWait, "with -wait, the longest to wait, queue time included")
	asJSON := fs.Bool("json", false, "print the queued job as JSON")
	key := fs.String("key", "", "a name for this run, such as a commit; a job of the service with the same key is returned instead of queueing another")
	if err := fs.Parse(args); err != nil {
		return usageError{err.Error()}
	}
	if fs.NArg() != 1 {
		return usageError{"job add needs one job service"}
	}
	cfg, err := loadProject(m)
	if err != nil {
		return err
	}
	j, added, err := stack.QueueJob(m.Dir, cfg, fs.Arg(0), "request", *key)
	if err != nil {
		return err
	}
	if !added {
		fmt.Fprintf(os.Stderr, "job %s has the key %s already: %s\n", j.ID, *key, j.State)
	}
	if !stack.AgentRuns() {
		fmt.Fprintln(os.Stderr, "bay: the agent, which runs the queue, is not running; bay check names what is missing")
	}
	if *asJSON && !*wait {
		return printJSON(j)
	}
	if added {
		fmt.Fprintf(os.Stderr, "queued job %s (%s %s)\n", j.ID, j.Project, j.Service)
	}
	if !*wait {
		fmt.Println(j.ID)
		return nil
	}
	return followJob(m, j.ID, *bound, true)
}

func jobList(m *stack.Machine, args []string) error {
	fs := flag.NewFlagSet("job list", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the jobs and the followed schedules as JSON")
	peerName := fs.String("peer", "", "list the jobs of this approved peer, read over the link")
	if err := fs.Parse(args); err != nil {
		return usageError{err.Error()}
	}
	var report stack.JobReport
	var err error
	if *peerName != "" {
		report, err = stack.FetchPeerJobsNamed(m.Dir, *peerName, 5*time.Second)
	} else {
		report, err = stack.LocalJobReport(m.Dir)
	}
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(report)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tPROJECT\tJOB\tSTATE\tEXIT\tTIME\tPEAK MEMORY\tQUEUED BY")
	for _, j := range report.Jobs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", j.ID, j.Project, j.Service, j.State,
			exitText(j), durationText(j), memoryText(j.PeakMemory), j.Origin)
	}
	tw.Flush()
	for _, s := range report.Schedules {
		fmt.Printf("\nschedule %s %s: %s, next %s\n", s.Project, s.Service, s.Schedule, s.Next)
	}
	return nil
}

func jobShow(m *stack.Machine, args []string) error {
	fs := flag.NewFlagSet("job show", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the job as JSON")
	peerName := fs.String("peer", "", "a job of this approved peer, read over the link")
	id, err := oneID(fs, args)
	if err != nil {
		return err
	}
	j, err := findJob(m, *peerName, id)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(j)
	}
	fmt.Printf("job       %s\nproject   %s (%s)\nservice   %s\nstate     %s\n", j.ID, j.Project, j.File, j.Service, j.State)
	fmt.Printf("queued    %s by %s\n", j.Queued.Local().Format(time.DateTime), j.Origin)
	if j.Started != nil {
		fmt.Printf("ran       %s\n", durationText(j))
	}
	if j.ExitCode != nil {
		fmt.Printf("exit      %d\n", *j.ExitCode)
	}
	if j.PeakMemory > 0 {
		fmt.Printf("memory    %s at most\n", memoryText(j.PeakMemory))
	}
	if j.Reason != "" {
		fmt.Printf("reason    %s\n", j.Reason)
	}
	if *peerName == "" {
		fmt.Printf("output    %s\n", stack.JobLogPath(m.Dir, j.ID))
	}
	return nil
}

// findJob reads a job of this machine, or of an approved peer over the link.
func findJob(m *stack.Machine, peerName, id string) (stack.Job, error) {
	if peerName == "" {
		return stack.GetJob(m.Dir, id)
	}
	report, err := stack.FetchPeerJobsNamed(m.Dir, peerName, 5*time.Second)
	if err != nil {
		return stack.Job{}, err
	}
	return stack.FindJob(report.Jobs, id)
}

func jobLogs(m *stack.Machine, args []string) error {
	fs := flag.NewFlagSet("job logs", flag.ContinueOnError)
	follow := fs.Bool("f", false, "follow the output until the job ends")
	bound := fs.Duration("timeout", defaultJobWait, "with -f, the longest to follow")
	peerName := fs.String("peer", "", "the end of the output of an approved peer's job, read over the link")
	id, err := oneID(fs, args)
	if err != nil {
		return err
	}
	if *peerName != "" {
		j, err := findJob(m, *peerName, id)
		if err != nil {
			return err
		}
		if j.Tail == "" {
			return fmt.Errorf("the link carries the output of the latest jobs only; on %s: bay job logs %s", *peerName, j.ID)
		}
		fmt.Print(j.Tail)
		return nil
	}
	j, err := stack.GetJob(m.Dir, id)
	if err != nil {
		return err
	}
	if *follow {
		return followJob(m, j.ID, *bound, false)
	}
	f, err := os.Open(stack.JobLogPath(m.Dir, j.ID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil // queued: no output yet
		}
		return err
	}
	defer f.Close()
	_, err = io.Copy(os.Stdout, f)
	return err
}

func jobWait(m *stack.Machine, args []string) error {
	fs := flag.NewFlagSet("job wait", flag.ContinueOnError)
	bound := fs.Duration("timeout", defaultJobWait, "the longest to wait")
	peerName := fs.String("peer", "", "wait for a job of this approved peer, read over the link; a lost link is tried again until the bound")
	id, err := oneID(fs, args)
	if err != nil {
		return err
	}
	if *peerName != "" {
		return waitPeerJob(m, *peerName, id, *bound)
	}
	j, err := stack.GetJob(m.Dir, id)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "waiting up to %s for job %s\n", *bound, j.ID)
	deadline := time.Now().Add(*bound)
	for !j.Done() {
		if time.Now().After(deadline) {
			return fmt.Errorf("job %s is still %s after %s; bay job wait %s waits again", j.ID, j.State, *bound, j.ID)
		}
		time.Sleep(time.Second)
		if j, err = stack.GetJob(m.Dir, j.ID); err != nil {
			return err
		}
	}
	return jobResult(m, j, "bay job logs "+j.ID)
}

// peerPoll is how often a wait reads a peer's jobs.
const peerPoll = 5 * time.Second

// waitPeerJob waits for a job of an approved peer. The link may drop while the
// job runs, as a network does; each reading that fails is tried again until
// the bound, and the outage and its end are said once each.
func waitPeerJob(m *stack.Machine, peerName, id string, bound time.Duration) error {
	fmt.Fprintf(os.Stderr, "waiting up to %s for job %s on %s\n", bound, id, peerName)
	deadline := time.Now().Add(bound)
	var lost time.Time
	for {
		report, err := stack.FetchPeerJobsNamed(m.Dir, peerName, 5*time.Second)
		if err == nil {
			if !lost.IsZero() {
				fmt.Fprintf(os.Stderr, "the link to %s is back after %s\n", peerName, time.Since(lost).Round(time.Second))
				lost = time.Time{}
			}
			j, err := stack.FindJob(report.Jobs, id)
			if err != nil {
				return err
			}
			id = j.ID
			if j.Done() {
				fmt.Print(j.Tail)
				return jobResult(m, j, "bay job logs -peer "+peerName+" "+j.ID)
			}
		} else if lost.IsZero() {
			lost = time.Now()
			fmt.Fprintf(os.Stderr, "the link to %s is lost (%v); the job runs on there, reading again every %s\n", peerName, err, peerPoll)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("job %s on %s has not ended after %s; bay job wait -peer %s %s waits again", id, peerName, bound, peerName, id)
		}
		time.Sleep(peerPoll)
	}
}

// followJob prints a job's output as it grows until the job ends, then
// reports its result.
func followJob(m *stack.Machine, id string, bound time.Duration, result bool) error {
	fmt.Fprintf(os.Stderr, "following job %s for up to %s\n", id, bound)
	deadline := time.Now().Add(bound)
	var offset int64
	for {
		j, err := stack.GetJob(m.Dir, id)
		if err != nil {
			return err
		}
		offset = copyFrom(stack.JobLogPath(m.Dir, id), offset)
		if j.Done() {
			offset = copyFrom(stack.JobLogPath(m.Dir, id), offset)
			if result {
				return jobResult(m, j, "bay job logs "+j.ID)
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("job %s is still %s after %s; bay job wait %s waits again", id, j.State, bound, id)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func copyFrom(path string, offset int64) int64 {
	f, err := os.Open(path)
	if err != nil {
		return offset
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset
	}
	n, _ := io.Copy(os.Stdout, f)
	return offset + n
}

// jobResult reports an ended job: exit 0 when it succeeded, otherwise a line
// saying how it ended and exit 1.
func jobResult(m *stack.Machine, j stack.Job, logs string) error {
	summary := fmt.Sprintf("job %s %s %s %s in %s", j.ID, j.Project, j.Service, j.State, j.Duration())
	if j.PeakMemory > 0 {
		summary += ", " + memoryText(j.PeakMemory) + " at most"
	}
	if j.State == stack.JobSucceeded {
		fmt.Fprintln(os.Stderr, summary)
		return nil
	}
	if j.ExitCode != nil {
		summary += fmt.Sprintf(", exit %d", *j.ExitCode)
	}
	if j.Reason != "" {
		summary += ": " + j.Reason
	}
	return errors.New(summary + "; " + logs + " prints its output")
}

func jobCancel(m *stack.Machine, args []string) error {
	fs := flag.NewFlagSet("job cancel", flag.ContinueOnError)
	id, err := oneID(fs, args)
	if err != nil {
		return err
	}
	j, err := stack.CancelJob(m.Dir, id)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "job %s: %s\n", j.ID, j.State)
	return nil
}

func jobSchedule(m *stack.Machine, follow bool) error {
	cfg, err := loadProject(m)
	if err != nil {
		return err
	}
	scheduled := 0
	for _, s := range cfg.Jobs() {
		if s.Schedule != "" {
			scheduled++
		}
	}
	if follow && scheduled == 0 {
		return fmt.Errorf("project %s has no job with x-bay.schedule", cfg.Name)
	}
	changed, err := stack.SetSchedule(m.Dir, stack.ScheduledProject{Project: cfg.Name, File: cfg.Path()}, follow)
	if err != nil {
		return err
	}
	switch {
	case follow && changed:
		fmt.Fprintf(os.Stderr, "following the schedules of %s\n", cfg.Name)
	case follow:
		fmt.Fprintf(os.Stderr, "the schedules of %s are followed already\n", cfg.Name)
	case changed:
		fmt.Fprintf(os.Stderr, "no longer following the schedules of %s\n", cfg.Name)
	default:
		fmt.Fprintf(os.Stderr, "the schedules of %s were not followed\n", cfg.Name)
	}
	if follow && !stack.AgentRuns() {
		fmt.Fprintln(os.Stderr, "bay: the agent, which follows schedules, is not running; bay check names what is missing")
	}
	return nil
}

// jobRun runs a job the agent has started: its state is running. The agent
// runs it with its output going to the job's log; a person can run a queued
// job only by queueing it.
func jobRun(m *stack.Machine, args []string) error {
	fs := flag.NewFlagSet("job run", flag.ContinueOnError)
	id, err := oneID(fs, args)
	if err != nil {
		return err
	}
	j, err := stack.GetJob(m.Dir, id)
	if err != nil {
		return err
	}
	if j.State != stack.JobRunning {
		return fmt.Errorf("job %s is %s; the agent runs a job once it starts it, and bay job add queues one", j.ID, j.State)
	}
	cfg, err := stack.LoadNamed(m, j.File, j.Project)
	if err != nil {
		stack.FinishJob(m.Dir, j.ID, stack.JobFailed, nil, err.Error(), 0)
		return err
	}
	rt, err := newRuntime(m)
	if err != nil {
		stack.FinishJob(m.Dir, j.ID, stack.JobFailed, nil, err.Error(), 0)
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, j.Timeout)
	defer cancel()
	outcome, err := rt.RunJob(ctx, cfg, j.Service, os.Stdout)
	state, reason := stack.JobSucceeded, ""
	var exit *int
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		state, reason = stack.JobFailed, fmt.Sprintf("stopped at its time limit of %s", j.Timeout)
	case errors.Is(err, context.Canceled):
		state, reason = stack.JobCancelled, "stopped by bay job cancel or a signal"
	case err != nil:
		state, reason = stack.JobFailed, err.Error()
	default:
		code := outcome.ExitCode
		exit = &code
		if code != 0 {
			state = stack.JobFailed
		}
	}
	if err == nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		if exit == nil && outcome.ExitCode != 0 {
			code := outcome.ExitCode
			exit = &code
		}
	}
	if outcome.LeftUp != "" {
		if reason != "" {
			reason += "; "
		}
		reason += outcome.LeftUp
	}
	if _, ferr := stack.FinishJob(m.Dir, j.ID, state, exit, reason, outcome.PeakMemory); ferr != nil {
		return ferr
	}
	if state != stack.JobSucceeded {
		if reason == "" {
			reason = fmt.Sprintf("exit %d", outcome.ExitCode)
		}
		return fmt.Errorf("job %s %s: %s", j.ID, state, reason)
	}
	return nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func exitText(j stack.Job) string {
	if j.ExitCode == nil {
		return "-"
	}
	return fmt.Sprint(*j.ExitCode)
}

func durationText(j stack.Job) string {
	if j.Started == nil {
		return "-"
	}
	return j.Duration().String()
}

func memoryText(b uint64) string {
	if b == 0 {
		return "-"
	}
	const mib = 1 << 20
	if b >= 1<<30 {
		return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30))
	}
	return fmt.Sprintf("%d MiB", (b+mib-1)/mib)
}
