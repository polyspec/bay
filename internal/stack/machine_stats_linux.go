package stack

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func sampleCPU(s *StatsSampler) (float64, error) {
	now, err := readCPU()
	if err != nil {
		return 0, err
	}
	return s.shareSince(now), nil
}

// readCPU reads the first line of /proc/stat: user nice system idle iowait irq
// softirq steal, in clock ticks. Idle and iowait are not busy.
func readCPU() (cpuTimes, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuTimes{}, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return cpuTimes{}, fmt.Errorf("/proc/stat is empty")
	}
	fields := strings.Fields(sc.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuTimes{}, fmt.Errorf("/proc/stat: unexpected %q", sc.Text())
	}
	var t cpuTimes
	for i, f := range fields[1:] {
		if i >= 8 {
			break // guest time is already counted in user
		}
		n, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return cpuTimes{}, err
		}
		t.total += n
		if i != 3 && i != 4 {
			t.busy += n
		}
	}
	return t, nil
}

// readMemory reads /proc/meminfo. Used memory is what is not available, which
// counts reclaimable cache as free, as the kernel does.
func readMemory(st *MachineStats) error {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return err
	}
	kb := map[string]uint64{}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			n, _ := strconv.ParseUint(fields[1], 10, 64)
			kb[strings.TrimSuffix(fields[0], ":")] = n * 1024
		}
	}
	st.MemTotal = kb["MemTotal"]
	st.MemUsed = kb["MemTotal"] - kb["MemAvailable"]
	st.SwapTotal = kb["SwapTotal"]
	st.SwapUsed = kb["SwapTotal"] - kb["SwapFree"]
	return nil
}
