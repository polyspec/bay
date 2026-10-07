package stack

import "testing"

// TestRoutedNamesAProjectWithADomain covers the condition under which up starts
// the proxy before the services: a project with a service that the proxy routes
// a domain to, and not a project of internal services alone.
func TestRoutedNamesAProjectWithADomain(t *testing.T) {
	web := &Service{Name: "web", Domains: []string{"web.first.test"}}
	work := &Service{Name: "work"}
	if !routed(&Config{Name: "first", Services: map[string]*Service{"web": web, "work": work}}) {
		t.Error("a project with a routed service is not routed")
	}
	if routed(&Config{Name: "inner", Services: map[string]*Service{"work": work}}) {
		t.Error("a project of internal services is routed")
	}
}
