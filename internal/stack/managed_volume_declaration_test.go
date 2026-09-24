package stack

import "testing"

func TestComposeManagedVolumeSizeDeclaration(t *testing.T) {
	cfg, err := Load(write(t, `name: volume-project
services:
  db:
    image: postgres
    volumes: [pgdata:/var/lib/postgresql]
volumes:
  pgdata:
    name: volume-project-data
    driver_opts: {size: 32g}
`))
	if err != nil {
		t.Fatalf("managed volume declaration rejected: %v", err)
	}
	if got := cfg.Services["db"].Volumes; len(got) != 1 || got[0] != "volume-project-data:/var/lib/postgresql" {
		t.Fatalf("resolved managed volume = %v", got)
	}
}
