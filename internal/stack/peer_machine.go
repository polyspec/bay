package stack

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// PeerPort is where a machine's link answers. It is fixed, so a peer is reached
// by an address alone.
const PeerPort = 8443

// PeerDocumentName is the file the link serves at PeerPath. It is written into
// the configuration directory, which the proxy already reads: nginx loads only
// *.conf from it, so a document beside them is served and not loaded.
const PeerDocumentName = "peer.json"

// PeerDocument is what a machine tells another about itself. It answers before
// there is any approval, so it carries the authority the approval is given to.
type PeerDocument struct {
	Name    string   `json:"name"`
	Address string   `json:"address"`
	Domains []string `json:"domains"`
	CA      string   `json:"ca"`
}

// MachineName returns the name macOS keeps for this machine on the network. It
// is shown and nothing is decided by it.
func MachineName() string {
	if out, err := exec.Command("scutil", "--get", "LocalHostName").Output(); err == nil {
		if name := strings.TrimSpace(string(out)); name != "" {
			return name
		}
	}
	name, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSuffix(name, ".local")
}

// LANAddress returns the address other machines on the network reach this one
// at. The container network is left out: it exists only on this machine.
func LANAddress() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	_, containers, _ := net.ParseCIDR("192.168.64.0/24")
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := n.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		if containers != nil && containers.Contains(ip) {
			continue
		}
		return ip.String()
	}
	return ""
}

// WritePeerDocument writes what this machine tells a peer about itself.
func WritePeerDocument(confDir string, doc PeerDocument) error {
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(confDir, PeerDocumentName), append(b, '\n'), 0o644)
}

// PeerLinkAddress returns the address a machine's link answers at.
func PeerLinkAddress(host string) string {
	if host == "" {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(PeerPort))
}
