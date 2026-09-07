package contract_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/polyspec/bay/internal/contract"
	"github.com/polyspec/bay/internal/stack"
)

// The example is printed by "bay schema" as the file to copy. It has
// to load, and the keys it shows have to take effect.
func TestExampleLoads(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shop")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(path, []byte(contract.Example()), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := stack.Load(path)
	if err != nil {
		t.Fatalf("the example does not load: %v", err)
	}
	if cfg.Name != "shop" || cfg.Domain != "shop.test" {
		t.Fatalf("name %q, domain %q", cfg.Name, cfg.Domain)
	}

	web := cfg.Services["web"]
	if web == nil {
		t.Fatal("no service named web")
	}
	if web.Port != 3000 {
		t.Errorf("web port = %d, want 3000 from ports", web.Port)
	}
	if web.Domain != "web.shop.test" {
		t.Errorf("web domain = %q", web.Domain)
	}
	if web.ContainerName != "shop-web" {
		t.Errorf("web container = %q", web.ContainerName)
	}

	api := cfg.Services["api"]
	if api == nil || api.Domain != "api.shop.test" || api.Port != 8080 {
		t.Errorf("api = %+v", api)
	}

	db := cfg.Services["db"]
	if db == nil || !db.Internal || db.Domain != "" || db.Port != 5432 {
		t.Errorf("db = %+v", db)
	}
	// The example tells the reader to reach the database by this address.
	if got := "shop-db." + stack.BackendDomain + ":5432"; db != nil &&
		db.ContainerName+"."+stack.BackendDomain+":5432" != got {
		t.Errorf("database address = %q, want %q",
			db.ContainerName+"."+stack.BackendDomain+":5432", got)
	}
}
