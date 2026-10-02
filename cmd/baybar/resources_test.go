package main

import (
	"strings"
	"testing"
	"time"

	"github.com/polyspec/bay/internal/stack"
)

func useSection(p panel) (section, bool) {
	for _, s := range p.Sections {
		if s.Header == text.T("RESOURCES") {
			return s, true
		}
	}
	return section{}, false
}

func detailOf(s section, key string) string {
	for _, r := range s.Rows {
		if r.Text == key {
			return r.Detail
		}
	}
	return ""
}

func runningService() (stack.GroupStatus, stack.ServiceStatus) {
	s := stack.ServiceStatus{Name: "node-web", State: "running", IPv4: "192.168.65.88",
		Container: "soksakim-hyper-node-web", Internal: true}
	return stack.GroupStatus{Name: "soksakim-hyper", Services: []stack.ServiceStatus{s}}, s
}

// The figures measured for node-web, read twice two seconds apart.
func measuredUse() serviceUse {
	at := time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC)
	prev := stack.ContainerStats{CPUUsageUsec: 92240, At: at, CPUs: 4}
	cur := stack.ContainerStats{CPUUsageUsec: 2_092_240, MemoryUsage: 19939328,
		MemoryLimit: 1073741824, BlockRead: 13361152, BlockWrite: 12288,
		NetRx: 1219495, NetTx: 749637, Processes: 6, CPUs: 4, At: at.Add(2 * time.Second)}
	return serviceUse{Prev: prev, Cur: cur}
}

func TestARunningServiceShowsWhatItUses(t *testing.T) {
	g, s := runningService()
	p := panel{}
	serviceView(&p, stack.Snapshot{}, g, s, false, nil, measuredUse())
	use, ok := useSection(p)
	if !ok {
		t.Fatal("no resources section")
	}
	for key, want := range map[string]string{
		text.T("CPU"):       text.T("%s of %d cores", "25%", 4),
		text.T("Memory"):    text.T("%s of %s", "19.0 MiB", "1.0 GiB"),
		text.T("Disk I/O"):  text.T("%s read · %s written", "12.7 MiB", "12.0 KiB"),
		text.T("Network"):   text.T("%s received · %s sent", "1.2 MiB", "732.1 KiB"),
		text.T("Processes"): "6",
	} {
		if got := detailOf(use, key); got != want {
			t.Errorf("%s: %q, want %q", key, got, want)
		}
	}
}

// CPU needs two readings, so the first says it is measuring.
func TestTheFirstReadingSaysCPUIsBeingMeasured(t *testing.T) {
	g, s := runningService()
	u := measuredUse()
	u.Prev = stack.ContainerStats{}
	p := panel{}
	serviceView(&p, stack.Snapshot{}, g, s, false, nil, u)
	use, _ := useSection(p)
	if got := detailOf(use, text.T("CPU")); got != text.T("measuring") {
		t.Errorf("CPU %q, want measuring", got)
	}
}

// The disk a container occupies is not reported per container, and a row that
// says a figure is missing tells the reader nothing, so there is none.
func TestThereIsNoRowForAFigureTheEngineDoesNotReport(t *testing.T) {
	g, s := runningService()
	p := panel{}
	serviceView(&p, stack.Snapshot{}, g, s, false, nil, measuredUse())
	use, _ := useSection(p)
	want := []string{text.T("CPU"), text.T("Memory"), text.T("Disk I/O"), text.T("Network"), text.T("Processes")}
	if len(use.Rows) != len(want) {
		t.Fatalf("%d rows, want exactly %v", len(use.Rows), want)
	}
	for i, r := range use.Rows {
		if r.Text != want[i] {
			t.Errorf("row %d is %q, want %q", i, r.Text, want[i])
		}
	}
}

func TestAServiceThatIsNotRunningShowsNoUse(t *testing.T) {
	g, s := runningService()
	s.State = "stopped"
	p := panel{}
	serviceView(&p, stack.Snapshot{}, g, s, false, nil, measuredUse())
	if _, ok := useSection(p); ok {
		t.Error("a stopped service shows resource use")
	}
}

// A reading that failed says why rather than showing nothing.
func TestAFailedReadingSaysWhy(t *testing.T) {
	g, s := runningService()
	p := panel{}
	serviceView(&p, stack.Snapshot{}, g, s, false, nil, serviceUse{Err: "resource use is read from Apple container only"})
	use, ok := useSection(p)
	if !ok || !strings.Contains(use.Note, "Apple container only") {
		t.Errorf("the reason is not shown: %+v", use)
	}
}

func TestBytesAreBinaryUnits(t *testing.T) {
	for n, want := range map[uint64]string{0: "0 B", 512: "512 B", 12288: "12.0 KiB",
		1073741824: "1.0 GiB"} {
		if got := ibytes(n); got != want {
			t.Errorf("ibytes(%d) = %q, want %q", n, got, want)
		}
	}
}

// A project's list gives each running service's CPU and memory in its row, from
// the same reading as the service's own screen.
func TestTheProjectListGivesEachRunningServicesUse(t *testing.T) {
	g, s := runningService()
	stopped := stack.ServiceStatus{Name: "runtime", State: "stopped", Container: "soksakim-hyper-runtime", Internal: true}
	g.Services = append(g.Services, stopped)
	uses := map[string]serviceUse{s.Container: measuredUse()}
	p := panel{}
	projectView(&p, g, false, uses)
	var first, other row
	for _, sec := range p.Sections {
		for _, r := range sec.Rows {
			switch r.Text {
			case s.Name:
				first = r
			case stopped.Name:
				other = r
			}
		}
	}
	want := text.T("%s CPU · %s", "25%", "19.0 MiB") + " · " + s.IPv4
	if first.Detail != want {
		t.Errorf("running row %q, want %q", first.Detail, want)
	}
	if other.Detail != "stopped" {
		t.Errorf("a stopped service's row %q, want its state alone", other.Detail)
	}
}

// Before the second reading, a row gives memory and leaves CPU out rather than
// writing a word that says it is missing.
func TestARowBeforeTheSecondReadingGivesMemoryOnly(t *testing.T) {
	g, s := runningService()
	u := measuredUse()
	u.Prev = stack.ContainerStats{}
	p := panel{}
	projectView(&p, g, false, map[string]serviceUse{s.Container: u})
	for _, sec := range p.Sections {
		for _, r := range sec.Rows {
			if r.Text == s.Name && r.Detail != "19.0 MiB · "+s.IPv4 {
				t.Errorf("row %q, want memory and address", r.Detail)
			}
		}
	}
}

// A service serving several domains is one row, and its domains are listed in
// that row.
func TestAServiceWithSeveralDomainsIsOneRow(t *testing.T) {
	s := stack.ServiceStatus{Name: "node-web", State: "running", IPv4: "192.168.65.88", Routed: true,
		Container: "soksakim-hyper-node-web",
		Domains:   []string{"node.soksakim.test", "console.node.soksakim.test", "example.node.soksakim.test"},
		URLs: []string{"https://node.soksakim.test/", "https://console.node.soksakim.test/",
			"https://example.node.soksakim.test/"}}
	g := stack.GroupStatus{Name: "soksakim-hyper", Services: []stack.ServiceStatus{s}}
	p := panel{}
	projectView(&p, g, false, map[string]serviceUse{s.Container: measuredUse()})
	var rows []row
	for _, sec := range p.Sections {
		if sec.Header == text.T("SERVICES") {
			rows = sec.Rows
		}
	}
	if len(rows) != 1 {
		t.Fatalf("%d rows, want one for the service", len(rows))
	}
	if strings.Join(rows[0].Links, " ") != strings.Join(s.URLs, " ") {
		t.Errorf("links %v, want %v", rows[0].Links, s.URLs)
	}
	if !strings.Contains(rows[0].Detail, "192.168.65.88") {
		t.Errorf("the row lost its address: %q", rows[0].Detail)
	}
}
