package stack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// A machine tells its approved peers about its jobs as it tells them its load:
// the agent writes jobs.json beside the peer document, and the link serves it
// at JobsPath to a client whose certificate an approved authority issued. The
// agent of a peer reads it to say when a job there ends.

// JobsPath is where the link serves the machine's jobs.
const JobsPath = "/__bay/jobs"

// JobsName is the file the agent writes beside the peer document.
const JobsName = "jobs.json"

// jobsReported is how many of the latest jobs a report holds, and
// jobsWithTail how many of them carry the last tailBytes of their output.
const (
	jobsReported = 50
	jobsWithTail = 10
	tailBytes    = 2048
)

// logTail returns the last n bytes of a file, from a line's start.
func logTail(path string, n int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	start := max(0, info.Size()-n)
	b := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(b, start); err != nil {
		return ""
	}
	if start > 0 {
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	return string(b)
}

// FindJob returns the job of a report that an ID, or a part of one from its
// start or its end, names.
func FindJob(jobs []Job, id string) (Job, error) {
	var found []Job
	for _, j := range jobs {
		if j.ID == id {
			return j, nil
		}
		if strings.HasPrefix(j.ID, id) || strings.HasSuffix(j.ID, id) {
			found = append(found, j)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return Job{}, fmt.Errorf("no job %s among the latest %d", id, jobsReported)
	default:
		return Job{}, fmt.Errorf("%s names %d jobs; give more of the ID", id, len(found))
	}
}

// JobReport is a machine's latest jobs and the schedules it follows.
type JobReport struct {
	Jobs      []Job           `json:"jobs"`
	Schedules []ScheduleEntry `json:"schedules"`
}

// ScheduleEntry is one job service on a schedule the machine follows.
type ScheduleEntry struct {
	Project  string `json:"project"`
	Service  string `json:"service"`
	Schedule string `json:"schedule"`
	// Next is the next time it is queued, or the reason the file cannot be
	// read.
	Next  string `json:"next,omitempty"`
	Error string `json:"error,omitempty"`
}

// LocalJobReport reads this machine's jobs and schedules.
func LocalJobReport(dir string) (JobReport, error) {
	jobs, err := Jobs(dir)
	if err != nil {
		return JobReport{}, err
	}
	if len(jobs) > jobsReported {
		jobs = jobs[len(jobs)-jobsReported:]
	}
	// The latest jobs carry the end of their output, so a peer that was cut
	// off reads how they ended without the machine.
	for i := max(0, len(jobs)-jobsWithTail); i < len(jobs); i++ {
		jobs[i].Tail = logTail(JobLogPath(dir, jobs[i].ID), tailBytes)
	}
	report := JobReport{Jobs: jobs, Schedules: []ScheduleEntry{}}
	if report.Jobs == nil {
		report.Jobs = []Job{}
	}
	list, err := Schedules(dir)
	if err != nil {
		return report, err
	}
	m := NewMachine(dir)
	now := time.Now()
	for _, p := range list {
		cfg, err := LoadNamed(m, p.File, p.Project)
		if err != nil {
			report.Schedules = append(report.Schedules, ScheduleEntry{Project: p.Project, Error: err.Error()})
			continue
		}
		for _, s := range cfg.Jobs() {
			if s.Schedule == "" {
				continue
			}
			e := ScheduleEntry{Project: cfg.Name, Service: s.Name, Schedule: s.Schedule}
			if sc, err := ParseSchedule(s.Schedule); err == nil {
				if next, ok := sc.Next(now); ok {
					e.Next = next.Format(time.RFC3339)
				}
			}
			report.Schedules = append(report.Schedules, e)
		}
	}
	return report, nil
}

// WriteJobReport writes the report beside the peer document of each engine's
// proxy, where the link serves it.
func WriteJobReport(dir string, report JobReport) error {
	b, err := json.Marshal(report)
	if err != nil {
		return err
	}
	m := NewMachine(dir)
	for _, engine := range []string{AppleEngine, DockerEngine} {
		conf := m.ConfDir(engine)
		if _, err := os.Stat(filepath.Join(conf, PeerDocumentName)); err != nil {
			continue
		}
		if err := writeInPlace(filepath.Join(conf, JobsName), append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// FetchPeerJobs reads an approved peer's jobs over its link.
func FetchPeerJobs(dir string, p Peer, timeout time.Duration) (JobReport, error) {
	var r JobReport
	body, err := fetchFromPeer(dir, p, JobsPath, 1<<20, timeout)
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(body, &r)
}

// FetchPeerJobsNamed reads the jobs of the approved peer with this name.
func FetchPeerJobsNamed(dir, name string, timeout time.Duration) (JobReport, error) {
	peers, err := Peers(dir)
	if err != nil {
		return JobReport{}, err
	}
	for _, p := range peers {
		if p.Name == name {
			return FetchPeerJobs(dir, p, timeout)
		}
	}
	return JobReport{}, fmt.Errorf("no approved peer named %s; bay peer lists them", name)
}

// CancelJob cancels a queued job, or stops a running one: the process running
// it is sent SIGTERM, stops the job's container and records it cancelled.
func CancelJob(dir, id string) (Job, error) {
	j, err := GetJob(dir, id)
	if err != nil {
		return j, err
	}
	if j.Done() {
		return j, fmt.Errorf("job %s has ended: %s", j.ID, j.State)
	}
	if j.State == JobQueued {
		j, err = UpdateJob(dir, j.ID, func(j *Job) error {
			if j.State != JobQueued {
				return fmt.Errorf("job %s started meanwhile; cancel it again", j.ID)
			}
			now := time.Now().UTC()
			j.State, j.Ended, j.Reason = JobCancelled, &now, "cancelled before it ran"
			return nil
		})
		return j, err
	}
	if j.PID <= 0 {
		return j, fmt.Errorf("job %s is starting; cancel it again in a moment", j.ID)
	}
	if err := syscall.Kill(j.PID, syscall.SIGTERM); err != nil {
		return j, fmt.Errorf("stopping job %s (process %d): %w", j.ID, j.PID, err)
	}
	j.State = "stopping"
	return j, nil
}

// announcedPath holds, per approved peer, the ended jobs this machine has
// announced.
func announcedPath(dir string) string { return filepath.Join(jobsDir(dir), "announced.json") }

// LoadAnnounced reads what was announced, by peer fingerprint and job ID.
func LoadAnnounced(dir string) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	b, err := os.ReadFile(announcedPath(dir))
	if err != nil {
		return out
	}
	var list map[string][]string
	if json.Unmarshal(b, &list) != nil {
		return out
	}
	for peer, ids := range list {
		out[peer] = map[string]bool{}
		for _, id := range ids {
			out[peer][id] = true
		}
	}
	return out
}

// SaveAnnounced records what was announced.
func SaveAnnounced(dir string, announced map[string]map[string]bool) error {
	list := map[string][]string{}
	for peer, ids := range announced {
		list[peer] = []string{}
		for id := range ids {
			list[peer] = append(list[peer], id)
		}
		sort.Strings(list[peer])
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(jobsDir(dir), 0o700); err != nil {
		return err
	}
	return replaceFile(announcedPath(dir), append(b, '\n'), 0o600)
}
