package stack

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// A machine tells its approved peers how busy it is: the share of its
// processors in use, and its memory and swap. The agent samples them and
// writes stats.json beside the peer document, which the link serves at
// StatsPath to a client holding a certificate from an approved authority, so
// a machine that is not approved learns nothing of it. The dashboard of a
// peer shows them on that machine's card.

// StatsPath is where the link serves the machine's load.
const StatsPath = "/__bay/stats"

// StatsName is the file the agent writes in the peer directory.
const StatsName = "stats.json"

// StatsInterval is how often the agent samples the load.
const StatsInterval = 5 * time.Second

// MachineStats is a machine's load at one moment.
type MachineStats struct {
	// CPU is the share of all processors in use over the last interval, in
	// percent.
	CPU       float64   `json:"cpu"`
	CPUs      int       `json:"cpus"`
	MemUsed   uint64    `json:"memUsed"`
	MemTotal  uint64    `json:"memTotal"`
	SwapUsed  uint64    `json:"swapUsed"`
	SwapTotal uint64    `json:"swapTotal"`
	At        time.Time `json:"at"`
}

// cpuTimes is a reading of the processor counters: the time spent busy and in
// total since boot, in the system's units.
type cpuTimes struct{ busy, total uint64 }

// StatsSampler samples a machine's load; the processor share needs two
// readings, so it keeps the previous one.
type StatsSampler struct{ prev *cpuTimes }

// Sample reads the load now.
func (s *StatsSampler) Sample() (MachineStats, error) {
	st := MachineStats{CPUs: runtime.NumCPU(), At: time.Now().UTC().Truncate(time.Second)}
	if err := readMemory(&st); err != nil {
		return st, err
	}
	cpu, err := sampleCPU(s)
	if err != nil {
		return st, err
	}
	st.CPU = cpu
	return st, nil
}

// shareSince returns the busy share between two counter readings and keeps
// the new one; the first reading has no share yet.
func (s *StatsSampler) shareSince(now cpuTimes) float64 {
	share := 0.0
	if s.prev != nil && now.total > s.prev.total {
		share = float64(now.busy-s.prev.busy) * 100 / float64(now.total-s.prev.total)
	}
	s.prev = &now
	return share
}

// WriteStats writes the load beside the peer document of each engine's
// proxy, where the link serves it. The peer directory is not used: it holds
// the client key and is readable by no one else, the proxy's workers included.
func WriteStats(dir string, st MachineStats) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	m := NewMachine(dir)
	for _, engine := range []string{AppleEngine, DockerEngine} {
		conf := m.ConfDir(engine)
		if _, err := os.Stat(filepath.Join(conf, PeerDocumentName)); err != nil {
			continue // this engine's proxy serves no link
		}
		if err := writeInPlace(filepath.Join(conf, StatsName), append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// FetchPeerStats reads an approved peer's load over its link, presenting this
// machine's client certificate and checking the peer against its authority.
func FetchPeerStats(dir string, p Peer, timeout time.Duration) (MachineStats, error) {
	var st MachineStats
	peerDir := PeerDir(dir)
	cert, err := tls.LoadX509KeyPair(filepath.Join(peerDir, ClientCertName+".crt"), filepath.Join(peerDir, ClientCertName+".key"))
	if err != nil {
		return st, err
	}
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			// The link answers with its default certificate, whose names are
			// not the address; the chain is checked against the authority
			// approved for the peer below instead.
			InsecureSkipVerify: true,
			VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
				if len(raw) == 0 {
					return fmt.Errorf("no certificate")
				}
				c, err := x509.ParseCertificate(raw[0])
				if err != nil {
					return err
				}
				return provesAuthority(c, p.CA)
			},
		}},
	}
	resp, err := client.Get("https://" + p.Address + StatsPath)
	if err != nil {
		return st, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return st, fmt.Errorf("%s answered %s", p.Address, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return st, err
	}
	return st, json.Unmarshal(body, &st)
}
