package stack

import (
	"os"
	"path/filepath"
	"testing"
)

// A project's name comes from its file when the file is readable, and from the
// registry entry recorded for the file's path when it is not, so a project
// whose file is gone can still be stopped from the command line.
func TestAProjectWhoseFileIsGoneIsNamedFromTheRegistry(t *testing.T) {
	m := &Machine{Dir: t.TempDir()}
	dir := t.TempDir()
	file := filepath.Join(dir, "compose.yaml")
	if err := m.Register(GroupRef{Name: "crudui", StackPath: file}); err != nil {
		t.Fatal(err)
	}
	for _, given := range []string{file, dir} {
		name, err := ProjectNameAt(m, given)
		if err != nil {
			t.Fatalf("%s: %v", given, err)
		}
		if name != "crudui" {
			t.Errorf("%s: name %q, want crudui", given, name)
		}
	}
}

func TestAReadableFileNamesItsProject(t *testing.T) {
	m := &Machine{Dir: t.TempDir()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"),
		[]byte("name: shop\nservices:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	name, err := ProjectNameAt(m, dir)
	if err != nil || name != "shop" {
		t.Errorf("name %q, %v; want shop", name, err)
	}
}

// Neither a file nor a registry entry names nothing, and says why.
func TestAPathNothingIsRecordedForIsReported(t *testing.T) {
	m := &Machine{Dir: t.TempDir()}
	if _, err := ProjectNameAt(m, t.TempDir()); err == nil {
		t.Error("a path with no file and no registry entry was given a name")
	}
}
