package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinkPartOnlyForAPHYTheKernelDrives(t *testing.T) {
	saved := sysClassNet
	t.Cleanup(func() { sysClassNet = saved })
	sysClassNet = t.TempDir()
	os.MkdirAll(filepath.Join(sysClassNet, "eth0"), 0o755)
	if items := (LinuxSetup{Interface: "eth0"}).linkItems(); items != nil {
		t.Fatalf("an interface without a PHY of its own has no part: %+v", items)
	}
	os.WriteFile(filepath.Join(sysClassNet, "eth0", "phydev"), nil, 0o644)
	if !hasPHY("eth0") {
		t.Fatal("phydev present: the kernel drives the PHY")
	}
	body := linkUnitBody("/usr/sbin/ethtool", "eth0")
	for _, want := range []string{"ExecStart=/usr/sbin/ethtool --set-eee eth0 eee off", "BindsTo=sys-subsystem-net-devices-eth0.device", "WantedBy=multi-user.target"} {
		if !strings.Contains(body, want) {
			t.Errorf("unit lacks %q:\n%s", want, body)
		}
	}
}
