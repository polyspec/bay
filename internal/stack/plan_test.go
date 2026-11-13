package stack

import "testing"

// The names up gives services are known from the file before up, and match
// what up reports.
func TestThePlanNamesServicesAsUpWill(t *testing.T) {
	cfg, err := LoadNamed(&Machine{Dir: t.TempDir()}, write(t, "name: shop\nservices:\n  db:\n    image: postgres\n    expose: [\"5432\"]\n"), "run7")
	if err != nil {
		t.Fatal(err)
	}
	m := &Machine{Dir: t.TempDir()}
	p := PlanProject(m, cfg)
	if len(p.Services) != 1 {
		t.Fatalf("services = %+v", p.Services)
	}
	s := p.Services[0]
	if s.Container != "run7-db" || s.Host != "run7-db."+BackendDomain || s.Port != 5432 {
		t.Errorf("planned %+v", s)
	}
	if s.Certificate == nil || s.Certificate.Names[0] != s.Host {
		t.Errorf("certificate %+v does not name the host", s.Certificate)
	}
}
