package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/adwise/developer-skills-manager/internal/source"
)

type configuration struct {
	Source    string `json:"source"`
	Namespace string `json:"namespace"`
}

func configurationPath(home string) string {
	return filepath.Join(home, ".config", "skills-manager", "config.json")
}

func readConfiguration(home string) (configuration, error) {
	var saved configuration
	data, err := os.ReadFile(configurationPath(home))
	if os.IsNotExist(err) {
		return saved, nil
	}
	if err != nil {
		return saved, fmt.Errorf("read configuration: %w", err)
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		return saved, fmt.Errorf("parse configuration: %w", err)
	}
	return saved, nil
}

func initialize(opts options, out io.Writer) error {
	opts.source = strings.TrimSpace(opts.source)
	if opts.source == "" {
		return fmt.Errorf("init requires --source <url-or-path>")
	}
	// Persist absolute paths so local sources work from any working directory.
	if info, err := os.Stat(opts.source); err == nil && info.IsDir() {
		absolute, err := filepath.Abs(opts.source)
		if err != nil {
			return err
		}
		opts.source = absolute
	}
	if _, err := (source.Repository{Source: opts.source, CacheDir: opts.cache}).Open(false); err != nil {
		return err
	}
	data, err := json.MarshalIndent(configuration{Source: opts.source, Namespace: opts.namespace}, "", "  ")
	if err != nil {
		return err
	}
	path := configurationPath(opts.home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("save configuration: %w", err)
	}
	fmt.Fprintf(out, "✓ Saved source %s in %s\nSkills install into %s\n", opts.source, path, filepath.Join(opts.home, ".agents", "skills", opts.namespace))
	return nil
}
