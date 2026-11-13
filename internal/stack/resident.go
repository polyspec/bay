package stack

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// A resident process (the DNS agent, the relay daemon, the Linux agent) runs
// the executable it was started from. A new version installed over it is
// picked up when the process notices its file changed and exits, but a process
// started from a version that predates that, or one that has not looked yet,
// still runs the old code while its job is registered exactly as install would
// register it. The setup is therefore in place only when the process started
// after its executable was last replaced; install restarts one that did not.

// startSlack absorbs the coarser clock a process start time is kept in
// (1/100 s on Linux, with a boot time kept in whole seconds), so a process started just after its file was written is
// not taken for an older one.
const startSlack = 2 * time.Second

// processStart returns when a process started. Tests replace it.
var processStart = processStartTime

// fileChanged returns when a file's inode last changed: written, renamed or
// replaced. Unlike the modification time, no archive or copy can set it back.
// Tests replace it.
var fileChanged = fileChangeTime

// residentStale reports whether the process pid runs an older copy of exe: it
// started before exe was last replaced. A process or file that cannot be read
// is not called stale; whether the job runs at all is checked elsewhere.
func residentStale(pid int, exe string) bool {
	if pid <= 0 || exe == "" {
		return false
	}
	started, err := processStart(pid)
	if err != nil {
		return false
	}
	changed, err := fileChanged(exe)
	if err != nil {
		return false
	}
	return started.Add(startSlack).Before(changed)
}

// launchdPID returns the pid launchd reports for a job target such as
// system/dev.bay.relay, or 0 when the job is not running.
func launchdPID(target string) int {
	out, err := exec.Command("launchctl", "print", target).Output()
	if err != nil {
		return 0
	}
	return parseLaunchdPID(string(out))
}

func parseLaunchdPID(out string) int {
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(k) == "pid" {
			if pid, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				return pid
			}
		}
	}
	return 0
}

// systemdPID returns the main pid of a unit, or 0 when it is not running.
func systemdPID(unit string) int {
	out, err := exec.Command("systemctl", "show", "--property=MainPID", "--value", unit).Output()
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return pid
}
