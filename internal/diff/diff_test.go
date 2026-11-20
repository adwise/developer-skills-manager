package diff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectoriesEmptyFilesAndExecutableChanges(t *testing.T) {
	before, after := t.TempDir(), t.TempDir()
	for _, item := range []struct {
		root, name string
		mode       os.FileMode
	}{
		{before, "deleted", 0o644}, {after, "added", 0o644},
		{before, "script", 0o644}, {after, "script", 0o755},
	} {
		if err := os.WriteFile(filepath.Join(item.root, item.name), nil, item.mode); err != nil {
			t.Fatal(err)
		}
	}
	text, err := Directories(before, after, "example")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"File added", "File deleted", "Executable: false → true"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
}
