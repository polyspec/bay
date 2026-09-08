package stack

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This test runs nginx's configuration check without registering any route or
// touching the shared proxy, its certificates, DNS, or an existing container.
func TestRenderNginxLongNamesActualRuntime(t *testing.T) {
	if os.Getenv("BAY_SERVICE_E2E") != "1" {
		t.Skip("set BAY_SERVICE_E2E=1 for isolated nginx configuration checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, containerBin(), "image", "inspect", ProxyImage).Output()
	if err != nil {
		t.Fatalf("ProxyImage %s must already exist; this test does not pull images: %v", ProxyImage, err)
	}
	var images []struct {
		Configuration struct {
			Descriptor struct{ Digest string } `json:"descriptor"`
		} `json:"configuration"`
	}
	if err := json.Unmarshal(out, &images); err != nil || len(images) != 1 {
		t.Fatal("ProxyImage inspection did not return exactly one image")
	}
	digest := images[0].Configuration.Descriptor.Digest
	decoded, err := hex.DecodeString(strings.TrimPrefix(digest, "sha256:"))
	if err != nil || len(decoded) != 32 || !strings.HasPrefix(digest, "sha256:") {
		t.Fatal("ProxyImage has no valid sha256 identity")
	}
	t.Logf("nginx image: %s %s", ProxyImage, digest)
	for _, tc := range []struct {
		name   string
		domain string
		length int
	}{
		{"native_project_61_bytes", "console.platform-native-" + strings.Repeat("a", 32) + ".test", 61},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.domain) != tc.length || checkDomain(tc.domain) != nil {
				t.Fatal("invalid long-name regression fixture")
			}
			checkNginxName(t, tc.domain, digest)
		})
	}
}

func checkNginxName(t *testing.T, domain, digest string) {
	t.Helper()
	dir := t.TempDir()
	certDir, confDir := filepath.Join(dir, "certs"), filepath.Join(dir, "conf")
	ca, err := LoadOrCreateCA(filepath.Join(dir, "ca"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ca.IssueWithNames(certDir, DefaultCertName, []string{"test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ca.Issue(certDir, domain); err != nil {
		t.Fatalf("issuing a temporary certificate for the %d-byte name: %v", len(domain), err)
	}
	if err := RenderNginx(confDir, []Route{{Domain: domain, Backend: "127.0.0.1:8080", Scheme: "http"}}, "127.0.0.1", DefaultCertName, "fixture-generation"); err != nil {
		t.Fatal(err)
	}

	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	group := "ctl-nginx-" + hex.EncodeToString(nonce[:])
	svc := &Service{Name: "check", ContainerName: group + "-check"}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e := newServiceEngine("")
	if _, found, err := e.lookup(ctx, svc.ContainerName); err != nil || found {
		t.Fatalf("isolated container name is not available: %v", err)
	}
	var created Instance
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		current, found, err := e.lookup(cleanup, svc.ContainerName)
		if err != nil {
			t.Error(err)
			return
		}
		if !found {
			return
		}
		if owns(group, svc, current) != nil || current.ImageDigest != digest || (created.Created != "" && current.Created != created.Created) {
			t.Error("refusing cleanup of a container whose test ownership or identity changed")
			return
		}
		if _, err := exec.CommandContext(cleanup, containerBin(), "rm", "--force", svc.ContainerName).CombinedOutput(); err != nil {
			t.Errorf("removing the owned nginx test container: %v", err)
		}
	})
	args := []string{"create", "--name", svc.ContainerName,
		"--label", LabelRole + "=" + roleService,
		"--label", LabelGroup + "=" + group,
		"--label", LabelService + "=" + svc.Name,
		"--volume", confDir + ":/etc/nginx/conf.d:ro",
		"--volume", certDir + ":/etc/nginx/certs:ro",
		"--entrypoint", "nginx", ProxyImage, "-t"}
	if output, err := exec.CommandContext(ctx, containerBin(), args...).CombinedOutput(); err != nil {
		t.Fatalf("creating the isolated nginx checker: %v\n%s", err, nginxCheckOutput(output))
	}
	var found bool
	created, found, err = e.lookup(ctx, svc.ContainerName)
	if err != nil || !found || owns(group, svc, created) != nil || created.ImageDigest != digest || created.Created == "" || created.State != "stopped" {
		t.Fatal("nginx checker identity differs; its process was not started")
	}
	output, err := exec.CommandContext(ctx, containerBin(), "start", "--attach", svc.ContainerName).CombinedOutput()
	if err != nil {
		t.Fatalf("actual nginx rejected generated configuration for a %d-byte domain: %v\n%s", len(domain), err, nginxCheckOutput(output))
	}
	if !strings.Contains(string(output), "syntax is ok") || !strings.Contains(string(output), "test is successful") {
		t.Fatalf("nginx did not confirm a successful configuration check:\n%s", nginxCheckOutput(output))
	}
}

func nginxCheckOutput(output []byte) string {
	const limit = 8192
	if len(output) > limit {
		output = output[len(output)-limit:]
	}
	return fmt.Sprintf("%s", strings.ToValidUTF8(string(output), "?"))
}
