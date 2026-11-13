package stack

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Energy-efficient ethernet (IEEE 802.3az) lets the link idle in a low-power
// state between frames. On the ODROID-N2+ (stmmac with an RTL8211F PHY) a wake
// from that state failed (phy_eee_wakeup_error), and the link dropped several
// times a day, once for eight minutes with the link reported up. A machine
// reached as a peer is worth the fraction of a watt, so the setup keeps
// energy-efficient ethernet off on the interface that holds the default route.
//
// The part covers interfaces whose PHY the kernel's phylib drives (they have a
// phydev link in sysfs), which is where the setting lives; a virtual interface,
// such as a cloud machine's, has none and nothing to keep off. The setting is
// lost when the machine starts, so a systemd unit applies it at boot.
// Uninstalling removes the unit; the interface keeps the setting until the
// next boot, when the driver's default returns.

const linkUnit = "bay-link.service"

func linkUnitPath() string { return filepath.Join(systemdDir, linkUnit) }

// sysClassNet is where interfaces are read. Tests replace it.
var sysClassNet = "/sys/class/net"

// hasPHY reports whether phylib drives the interface's PHY.
func hasPHY(iface string) bool {
	_, err := os.Stat(filepath.Join(sysClassNet, iface, "phydev"))
	return err == nil
}

func linkUnitBody(ethtool, iface string) string {
	device := "sys-subsystem-net-devices-" + iface + ".device"
	return "# Written by \"bay install\".\n" +
		"[Unit]\n" +
		"Description=bay: keep energy-efficient ethernet off on " + iface + ", whose wake from low-power idle drops the link\n" +
		"BindsTo=" + device + "\n" +
		"After=" + device + "\n" +
		"\n" +
		"[Service]\n" +
		"Type=oneshot\n" +
		"RemainAfterExit=yes\n" +
		"ExecStart=" + ethtool + " --set-eee " + iface + " eee off\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=multi-user.target\n"
}

// eeeOff reports whether energy-efficient ethernet is off on the interface,
// as ethtool reads it.
func eeeOff(ethtool, iface string) bool {
	out, err := exec.Command(ethtool, "--show-eee", iface).Output()
	return err == nil && strings.Contains(string(out), "EEE status: disabled")
}

// linkItems is the part for the interface holding the default route, or none
// when the interface has no PHY of its own.
func (l LinuxSetup) linkItems() []SetupItem {
	if l.Interface == "" || !hasPHY(l.Interface) {
		return nil
	}
	name := "energy-efficient ethernet off on " + l.Interface
	if !systemdRuns() {
		return []SetupItem{{Name: name + ": systemd does not run this system; install cannot keep it off at boot"}}
	}
	ethtool, err := exec.LookPath("ethtool")
	if err != nil {
		return []SetupItem{{Name: name + ": ethtool is not installed; install ethtool"}}
	}
	return []SetupItem{{
		Name: name, OK: l.linkCurrent(ethtool),
		SetupStep: SetupStep{Text: "register " + linkUnitPath() + " and turn energy-efficient ethernet off on " +
			l.Interface + " (the link renegotiates once)", Asks: AsksAdministrator},
	}}
}

func (l LinuxSetup) linkCurrent(ethtool string) bool {
	unit, err := os.ReadFile(linkUnitPath())
	if err != nil || string(unit) != linkUnitBody(ethtool, l.Interface) {
		return false
	}
	return exec.Command("systemctl", "is-enabled", "--quiet", linkUnit).Run() == nil && eeeOff(ethtool, l.Interface)
}

func (l LinuxSetup) applyLink() error {
	ethtool, err := exec.LookPath("ethtool")
	if err != nil {
		return err
	}
	if err := os.WriteFile(linkUnitPath(), []byte(linkUnitBody(ethtool, l.Interface)), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", linkUnit}, {"restart", linkUnit}} {
		if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
		}
	}
	if !eeeOff(ethtool, l.Interface) {
		return fmt.Errorf("energy-efficient ethernet is still on on %s after %s ran", l.Interface, linkUnit)
	}
	fmt.Printf("registered %s; energy-efficient ethernet is off on %s\n", linkUnitPath(), l.Interface)
	return nil
}

func removeLink() error {
	if _, err := os.Stat(linkUnitPath()); err != nil {
		return nil
	}
	exec.Command("systemctl", "disable", linkUnit).Run()
	if err := os.Remove(linkUnitPath()); err != nil {
		return err
	}
	exec.Command("systemctl", "daemon-reload").Run()
	fmt.Printf("removed %s\n", linkUnitPath())
	return nil
}
