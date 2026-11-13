package stack

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The machine keeps one queue of jobs, in <state>/jobs, and its agent runs
// them one at a time in the order they were queued: two heavy runs on one
// machine compete for its memory, and one after the other they each get it.
// Each job is a file of its own, written under the jobs lock, and its output is
// a file beside it.

// Job states.
const (
	JobQueued    = "queued"
	JobRunning   = "running"
	JobSucceeded = "succeeded"
	JobFailed    = "failed"
	JobCancelled = "cancelled"
)

// Job is one run of a job service.
type Job struct {
	ID      string `json:"id"`
	Project string `json:"project"`
	File    string `json:"file"`
	Service string `json:"service"`
	// Origin is "request" for a job queued by job add and "schedule" for one
	// the agent queued from the service's schedule.
	Origin string `json:"origin"`
	// Key is the caller's name for the run, such as a commit: queueing a job
	// with a key already used for the service returns that job, so a caller
	// cut off before it read the answer queues again without running twice.
	Key      string        `json:"key,omitempty"`
	State    string        `json:"state"`
	Timeout  time.Duration `json:"timeout"`
	Queued   time.Time     `json:"queued"`
	Started  *time.Time    `json:"started,omitempty"`
	Ended    *time.Time    `json:"ended,omitempty"`
	ExitCode *int          `json:"exitCode,omitempty"`
	// PeakMemory is the most memory the job's container used, in bytes, where
	// the engine's host reports it.
	PeakMemory uint64 `json:"peakMemory,omitempty"`
	// Reason says why a job failed or was cancelled, when its exit code does
	// not.
	Reason string `json:"reason,omitempty"`
	// PID is the process running the job, while it runs.
	PID int `json:"pid,omitempty"`
	// Tail is the end of the job's output, in a report only.
	Tail string `json:"tail,omitempty"`
}

// Done reports whether the job has ended.
func (j Job) Done() bool {
	return j.State == JobSucceeded || j.State == JobFailed || j.State == JobCancelled
}

// Duration is how long the job ran, or has run so far.
func (j Job) Duration() time.Duration {
	if j.Started == nil {
		return 0
	}
	end := time.Now()
	if j.Ended != nil {
		end = *j.Ended
	}
	return end.Sub(*j.Started).Round(time.Second)
}

func jobsDir(dir string) string        { return filepath.Join(dir, "jobs") }
func jobPath(dir, id string) string    { return filepath.Join(jobsDir(dir), id+".json") }
func JobLogPath(dir, id string) string { return filepath.Join(jobsDir(dir), id+".log") }
func schedulesPath(dir string) string  { return filepath.Join(jobsDir(dir), "schedules.json") }

// jobsLockWait bounds the wait for another command's change to the queue.
const jobsLockWait = 30 * time.Second

// newJobID is sortable by the time it was made, to the microsecond, and
// unique within it.
func newJobID(now time.Time) string {
	b := make([]byte, 2)
	rand.Read(b)
	return now.UTC().Format("20060102-150405.000000") + "-" + hex.EncodeToString(b)
}

// QueueJob adds a job for one job service of a project. With a key that a
// job of the service already has, it returns that job and false.
func QueueJob(dir string, cfg *Config, service, origin, key string) (Job, bool, error) {
	s, ok := cfg.Services[strings.ToLower(service)]
	if !ok {
		return Job{}, false, fmt.Errorf("project %s has no service %q", cfg.Name, service)
	}
	if !s.Job {
		return Job{}, false, fmt.Errorf("%s is not a job; declare it with x-bay.job: true", s.Name)
	}
	now := time.Now()
	j := Job{ID: newJobID(now), Project: cfg.Name, File: cfg.Path(), Service: s.Name,
		Origin: origin, Key: key, State: JobQueued, Timeout: s.JobTimeout, Queued: now.UTC()}
	added := true
	err := withLock(dir, "jobs", jobsLockWait, func() error {
		jobs, err := readJobs(dir)
		if err != nil {
			return err
		}
		for _, other := range jobs {
			same := other.Project == j.Project && other.Service == j.Service
			if same && key != "" && other.Key == key {
				j, added = other, false
				return nil
			}
			// A schedule that comes round while its last job waits or runs
			// adds nothing: the queue would otherwise grow without end on a
			// machine slower than the schedule.
			if same && origin == "schedule" && !other.Done() {
				return ErrScheduledPending
			}
		}
		return writeJob(dir, j)
	})
	return j, added, err
}

// ErrScheduledPending is returned when a schedule comes round while its last
// job is queued or running.
var ErrScheduledPending = errors.New("a job of this schedule is queued or running")

func writeJob(dir string, j Job) error {
	if err := os.MkdirAll(jobsDir(dir), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return replaceFile(jobPath(dir, j.ID), append(b, '\n'), 0o600)
}

func readJobs(dir string) ([]Job, error) {
	entries, err := os.ReadDir(jobsDir(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var jobs []Job
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || name == filepath.Base(schedulesPath(dir)) ||
			name == filepath.Base(announcedPath(dir)) || strings.HasPrefix(name, ".") {
			continue
		}
		j, err := readJob(dir, strings.TrimSuffix(name, ".json"))
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	// The queue order is the order jobs were queued in.
	sort.Slice(jobs, func(a, b int) bool {
		if !jobs[a].Queued.Equal(jobs[b].Queued) {
			return jobs[a].Queued.Before(jobs[b].Queued)
		}
		return jobs[a].ID < jobs[b].ID
	})
	return jobs, nil
}

func readJob(dir, id string) (Job, error) {
	var j Job
	b, err := os.ReadFile(jobPath(dir, id))
	if err != nil {
		if os.IsNotExist(err) {
			return j, fmt.Errorf("no job %s; bay job list shows them", id)
		}
		return j, err
	}
	if err := json.Unmarshal(b, &j); err != nil {
		return j, fmt.Errorf("job %s: %w", id, err)
	}
	return j, nil
}

// Jobs lists the machine's jobs, oldest first.
func Jobs(dir string) ([]Job, error) { return readJobs(dir) }

// GetJob reads one job. A prefix of its ID that names one job is enough.
func GetJob(dir, id string) (Job, error) {
	if j, err := readJob(dir, id); err == nil {
		return j, nil
	}
	jobs, err := readJobs(dir)
	if err != nil {
		return Job{}, err
	}
	var found []Job
	for _, j := range jobs {
		if strings.HasPrefix(j.ID, id) || strings.HasSuffix(j.ID, id) {
			found = append(found, j)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return Job{}, fmt.Errorf("no job %s; bay job list shows them", id)
	default:
		return Job{}, fmt.Errorf("%s names %d jobs; give more of the ID", id, len(found))
	}
}

// UpdateJob changes one job under the jobs lock.
func UpdateJob(dir, id string, change func(*Job) error) (Job, error) {
	var j Job
	err := withLock(dir, "jobs", jobsLockWait, func() error {
		var err error
		if j, err = readJob(dir, id); err != nil {
			return err
		}
		if err := change(&j); err != nil {
			return err
		}
		return writeJob(dir, j)
	})
	return j, err
}

// NextJob marks the oldest queued job running and returns it. It returns
// false when a job runs already or none is queued.
func NextJob(dir string) (Job, bool, error) {
	var next Job
	found := false
	err := withLock(dir, "jobs", jobsLockWait, func() error {
		jobs, err := readJobs(dir)
		if err != nil {
			return err
		}
		for _, j := range jobs {
			if j.State == JobRunning {
				return nil
			}
		}
		for _, j := range jobs {
			if j.State == JobQueued {
				now := time.Now().UTC()
				j.State, j.Started = JobRunning, &now
				next, found = j, true
				return writeJob(dir, j)
			}
		}
		return nil
	})
	return next, found, err
}

// FinishJob records how a job ended, unless it has ended already.
func FinishJob(dir, id, state string, exit *int, reason string, peak uint64) (Job, error) {
	return UpdateJob(dir, id, func(j *Job) error {
		if j.Done() {
			return nil
		}
		now := time.Now().UTC()
		j.State, j.Ended, j.ExitCode, j.Reason, j.PID = state, &now, exit, reason, 0
		if peak > j.PeakMemory {
			j.PeakMemory = peak
		}
		if j.Started == nil {
			j.Started = &now
		}
		return nil
	})
}

// jobsKept is how many ended jobs the queue keeps, with their output.
const jobsKept = 200

// PruneJobs removes the oldest ended jobs beyond jobsKept.
func PruneJobs(dir string) error {
	return withLock(dir, "jobs", jobsLockWait, func() error {
		jobs, err := readJobs(dir)
		if err != nil {
			return err
		}
		var ended []Job
		for _, j := range jobs {
			if j.Done() {
				ended = append(ended, j)
			}
		}
		for i := 0; i < len(ended)-jobsKept; i++ {
			os.Remove(jobPath(dir, ended[i].ID))
			os.Remove(JobLogPath(dir, ended[i].ID))
		}
		return nil
	})
}

// ScheduledProject is a Compose file whose job schedules the agent follows.
type ScheduledProject struct {
	Project string `json:"project"`
	File    string `json:"file"`
}

// Schedules lists the projects whose schedules the agent follows.
func Schedules(dir string) ([]ScheduledProject, error) {
	b, err := os.ReadFile(schedulesPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []ScheduledProject
	return out, json.Unmarshal(b, &out)
}

// SetSchedule follows or stops following a project's schedules. It reports
// whether the list changed.
func SetSchedule(dir string, p ScheduledProject, follow bool) (bool, error) {
	changed := false
	err := withLock(dir, "jobs", jobsLockWait, func() error {
		list, err := Schedules(dir)
		if err != nil {
			return err
		}
		var out []ScheduledProject
		present := false
		for _, q := range list {
			if q.Project == p.Project {
				present = true
				if !follow || q.File != p.File {
					changed = true
					continue
				}
			}
			out = append(out, q)
		}
		if follow && (!present || changed) {
			out, changed = append(out, p), true
		}
		if !changed {
			return nil
		}
		if err := os.MkdirAll(jobsDir(dir), 0o700); err != nil {
			return err
		}
		sort.Slice(out, func(a, b int) bool { return out[a].Project < out[b].Project })
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		return replaceFile(schedulesPath(dir), append(b, '\n'), 0o600)
	})
	return changed, err
}

// AgentRuns reports whether the resident agent, which runs the queue, is
// running: the DNS agent on macOS, the systemd agent elsewhere.
func AgentRuns() bool {
	if hostSetsUpNames {
		return DNSAgentLoaded() && launchdPID(dnsAgentTarget()) > 0
	}
	return systemdPID(agentUnit) > 0
}
