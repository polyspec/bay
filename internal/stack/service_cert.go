package stack

import (
	"os"
	"path/filepath"
	"strings"
)

// Every service is given a certificate for its own name, as a routed domain is:
// the machine's authority issues it for <container>.bay.test, the container
// name and the service's domains, for use as a server and as a client. A
// service that speaks TLS itself, such as Redis, MySQL, MariaDB or PostgreSQL,
// points its configuration at the mounted files, and every other service
// verifies it, because every container trusts the authority.
//
// The files are mounted read-only, one file each:
//
//	/etc/bay/tls/cert.pem  the certificate
//	/etc/bay/tls/key.pem   its key
//	/etc/bay/tls/ca.pem    the authority
//
// The key is readable by every user in the container, because the process of
// an image runs as its own user and Apple `container` shows every mounted file
// as owned by root. PostgreSQL refuses a key other users can read; its command
// copies the key to a file it owns first (see the schema). A service whose
// Compose file sets x-bay.certificate: false is given none of this.

const (
	ServiceTLSDir  = "/etc/bay/tls"
	serviceCertDir = "services"
)

// ServiceCertificate is where a service's certificate is, on the host and in
// its container.
type ServiceCertificate struct {
	Names []string `json:"names"`
	// Cert, Key and CA are the host paths.
	Cert string `json:"cert"`
	Key  string `json:"key"`
	CA   string `json:"ca"`
	// ContainerCert, ContainerKey and ContainerCA are the paths in the
	// container.
	ContainerCert string `json:"containerCert"`
	ContainerKey  string `json:"containerKey"`
	ContainerCA   string `json:"containerCa"`
}

// ServiceCertNames are the names a service's certificate covers.
func ServiceCertNames(s *Service) []string {
	names := []string{s.ContainerName + "." + BackendDomain, s.ContainerName}
	for _, d := range s.Domains {
		names = append(names, strings.ToLower(d))
	}
	return names
}

// ServiceCert returns where a service's certificate is.
func ServiceCert(dir string, s *Service) ServiceCertificate {
	base := filepath.Join(dir, serviceCertDir, s.ContainerName)
	return ServiceCertificate{
		Names: ServiceCertNames(s),
		Cert:  base + ".crt", Key: base + ".key", CA: filepath.Join(dir, "ca.crt"),
		ContainerCert: ServiceTLSDir + "/cert.pem",
		ContainerKey:  ServiceTLSDir + "/key.pem",
		ContainerCA:   ServiceTLSDir + "/ca.pem",
	}
}

// ensureServiceCert issues the service's certificate when it is missing, has
// 30 days or less left, or does not cover the service's names. A certificate
// written again keeps its file, so a running container's mount reads it.
func ensureServiceCert(dir string, s *Service) error {
	if !s.Certificate || dir == "" {
		return nil
	}
	ca, err := LoadOrCreateCA(dir)
	if err != nil {
		return err
	}
	certDir := filepath.Join(dir, serviceCertDir)
	if _, err := ca.IssueWithNames(certDir, s.ContainerName, ServiceCertNames(s)); err != nil {
		return err
	}
	return os.Chmod(ServiceCert(dir, s).Key, 0o644)
}

// certArguments are the mounts that give a service its certificate. A path the
// service mounts itself is left to the service.
func certArguments(dir string, s *Service) []string {
	if !s.Certificate || dir == "" {
		return nil
	}
	c := ServiceCert(dir, s)
	if _, err := os.Stat(c.Cert); err != nil {
		return nil
	}
	taken := map[string]bool{}
	for _, v := range s.Volumes {
		if parts := strings.Split(v, ":"); len(parts) >= 2 {
			taken[parts[1]] = true
		}
	}
	var args []string
	for _, m := range [][2]string{{c.Cert, c.ContainerCert}, {c.Key, c.ContainerKey}, {c.CA, c.ContainerCA}} {
		if !taken[m[1]] {
			args = append(args, "--volume", m[0]+":"+m[1]+":ro")
		}
	}
	return args
}
