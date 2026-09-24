package main

import (
	"reflect"
	"testing"

	"github.com/polyspec/bay/internal/stack"
)

var siteLinks = []string{
	"https://console.site.test/", "https://example.site.test/", "https://admin.example.site.test/",
}

func multiDomainGroup() stack.GroupStatus {
	return stack.GroupStatus{
		Name: "site", Domain: "site.test",
		Services: []stack.ServiceStatus{{
			Name: "web", State: "running", Routed: true,
			Domains: []string{"console.site.test", "example.site.test", "admin.example.site.test"},
			URLs:    siteLinks,
		}, {
			Name: "db", State: "running", Internal: true, Address: "site-db.container.test:5432",
		}},
	}
}

func links(s section, name string) []string {
	var out []string
	for _, r := range s.Rows {
		if r.Text == name && r.Link != "" {
			out = append(out, r.Link)
		}
	}
	return out
}

func TestProjectViewLinksEveryDomainOfAService(t *testing.T) {
	p := &panel{}
	projectView(p, multiDomainGroup(), false)
	services := p.Sections[0]
	if got := links(services, "web"); !reflect.DeepEqual(got, siteLinks) {
		t.Fatalf("web links = %v, want %v", got, siteLinks)
	}
	if got := links(services, "db"); len(got) != 0 {
		t.Fatalf("internal service links = %v, want none", got)
	}
	controls := 0
	for _, r := range services.Rows {
		if r.Text == "web" && len(r.Buttons) > 0 {
			controls++
		}
	}
	if controls != 1 {
		t.Fatalf("web has %d rows with controls, want 1", controls)
	}
}

func TestDashboardAddressesListEveryDomainOfAService(t *testing.T) {
	snap := stack.Snapshot{Groups: []stack.GroupStatus{multiDomainGroup()}}
	snap.Machine.Proxies = []stack.ProxyStatus{{Name: "bay-edge", State: "running", Generation: "g", Routes: 3}}
	p := &panel{}
	dashboardView(p, snap, false)
	for _, s := range p.Sections {
		if got := links(s, "web"); len(got) > 0 {
			if !reflect.DeepEqual(got, siteLinks) {
				t.Fatalf("addresses = %v, want %v", got, siteLinks)
			}
			return
		}
	}
	t.Fatal("the dashboard links no domain of web")
}

func TestServiceViewStatesEveryDomainAndCertificate(t *testing.T) {
	g := multiDomainGroup()
	snap := stack.Snapshot{Groups: []stack.GroupStatus{g}}
	p := &panel{}
	serviceView(p, snap, g, g.Services[0], false, nil)
	var addresses, certificates []string
	for _, s := range p.Sections {
		for _, r := range s.Rows {
			if r.Kind == "kv" && r.Link != "" {
				addresses = append(addresses, r.Link)
			}
			if r.Kind == "kv" && r.Text == "Certificate" {
				certificates = append(certificates, r.Detail)
			}
		}
	}
	if !reflect.DeepEqual(addresses, siteLinks) {
		t.Fatalf("addresses = %v, want %v", addresses, siteLinks)
	}
	if !reflect.DeepEqual(certificates, g.Services[0].Domains) {
		t.Fatalf("certificates = %v, want %v", certificates, g.Services[0].Domains)
	}
}
