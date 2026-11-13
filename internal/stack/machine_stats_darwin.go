package stack

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// sampleCPU reads the processor share over one second from iostat, whose
// second report covers the interval; macOS keeps no counters a process reads
// without cgo. Paths are absolute: launchd starts the agent with a search
// path that holds neither /usr/sbin nor /usr/bin.
func sampleCPU(*StatsSampler) (float64, error) {
	out, err := exec.Command("/usr/sbin/iostat", "-n", "0", "-c", "2", "-w", "1").Output()
	if err != nil {
		return 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	fields := strings.Fields(lines[len(lines)-1])
	// The columns are us sy id, then the load averages.
	if len(fields) < 3 {
		return 0, fmt.Errorf("iostat: unexpected %q", lines[len(lines)-1])
	}
	idle, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return 0, err
	}
	return 100 - idle, nil
}

// readMemory reads the memory in use as Activity Monitor counts it, wired,
// active and compressed pages, and the swap in use.
func readMemory(st *MachineStats) error {
	total, err := sysctlUint("hw.memsize")
	if err != nil {
		return err
	}
	st.MemTotal = total
	out, err := exec.Command("/usr/bin/vm_stat").Output()
	if err != nil {
		return err
	}
	page := uint64(16384)
	pages := map[string]uint64{}
	for _, line := range strings.Split(string(out), "\n") {
		if i := strings.Index(line, "page size of "); i >= 0 {
			if n, err := strconv.ParseUint(strings.Fields(line[i+len("page size of "):])[0], 10, 64); err == nil {
				page = n
			}
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if n, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(value), "."), 10, 64); err == nil {
			pages[strings.Trim(name, "\" ")] = n
		}
	}
	st.MemUsed = (pages["Pages wired down"] + pages["Pages active"] + pages["Pages occupied by compressor"]) * page
	swap, err := exec.Command("/usr/sbin/sysctl", "-n", "vm.swapusage").Output()
	if err != nil {
		return err
	}
	// total = 7168.00M  used = 5928.06M  free = 1239.94M  (encrypted)
	f := strings.Fields(string(swap))
	for i := 0; i+2 < len(f); i++ {
		if f[i+1] != "=" {
			continue
		}
		mb, err := strconv.ParseFloat(strings.TrimSuffix(f[i+2], "M"), 64)
		if err != nil {
			continue
		}
		switch f[i] {
		case "total":
			st.SwapTotal = uint64(mb * 1024 * 1024)
		case "used":
			st.SwapUsed = uint64(mb * 1024 * 1024)
		}
	}
	return nil
}

func sysctlUint(name string) (uint64, error) {
	out, err := exec.Command("/usr/sbin/sysctl", "-n", name).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
}
