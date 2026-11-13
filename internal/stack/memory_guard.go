package stack

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// A Linux machine that runs projects for others, such as a board reached
// through a Mac, has to stay reachable while they use it: slow is acceptable,
// unreachable is not. Two parts of the setup hold it there.
//
// The containers bay runs are placed in one systemd slice. Past MemoryHigh the
// kernel reclaims from them and throttles them, so they slow down; at
// MemoryMax the kernel kills inside the slice, never outside it. What is left
// stays with the system: the shell, Docker, the agent and the network driver.
//
// The kernel keeps a reserve of free memory (vm.min_free_kbytes) for
// allocations that cannot wait, among them the receive buffers of the network
// driver, and starts reclaiming earlier (vm.watermark_scale_factor). On the
// ODROID the default reserve was 22 MiB; under pressure the driver stopped
// receiving while the link stayed up, until the link was brought down and up.

const (
	// containerSlice holds the containers bay runs on Docker.
	containerSlice = "bay.slice"
	// containerMemoryHigh, containerMemoryMax and containerSwapMax are shares
	// of the machine's memory, as systemd reads them.
	containerMemoryHigh = "75%"
	containerMemoryMax  = "85%"
	containerSwapMax    = "25%"
	// watermarkScale starts reclaim when 2% of memory is left above the low
	// watermark, instead of the default 0.1%.
	watermarkScale = 200
)

// sysctlDir is where the reserve is written. Tests replace it.
var sysctlDir = "/etc/sysctl.d"

// memInfoPath is read for the machine's memory. Tests replace it.
var memInfoPath = "/proc/meminfo"

// procSysVM is where the live values are read. Tests replace it.
var procSysVM = "/proc/sys/vm"

func containerSlicePath() string { return filepath.Join(systemdDir, containerSlice) }
func memoryReservePath() string  { return filepath.Join(sysctlDir, "99-bay.conf") }

func containerSliceBody() string {
	return "# Written by \"bay install\".\n" +
		"[Unit]\n" +
		"Description=bay: the containers bay runs, held below the machine's memory\n" +
		"\n" +
		"[Slice]\n" +
		"MemoryAccounting=yes\n" +
		"MemoryHigh=" + containerMemoryHigh + "\n" +
		"MemoryMax=" + containerMemoryMax + "\n" +
		"MemorySwapMax=" + containerSwapMax + "\n"
}

// memoryReserveKB is the free memory the kernel keeps: 1/64 of the machine's
// memory, at least 32 MiB and at most 256 MiB.
func memoryReserveKB() (int, error) {
	b, err := os.ReadFile(memInfoPath)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "MemTotal:"); ok {
			total, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(v), " kB"))
			if err != nil {
				return 0, err
			}
			return min(max(total/64, 32*1024), 256*1024), nil
		}
	}
	return 0, fmt.Errorf("no MemTotal in %s", memInfoPath)
}

func memoryReserveBody(kb int) string {
	return "# Written by \"bay install\": keep the machine reachable under memory pressure.\n" +
		"vm.min_free_kbytes = " + strconv.Itoa(kb) + "\n" +
		"vm.watermark_scale_factor = " + strconv.Itoa(watermarkScale) + "\n"
}

func readVM(name string) int {
	b, err := os.ReadFile(filepath.Join(procSysVM, name))
	if err != nil {
		return -1
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return -1
	}
	return v
}

// memoryReserveItem is the kernel's reserve, written and in force.
func memoryReserveItem() SetupItem {
	kb, err := memoryReserveKB()
	if err != nil {
		return SetupItem{Name: "memory reserve: " + err.Error()}
	}
	file, _ := os.ReadFile(memoryReservePath())
	return SetupItem{
		Name: fmt.Sprintf("memory reserve %d MiB, reclaim from %d.%d%% (%s)",
			kb/1024, watermarkScale/100, watermarkScale%100/10, memoryReservePath()),
		OK: string(file) == memoryReserveBody(kb) &&
			readVM("min_free_kbytes") == kb && readVM("watermark_scale_factor") == watermarkScale,
		SetupStep: SetupStep{Text: "write and load " + memoryReservePath(), Asks: AsksAdministrator},
	}
}

func applyMemoryReserve() error {
	kb, err := memoryReserveKB()
	if err != nil {
		return err
	}
	if err := os.WriteFile(memoryReservePath(), []byte(memoryReserveBody(kb)), 0o644); err != nil {
		return err
	}
	if out, err := exec.Command("sysctl", "-p", memoryReservePath()).CombinedOutput(); err != nil {
		return fmt.Errorf("sysctl -p %s: %s", memoryReservePath(), strings.TrimSpace(string(out)))
	}
	fmt.Printf("loaded %s\n", memoryReservePath())
	return nil
}

// dockerCgroupDriver is the cgroup driver the Docker engine uses. Tests
// replace it.
var dockerCgroupDriver = func() (string, error) {
	out, err := runEngineWithin(engineReadWait, DockerEngine, "info", "--format", "{{.CgroupDriver}}")
	return strings.TrimSpace(string(out)), err
}

// containerSliceItem is the slice, registered with systemd, and a Docker
// engine that places containers in systemd slices.
func containerSliceItem() SetupItem {
	name := "containers held below " + containerMemoryMax + " of memory, slowed from " +
		containerMemoryHigh + " (" + containerSlice + ")"
	if !systemdRuns() {
		return SetupItem{Name: name + ": systemd does not run this system; install cannot hold them"}
	}
	if driver, err := dockerCgroupDriver(); err != nil {
		return SetupItem{Name: name + ": the Docker engine did not say its cgroup driver: " + err.Error()}
	} else if driver != "systemd" {
		return SetupItem{Name: name + ": the Docker engine uses the " + driver +
			" cgroup driver; set \"exec-opts\": [\"native.cgroupdriver=systemd\"] in /etc/docker/daemon.json"}
	}
	file, _ := os.ReadFile(containerSlicePath())
	return SetupItem{
		Name:      name,
		OK:        string(file) == containerSliceBody() && sliceLoaded(),
		SetupStep: SetupStep{Text: "register " + containerSlicePath(), Asks: AsksAdministrator},
	}
}

// sliceLoaded reports whether systemd has read the slice as it is written.
func sliceLoaded() bool {
	out, err := exec.Command("systemctl", "show", "--property=LoadState,NeedDaemonReload", containerSlice).Output()
	return err == nil && strings.Contains(string(out), "LoadState=loaded") &&
		strings.Contains(string(out), "NeedDaemonReload=no")
}

func applyContainerSlice() error {
	if err := os.WriteFile(containerSlicePath(), []byte(containerSliceBody()), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"start", containerSlice}} {
		if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
		}
	}
	fmt.Printf("registered %s\n", containerSlicePath())
	return nil
}

// removeMemoryGuard removes both parts. The kernel's values are read again
// from every file that remains, so what another file sets is restored.
func removeMemoryGuard() error {
	if _, err := os.Stat(containerSlicePath()); err == nil {
		if err := os.Remove(containerSlicePath()); err != nil {
			return err
		}
		exec.Command("systemctl", "daemon-reload").Run()
		fmt.Printf("removed %s\n", containerSlicePath())
	}
	if _, err := os.Stat(memoryReservePath()); err == nil {
		if err := os.Remove(memoryReservePath()); err != nil {
			return err
		}
		exec.Command("sysctl", "--system").Run()
		fmt.Printf("removed %s\n", memoryReservePath())
	}
	return nil
}

// serviceCgroupParent is the slice a service container is created in: the
// containers' slice on a Linux machine whose setup registered it, and none
// elsewhere. Docker Desktop and Apple container run containers in a virtual
// machine, which the host's slices do not reach.
func serviceCgroupParent() string {
	if hostSetsUpNames || ServiceEngine() != DockerEngine {
		return ""
	}
	if _, err := os.Stat(containerSlicePath()); err != nil {
		return ""
	}
	return containerSlice
}
