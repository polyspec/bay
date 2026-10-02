package stack

import (
	"testing"
	"time"
)

// measuredStats is what `container stats --no-stream --format json` printed for
// soksakim-hyper-node-web on this machine.
const measuredStats = `[
    {
        "blockReadBytes": 13361152,
        "blockWriteBytes": 12288,
        "cpuUsageUsec": 92240,
        "id": "soksakim-hyper-node-web",
        "memoryLimitBytes": 1073741824,
        "memoryUsageBytes": 19939328,
        "networkRxBytes": 1219495,
        "networkTxBytes": 749637,
        "numProcesses": 6
    }
]`

func TestTheEnginesStatisticsAreRead(t *testing.T) {
	conf := `[{"configuration":{"id":"soksakim-hyper-node-web","resources":{"cpuOverhead":1,"cpus":4,"memoryInBytes":1073741824}}}]`
	got, err := decodeStatsAll([]byte(measuredStats), []byte(conf))
	if err != nil {
		t.Fatal(err)
	}
	want := ContainerStats{CPUUsageUsec: 92240, MemoryUsage: 19939328, MemoryLimit: 1073741824,
		BlockRead: 13361152, BlockWrite: 12288, NetRx: 1219495, NetTx: 749637, Processes: 6, CPUs: 4}
	if got["soksakim-hyper-node-web"] != want {
		t.Errorf("read %+v\nwant %+v", got["soksakim-hyper-node-web"], want)
	}
	if _, ok := got["another"]; ok {
		t.Error("statistics were reported for a container the engine did not name")
	}
}

// CPU is the change in CPU time between two readings over the time between
// them, as a share of the cores the container was given.
func TestCPUIsTheChangeBetweenTwoReadingsOverTheCoresGiven(t *testing.T) {
	at := time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC)
	prev := ContainerStats{CPUUsageUsec: 1_000_000, At: at}
	cur := ContainerStats{CPUUsageUsec: 3_000_000, At: at.Add(2 * time.Second)}
	// Two seconds of CPU in two seconds of time is one core in full; of four
	// cores that is a quarter.
	got, ok := CPUShare(prev, cur, 4)
	if !ok || got < 24.99 || got > 25.01 {
		t.Errorf("CPU share %.2f%% (%v), want 25%%", got, ok)
	}
}

func TestOneReadingGivesNoCPUShare(t *testing.T) {
	if _, ok := CPUShare(ContainerStats{}, ContainerStats{CPUUsageUsec: 5, At: time.Now()}, 4); ok {
		t.Error("a CPU share was given from one reading")
	}
}

// A container restarted between readings reports less CPU time than before.
// That is a new container, not negative use.
func TestAResetCounterGivesNoCPUShare(t *testing.T) {
	at := time.Now()
	if _, ok := CPUShare(ContainerStats{CPUUsageUsec: 9_000_000, At: at},
		ContainerStats{CPUUsageUsec: 10, At: at.Add(time.Second)}, 4); ok {
		t.Error("a counter that went backwards gave a CPU share")
	}
}

// measuredPair is one `container stats` call naming two containers, and the
// cores `container inspect` reported for them.
const measuredPair = `[{"blockReadBytes":98632704,"blockWriteBytes":3731456,"cpuUsageUsec":32654242,"id":"soksakim-hyper-php","memoryLimitBytes":1073741824,"memoryUsageBytes":300249088,"networkRxBytes":729677,"networkTxBytes":371095,"numProcesses":36},{"blockReadBytes":150488064,"blockWriteBytes":3764224,"cpuUsageUsec":95705034,"id":"soksakim-hyper-node","memoryLimitBytes":1073741824,"memoryUsageBytes":368582656,"networkRxBytes":682989,"networkTxBytes":372710,"numProcesses":44}]`

const measuredPairInspect = `[{"configuration":{"id":"soksakim-hyper-php","resources":{"cpus":4}}},{"configuration":{"id":"soksakim-hyper-node","resources":{"cpus":4}}}]`

// One call reads every container it names.
func TestOneReadingCoversEveryContainerNamed(t *testing.T) {
	got, err := decodeStatsAll([]byte(measuredPair), []byte(measuredPairInspect))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("read %d containers, want 2", len(got))
	}
	if got["soksakim-hyper-node"].MemoryUsage != 368582656 || got["soksakim-hyper-node"].CPUs != 4 {
		t.Errorf("node read as %+v", got["soksakim-hyper-node"])
	}
	if got["soksakim-hyper-php"].Processes != 36 || got["soksakim-hyper-php"].CPUs != 4 {
		t.Errorf("php read as %+v", got["soksakim-hyper-php"])
	}
}
