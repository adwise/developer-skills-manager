package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyContributionCheckoutIdentity(t *testing.T) {
	local, checkout := t.TempDir(), t.TempDir()
	git(t, local, "init", "-q")
	git(t, checkout, "init", "-q")
	if err := VerifyCheckout(local, local); err != nil {
		t.Fatal(err)
	}
	git(t, checkout, "remote", "add", "origin", local)
	if err := VerifyCheckout(local, checkout); err != nil {
		t.Fatal(err)
	}
	git(t, local, "remote", "add", "origin", "git@example.org:team/skills.git")
	git(t, checkout, "remote", "set-url", "origin", "https://example.org/team/skills.git")
	if err := VerifyCheckout(local, checkout); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCheckout("git@example.org:team/skills.git", checkout); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCheckout("git@example.org:other/skills.git", checkout); err == nil {
		t.Fatal("unrelated origin accepted")
	}
	child := filepath.Join(checkout, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCheckout("git@example.org:team/skills.git", child); err == nil {
		t.Fatal("non-root checkout accepted")
	}
}
