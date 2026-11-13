package stack

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRetiredLinkUnitIsRemovedWhereItIs(t *testing.T) {
	saved := systemdDir
	t.Cleanup(func() { systemdDir = saved })
	systemdDir = t.TempDir()
	if items := retiredLinkItems(); items != nil {
		t.Fatalf("no unit: nothing to retire, got %+v", items)
	}
	unit := "[Service]\nExecStart=/usr/sbin/ethtool --set-eee eth0 eee off\n"
	os.WriteFile(filepath.Join(systemdDir, linkUnit), []byte(unit), 0o644)
	if items := retiredLinkItems(); len(items) != 1 || items[0].OK {
		t.Fatalf("unit present: one missing step, got %+v", items)
	}
	if got := linkUnitInterface(); got != "eth0" {
		t.Fatalf("interface %q", got)
	}
}
