package stack

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMemoryReserveFollowsTheMachine(t *testing.T) {
	saved := memInfoPath
	t.Cleanup(func() { memInfoPath = saved })
	for _, c := range []struct{ total, want int }{
		{3858692, 60292},     // the ODROID-N2+: 1/64
		{1 << 20, 32 * 1024}, // 1 GiB: at least 32 MiB
		{64 << 20, 256 * 1024},
	} {
		memInfoPath = filepath.Join(t.TempDir(), "meminfo")
		os.WriteFile(memInfoPath, []byte("MemTotal:       "+strconv.Itoa(c.total)+" kB\nMemFree: 1 kB\n"), 0o644)
		got, err := memoryReserveKB()
		if err != nil || got != c.want {
			t.Errorf("MemTotal %d kB: reserve %d, %v; want %d", c.total, got, err, c.want)
		}
	}
}


func TestMemoryReserveItemReadsTheLiveValues(t *testing.T) {
	savedInfo, savedDir, savedVM := memInfoPath, sysctlDir, procSysVM
	t.Cleanup(func() { memInfoPath, sysctlDir, procSysVM = savedInfo, savedDir, savedVM })
	dir := t.TempDir()
	memInfoPath = filepath.Join(dir, "meminfo")
	os.WriteFile(memInfoPath, []byte("MemTotal: 3858692 kB\n"), 0o644)
	sysctlDir = filepath.Join(dir, "sysctl.d")
	procSysVM = filepath.Join(dir, "vm")
	os.MkdirAll(sysctlDir, 0o755)
	os.MkdirAll(procSysVM, 0o755)

	if memoryReserveItem().OK {
		t.Fatal("nothing written: not in place")
	}
	os.WriteFile(memoryReservePath(), []byte(memoryReserveBody(60292)), 0o644)
	os.WriteFile(filepath.Join(procSysVM, "min_free_kbytes"), []byte("22528\n"), 0o644)
	os.WriteFile(filepath.Join(procSysVM, "watermark_scale_factor"), []byte("200\n"), 0o644)
	if memoryReserveItem().OK {
		t.Fatal("written but not loaded: not in place")
	}
	os.WriteFile(filepath.Join(procSysVM, "min_free_kbytes"), []byte("60292\n"), 0o644)
	if it := memoryReserveItem(); !it.OK {
		t.Fatalf("written and loaded: %+v", it)
	}
}

func TestContainersLeaveTheSliceAloneWhereTheHostHasNone(t *testing.T) {
	saved := hostSetsUpNames
	t.Cleanup(func() { hostSetsUpNames = saved })
	hostSetsUpNames = true
	if got := serviceCgroupParent(); got != "" {
		t.Fatalf("macOS: cgroup parent %q, want none", got)
	}
	body := containerSliceBody()
	for _, want := range []string{"MemoryHigh=75%", "MemoryMax=85%", "MemorySwapMax=25%"} {
		if !strings.Contains(body, want) {
			t.Errorf("slice lacks %s:\n%s", want, body)
		}
	}
}
