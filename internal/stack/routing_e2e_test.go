package stack

import "testing"

// TestOneServiceServesSeveralDomains routes three domains to one container:
// each domain gets its own route and certificate, and the backend receives the
// requested host.
func TestOneServiceServesSeveralDomains(t *testing.T) {
	requireE2E(t)

	m := NewMachine(t.TempDir())
	web := &Service{
		Name: "web", ContainerName: "e2emulti-web", Image: e2eImage, Command: e2eServer("multi"),
		Domains: []string{"console.multi.test", "example.multi.test", "admin.example.multi.test"},
		Port:    80, Network: ProxyNetwork,
	}
	t.Cleanup(func() { Remove(web.ContainerName); stopEveryProxy() })

	mustRegister(t, m, GroupRef{Name: "e2emulti", Domains: []string{"multi.test"}})
	if err := StartService("e2emulti", web); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, web.ContainerName)
	res, err := SyncProxy(m)
	if err != nil {
		t.Fatal(err)
	}
	if got := ownRoutes(res.Routes, "multi.test"); len(got) != 3 {
		t.Fatalf("routes = %+v, want one per domain", got)
	}
	proxy := waitRunning(t, ProxyName)
	client := caClient(t, m, proxy.IPv4)
	for _, d := range web.Domains {
		waitForBody(t, client, "https://"+d+"/", "multi "+d)
	}
}
