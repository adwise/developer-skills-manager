package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Identity preserves local source identity independently of the invoking cwd.
func Identity(value string) (string, error) {
	if info, err := os.Stat(value); err == nil && info.IsDir() {
		absolute, err := filepath.Abs(value)
		if err != nil {
			return "", err
		}
		return filepath.EvalSymlinks(absolute)
	}
	return strings.TrimSpace(value), nil
}

func VerifyCheckout(identity, checkout string) error {
	root, err := gitOutput("-C", checkout, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("target must be a Git checkout: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(checkout)
	if err != nil {
		return err
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return err
	}
	actual, err := filepath.EvalSymlinks(strings.TrimSpace(root))
	if err != nil {
		return err
	}
	if canonical != actual {
		return fmt.Errorf("repo must point to the checkout root")
	}
	if canonical == identity {
		return nil
	}
	origin, err := gitOutput("-C", canonical, "remote", "get-url", "origin")
	if err != nil {
		return fmt.Errorf("cannot verify checkout origin: %w", err)
	}
	if sameRepository(origin, identity) {
		return nil
	}
	expected := identity
	if info, err := os.Stat(identity); err == nil && info.IsDir() {
		expected, err = gitOutput("-C", identity, "remote", "get-url", "origin")
		if err != nil {
			return fmt.Errorf("cannot verify checkout against local source without an origin")
		}
	}
	if !sameRepository(origin, expected) {
		return fmt.Errorf("checkout does not belong to installation source %q", identity)
	}
	return nil
}
