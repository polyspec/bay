package stack

import (
	"encoding/json"
	"fmt"
	"time"
)

// ContainerStats is one reading of what a running container uses, as the
// engine reports it. CPU time is cumulative; a share of the CPU needs two
// readings.
type ContainerStats struct {
	CPUUsageUsec uint64
	MemoryUsage  uint64
	MemoryLimit  uint64
	BlockRead    uint64
	BlockWrite   uint64
	NetRx        uint64
	NetTx        uint64
	Processes    int
	// CPUs is the number of cores the container was given.
	CPUs int
	// At is when the reading was taken.
	At time.Time
}

// ReadStats reads the statistics of the named running containers in one call.
// A call takes about two seconds whether it names one container or ten, so a
// project's containers are read together, for the project whose screen is open
// and apart from the machine's refresh.
//
// Docker reports its statistics in another form and reading them is not
// implemented; a Docker container returns an error that says so.
func ReadStats(names ...string) (map[string]ContainerStats, error) {
	if len(names) == 0 {
		return map[string]ContainerStats{}, nil
	}
	for _, name := range names {
		engine, err := engineOf(name)
		if err != nil {
			return nil, err
		}
		if engine != AppleEngine {
			return nil, fmt.Errorf("resource use is read from Apple container only; %s runs on %s", name, engine)
		}
	}
	out, err := runEngine(AppleEngine, append([]string{"stats", "--no-stream", "--format", "json"}, names...)...)
	if err != nil {
		return nil, err
	}
	// The cores given are in the configuration, which stats does not report.
	conf, err := runEngine(AppleEngine, append([]string{"inspect"}, names...)...)
	if err != nil {
		return nil, err
	}
	got, err := decodeStatsAll(out, conf)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for name, s := range got {
		s.At = now
		got[name] = s
	}
	return got, nil
}

// decodeStatsAll reads one stats call and the configuration of the same
// containers into one reading per container, keyed by its name.
func decodeStatsAll(stats, conf []byte) (map[string]ContainerStats, error) {
	var raw []struct {
		ID               string `json:"id"`
		CPUUsageUsec     uint64 `json:"cpuUsageUsec"`
		MemoryUsageBytes uint64 `json:"memoryUsageBytes"`
		MemoryLimitBytes uint64 `json:"memoryLimitBytes"`
		BlockReadBytes   uint64 `json:"blockReadBytes"`
		BlockWriteBytes  uint64 `json:"blockWriteBytes"`
		NetworkRxBytes   uint64 `json:"networkRxBytes"`
		NetworkTxBytes   uint64 `json:"networkTxBytes"`
		NumProcesses     int    `json:"numProcesses"`
	}
	if err := json.Unmarshal(stats, &raw); err != nil {
		return nil, fmt.Errorf("reading container statistics: %w", err)
	}
	var given []struct {
		Configuration struct {
			ID        string `json:"id"`
			Resources struct {
				CPUs int `json:"cpus"`
			} `json:"resources"`
		} `json:"configuration"`
	}
	if err := json.Unmarshal(conf, &given); err != nil {
		return nil, fmt.Errorf("reading container configuration: %w", err)
	}
	cpus := map[string]int{}
	for _, g := range given {
		cpus[g.Configuration.ID] = g.Configuration.Resources.CPUs
	}
	out := make(map[string]ContainerStats, len(raw))
	for _, r := range raw {
		out[r.ID] = ContainerStats{
			CPUUsageUsec: r.CPUUsageUsec, MemoryUsage: r.MemoryUsageBytes,
			MemoryLimit: r.MemoryLimitBytes, BlockRead: r.BlockReadBytes,
			BlockWrite: r.BlockWriteBytes, NetRx: r.NetworkRxBytes,
			NetTx: r.NetworkTxBytes, Processes: r.NumProcesses, CPUs: cpus[r.ID],
		}
	}
	return out, nil
}

// CPUShare returns the share of the container's cores used between two
// readings, in percent. It reports false for a single reading, and for a CPU
// time that went down, which is a container restarted between the readings
// rather than negative use.
func CPUShare(prev, cur ContainerStats, cpus int) (float64, bool) {
	if prev.At.IsZero() || cur.At.IsZero() || cpus <= 0 {
		return 0, false
	}
	elapsed := cur.At.Sub(prev.At)
	if elapsed <= 0 || cur.CPUUsageUsec < prev.CPUUsageUsec {
		return 0, false
	}
	used := float64(cur.CPUUsageUsec - prev.CPUUsageUsec)
	return used / float64(elapsed.Microseconds()) / float64(cpus) * 100, true
}
