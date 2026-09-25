package stack

import (
	"reflect"
	"strings"
	"testing"
)

const siteCompose = `name: site
x-bay:
  domain: site.test
services:
  web:
    image: nginx
    x-bay:
      domains: [console.site.test, example.site.test, admin.example.site.test]
  db:
    image: postgres
`

func TestServiceArgumentsLabelEveryDomain(t *testing.T) {
	cfg, err := Load(write(t, siteCompose))
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(serviceArguments("site", cfg.Services["web"], "f"), " ")
	if want := "--label " + LabelDomain + "=console.site.test,example.site.test,admin.example.site.test"; !strings.Contains(args, want) {
		t.Errorf("web arguments %q lack %q", args, want)
	}
	args = strings.Join(serviceArguments("site", cfg.Services["db"], "f"), " ")
	if want := "--label " + LabelDomain + "= "; !strings.Contains(args, want) {
		t.Errorf("db arguments %q lack the empty domain label", args)
	}
}

func siteInstance(domains string) Instance {
	return Instance{
		Name: "site-web", State: "running",
		Labels: map[string]string{
			LabelRole: roleService, LabelGroup: "site", LabelService: "web", LabelPort: "8080", LabelDomain: domains,
		},
	}
}

func TestRoutesCoverEveryDomainOfAService(t *testing.T) {
	routes, conflicts := routesFrom(serviceInstances([]Instance{
		siteInstance("console.site.test,example.site.test,admin.example.site.test"),
	}))
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v", conflicts)
	}
	backend := "site-web." + BackendDomain + ":8080"
	want := []Route{
		{Domain: "admin.example.site.test", Backend: backend, Scheme: "http", container: "site-web"},
		{Domain: "console.site.test", Backend: backend, Scheme: "http", container: "site-web"},
		{Domain: "example.site.test", Backend: backend, Scheme: "http", container: "site-web"},
	}
	if !reflect.DeepEqual(routes, want) {
		t.Fatalf("routes = %+v, want %+v", routes, want)
	}
}

func TestRoutesOfASingleDomainContainer(t *testing.T) {
	routes, _ := routesFrom(serviceInstances([]Instance{siteInstance("web.shop.test")}))
	if len(routes) != 1 || routes[0].Domain != "web.shop.test" {
		t.Fatalf("routes = %+v, want web.shop.test only", routes)
	}
}

func TestRoutesSkipAnInternalContainer(t *testing.T) {
	routes, _ := routesFrom(serviceInstances([]Instance{siteInstance("")}))
	if len(routes) != 0 {
		t.Fatalf("routes = %+v, want none", routes)
	}
}

func TestDomainServedByAnotherProjectNamesBothServices(t *testing.T) {
	cfg, err := Load(write(t, siteCompose))
	if err != nil {
		t.Fatal(err)
	}
	running := serviceInstances([]Instance{{
		Name: "other-front", State: "running",
		Labels: map[string]string{
			LabelRole: roleService, LabelGroup: "other", LabelService: "front", LabelPort: "80",
			LabelDomain: "front.other.test,admin.example.site.test",
		},
	}})
	err = domainsFree(cfg, running)
	if err == nil {
		t.Fatal("a domain another project serves was allowed")
	}
	for _, want := range []string{"admin.example.site.test", `"other"`, `"front"`, `"site"`, `"web"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to name %s", err, want)
		}
	}
	if err := domainsFree(cfg, nil); err != nil {
		t.Fatalf("no running service, err = %v", err)
	}
}
