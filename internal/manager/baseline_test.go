package manager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBaselineRecordsInstalledContentWithRelativeCatalogRoot(t *testing.T) {
	m, repo, _ := workflowFixture(t)
	id := "group-example"
	if err := m.Install(id); err != nil {
		t.Fatal(err)
	}
	items, _, err := m.Repository.Catalog(false)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(items[0].Path, "later.txt"), "source changed after copy")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.saveBaseline(items[0], relative); err != nil {
		t.Fatal(err)
	}
	_, base, err := m.readBaseline(id)
	if err != nil {
		t.Fatal(err)
	}
	same, err := samePackage(base, filepath.Join(m.GlobalDir, id))
	if err != nil || !same {
		t.Fatalf("baseline does not match installed content: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "later.txt")); !os.IsNotExist(err) {
		t.Fatal("baseline copied changing upstream instead of installed package")
	}
}
