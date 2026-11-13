package stack

import (
	"time"

	"golang.org/x/sys/unix"
)

func processStartTime(pid int) (time.Time, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return time.Time{}, err
	}
	t := kp.Proc.P_starttime
	return time.Unix(t.Sec, int64(t.Usec)*1000), nil
}

func fileChangeTime(path string) (time.Time, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return time.Time{}, err
	}
	return time.Unix(st.Ctim.Sec, st.Ctim.Nsec), nil
}
