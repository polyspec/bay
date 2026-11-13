package stack

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/polyspec/bay/internal/contract"
)

// ProjectReport is what up --json prints: each service of a project, the
// name and address it is reached at, and its certificate.
type ProjectReport struct {
	SchemaVersion int             `json:"schemaVersion"`
	Project       string          `json:"project"`
	ComposeFile   string          `json:"composeFile"`
	Services      []ServiceReport `json:"services"`
}

// ServiceReport is one service of a ProjectReport.
type ServiceReport struct {
	ServiceStatus
	// Host is the name other services and this machine reach it at.
	Host        string              `json:"host"`
	Certificate *ServiceCertificate `json:"certificate,omitempty"`
}

// ReportProject reads the state of a project's services.
func ReportProject(m *Machine, addr string, cfg *Config) (ProjectReport, error) {
	snap, err := Take(m, addr)
	if err != nil {
		return ProjectReport{}, err
	}
	report := ProjectReport{SchemaVersion: contract.SchemaVersion, Project: cfg.Name, ComposeFile: cfg.Path()}
	var group *GroupStatus
	for i := range snap.Groups {
		if snap.Groups[i].Name == cfg.Name {
			group = &snap.Groups[i]
		}
	}
	if group == nil {
		return report, fmt.Errorf("project %s is not registered", cfg.Name)
	}
	byName := map[string]*Service{}
	for _, s := range cfg.Sorted() {
		byName[s.Name] = s
	}
	for _, st := range group.Services {
		r := ServiceReport{ServiceStatus: st, Host: st.Container + "." + BackendDomain}
		if s := byName[st.Name]; s != nil && s.Certificate {
			c := ServiceCert(m.Dir, s)
			if _, err := os.Stat(c.Cert); err == nil {
				r.Certificate = &c
			}
		}
		report.Services = append(report.Services, r)
	}
	return report, nil
}

// ClientCertificate is a client certificate written for a caller outside the
// containers, such as a test that connects with mutual TLS.
type ClientCertificate struct {
	Name string `json:"name"`
	Cert string `json:"cert"`
	Key  string `json:"key"`
	CA   string `json:"ca"`
}

// IssueClient writes a certificate for name, issued by the machine's authority
// for use as a client, with its key and the authority, to out as cert.pem,
// key.pem and ca.pem. An existing certificate with more than 30 days left is
// written again unchanged.
func IssueClient(dir, name, out string) (ClientCertificate, error) {
	if !validName(name) {
		return ClientCertificate{}, fmt.Errorf("client name %q must be letters, digits, - or _", name)
	}
	ca, err := LoadOrCreateCA(dir)
	if err != nil {
		return ClientCertificate{}, err
	}
	store := filepath.Join(dir, "clients")
	if _, err := ca.IssueWithNames(store, name, []string{name}); err != nil {
		return ClientCertificate{}, err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return ClientCertificate{}, err
	}
	c := ClientCertificate{Name: name, Cert: filepath.Join(out, "cert.pem"), Key: filepath.Join(out, "key.pem"), CA: filepath.Join(out, "ca.pem")}
	for _, f := range []struct {
		from, to string
		mode     os.FileMode
	}{
		{filepath.Join(store, name+".crt"), c.Cert, 0o644},
		{filepath.Join(store, name+".key"), c.Key, 0o600},
		{ca.CertPath(), c.CA, 0o644},
	} {
		b, err := os.ReadFile(f.from)
		if err != nil {
			return ClientCertificate{}, err
		}
		if err := os.WriteFile(f.to, b, f.mode); err != nil {
			return ClientCertificate{}, err
		}
	}
	return c, nil
}

// PlannedService is a service as up will create it, read from the Compose file
// alone. Its fields carry the names of the same fields in up --json.
type PlannedService struct {
	Name      string   `json:"name"`
	Container string   `json:"container"`
	Host      string   `json:"host"`
	Port      int      `json:"port,omitempty"`
	Domains   []string `json:"domains"`
	Image     string   `json:"image"`
	// Job is set for a service that runs when a job is queued, with its
	// schedule, if any, and its time limit.
	Job *PlannedJob `json:"job,omitempty"`
	// Certificate is where up writes the service's certificate, which exists
	// once up has run.
	Certificate *ServiceCertificate `json:"certificate,omitempty"`
}

// PlannedJob is a job service's schedule and time limit.
type PlannedJob struct {
	Schedule string `json:"schedule,omitempty"`
	Timeout  string `json:"timeout"`
}

// ProjectPlan is what plan --json prints.
type ProjectPlan struct {
	SchemaVersion int              `json:"schemaVersion"`
	Project       string           `json:"project"`
	ComposeFile   string           `json:"composeFile"`
	Services      []PlannedService `json:"services"`
}

// PlanProject returns the names, ports and certificate paths of a project's
// services as up will create them. It reads the Compose file only, so a
// configuration that needs a service's address, such as a replication setting
// or a certificate request, can be written before up.
func PlanProject(m *Machine, cfg *Config) ProjectPlan {
	plan := ProjectPlan{SchemaVersion: contract.SchemaVersion, Project: cfg.Name, ComposeFile: cfg.Path()}
	for _, s := range cfg.Sorted() {
		p := PlannedService{
			Name: s.Name, Container: s.ContainerName, Host: s.ContainerName + "." + BackendDomain,
			Port: s.Port, Domains: append([]string{}, s.Domains...), Image: s.Image,
		}
		if s.Job {
			p.Job = &PlannedJob{Schedule: s.Schedule, Timeout: s.JobTimeout.String()}
		}
		if s.Certificate {
			c := ServiceCert(m.Dir, s)
			p.Certificate = &c
		}
		plan.Services = append(plan.Services, p)
	}
	return plan
}
