package stack

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// userHZ is the unit of the start time in /proc/<pid>/stat. It is fixed at 100
// in the kernel's interface to programs, whatever the kernel's own tick.
const userHZ = 100

func processStartTime(pid int) (time.Time, error) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return time.Time{}, err
	}
	// The command name, in parentheses, may hold spaces; the fields after it
	// start at the state, field 3. The start time is field 22.
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return time.Time{}, fmt.Errorf("unreadable /proc/%d/stat", pid)
	}
	fields := strings.Fields(s[i+1:])
	if len(fields) < 20 {
		return time.Time{}, fmt.Errorf("unreadable /proc/%d/stat", pid)
	}
	ticks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	boot, err := bootTime()
	if err != nil {
		return time.Time{}, err
	}
	return boot.Add(time.Duration(ticks) * time.Second / userHZ), nil
}

func bootTime() (time.Time, error) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "btime "); ok {
			sec, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return time.Time{}, err
			}
			return time.Unix(sec, 0), nil
		}
	}
	return time.Time{}, fmt.Errorf("no btime in /proc/stat")
}

func fileChangeTime(path string) (time.Time, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return time.Time{}, err
	}
	return time.Unix(st.Ctim.Sec, st.Ctim.Nsec), nil
}
