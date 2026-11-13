package stack

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// bay 0.0.6 kept energy-efficient ethernet off on a Linux machine's uplink,
// through bay-link.service, taking the ODROID's link drops for failed wakes
// from low-power idle. They were not: the Mac's USB network adapter left the
// USB bus and came back (07:55:32.96), and the ODROID saw its link go down a
// moment later (07:55:33). The part had no cause left, so the setup retires
// it: install removes the unit and turns energy-efficient ethernet on again,
// as the driver had it.

const linkUnit = "bay-link.service"

func linkUnitPath() string { return filepath.Join(systemdDir, linkUnit) }

// retiredLinkItems is the retirement, while the unit is there.
func retiredLinkItems() []SetupItem {
	if _, err := os.Stat(linkUnitPath()); err != nil {
		return nil
	}
	return []SetupItem{{
		Name: "retired " + linkUnit + " (energy-efficient ethernet was not why the link dropped)",
		SetupStep: SetupStep{Text: "remove " + linkUnitPath() + " and turn energy-efficient ethernet on again",
			Asks: AsksAdministrator},
	}}
}

// linkUnitInterface reads the interface the unit was written for.
func linkUnitInterface() string {
	b, err := os.ReadFile(linkUnitPath())
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(line, "ExecStart="); ok {
			f := strings.Fields(rest)
			for i, w := range f {
				if w == "--set-eee" && i+1 < len(f) {
					return f[i+1]
				}
			}
		}
	}
	return ""
}

// removeLink removes the unit and turns energy-efficient ethernet on again on
// the interface it named. It runs as root.
func removeLink() error {
	if _, err := os.Stat(linkUnitPath()); err != nil {
		return nil
	}
	iface := linkUnitInterface()
	exec.Command("systemctl", "disable", linkUnit).Run()
	if err := os.Remove(linkUnitPath()); err != nil {
		return err
	}
	exec.Command("systemctl", "daemon-reload").Run()
	if ethtool, err := exec.LookPath("ethtool"); err == nil && iface != "" {
		if out, err := exec.Command(ethtool, "--set-eee", iface, "eee", "on").CombinedOutput(); err != nil {
			fmt.Printf("energy-efficient ethernet on %s: %s\n", iface, strings.TrimSpace(string(out)))
		}
	}
	fmt.Printf("removed %s\n", linkUnitPath())
	return nil
}
