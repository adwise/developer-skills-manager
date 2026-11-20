package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
)

type sourceConfiguration struct {
	Source    string `json:"source"`
	Namespace string `json:"namespace"`
	Repo      string `json:"repo,omitempty"`
}

type configuration struct {
	Sources map[string]sourceConfiguration `json:"sources,omitempty"`
	// Legacy fields are accepted on input only (also mirrored for single-source callers).
	Source    string `json:"source,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Repo      string `json:"repo,omitempty"`
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
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&saved); err != nil {
		return saved, fmt.Errorf("parse configuration: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return saved, fmt.Errorf("parse configuration: expected one JSON object")
	}
	if saved.Sources != nil && (saved.Source != "" || saved.Namespace != "" || saved.Repo != "") {
		return saved, fmt.Errorf("configuration cannot mix sources and legacy fields")
	}
	if saved.Source != "" {
		ns := saved.Namespace
		if ns == "" {
			ns = "skills-manager"
		}
		saved.Sources = map[string]sourceConfiguration{"default": {Source: saved.Source, Namespace: ns, Repo: saved.Repo}}
		if err := saveConfiguration(home, saved); err != nil {
			return saved, err
		}
	}
	if err := validateConfiguration(saved); err != nil {
		return saved, err
	}
	if len(saved.Sources) == 1 {
		for _, entry := range saved.Sources {
			saved.Source, saved.Namespace, saved.Repo = entry.Source, entry.Namespace, entry.Repo
		}
	}
	return saved, nil
}

func validateConfiguration(saved configuration) error {
	namespaces := make(map[string]string)
	for name, entry := range saved.Sources {
		if !skill.IsValidName(name) {
			return fmt.Errorf("invalid source name %q", name)
		}
		if !skill.IsValidName(entry.Namespace) {
			return fmt.Errorf("invalid namespace %q", entry.Namespace)
		}
		if strings.TrimSpace(entry.Source) == "" || strings.HasPrefix(entry.Source, "-") {
			return fmt.Errorf("invalid source for %s", name)
		}
		if other, exists := namespaces[entry.Namespace]; exists {
			return fmt.Errorf("sources %s and %s share namespace %s", other, name, entry.Namespace)
		}
		namespaces[entry.Namespace] = name
	}
	return nil
}

func sourceNames(saved configuration) []string {
	names := make([]string, 0, len(saved.Sources))
	for name := range saved.Sources {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func absoluteDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repo must be a directory")
	}
	return absolute, nil
}

func normalizeSource(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "-") {
		return "", fmt.Errorf("requires --source <url-or-path>")
	}
	if info, err := os.Stat(value); err == nil && info.IsDir() {
		return filepath.Abs(value)
	}
	return value, nil
}

func initialize(opts options, out io.Writer) error {
	saved, err := readConfiguration(opts.home)
	if err != nil {
		return err
	}
	name := opts.from
	if name == "" {
		if len(saved.Sources) > 1 {
			return fmt.Errorf("init requires --from NAME with multiple sources")
		}
		name = "default"
		for existing := range saved.Sources {
			name = existing
		}
	}
	return putSource(opts, saved, name, false, out)
}

func putSource(opts options, saved configuration, name string, add bool, out io.Writer) error {
	if !skill.IsValidName(name) {
		return fmt.Errorf("invalid source name %q", name)
	}
	previous, exists := saved.Sources[name]
	if add && exists {
		return fmt.Errorf("source %q already exists", name)
	}
	value, err := normalizeSource(opts.source)
	if err != nil {
		return err
	}
	ns := opts.namespace
	if add && !opts.namespaceOverride {
		ns = name
	}
	if exists && !opts.namespaceOverride {
		ns = previous.Namespace
	}
	// Changing an existing namespace would strand its installations and baselines.
	if exists && ns != previous.Namespace {
		return fmt.Errorf("cannot change namespace of an existing source; remove its configuration first")
	}
	repo := previous.Repo
	if !exists && !add {
		repo = saved.Repo
	}
	if opts.repoOverride != "" {
		repo, err = absoluteDirectory(opts.repoOverride)
		if err != nil {
			return err
		}
	}
	if saved.Sources == nil {
		saved.Sources = make(map[string]sourceConfiguration)
	}
	saved.Sources[name] = sourceConfiguration{Source: value, Namespace: ns, Repo: repo}
	if err := validateConfiguration(saved); err != nil {
		return err
	}
	if _, err := (source.Repository{Source: value, CacheDir: sourceCache(opts.cache, name, value)}).Open(false); err != nil {
		return err
	}
	if err := saveConfiguration(opts.home, saved); err != nil {
		return err
	}
	fmt.Fprintf(out, "✓ Saved source %s (%s) in %s\nSkills install into %s\n", name, value, configurationPath(opts.home), filepath.Join(opts.home, ".agents", "skills", ns))
	return nil
}

func setRepo(home, repo string, out io.Writer) error { return setSourceRepo(home, repo, "", out) }

func setSourceRepo(home, repo, from string, out io.Writer) error {
	absolute, err := absoluteDirectory(repo)
	if err != nil {
		return err
	}
	saved, err := readConfiguration(home)
	if err != nil {
		return err
	}
	if from == "" && len(saved.Sources) > 1 {
		return fmt.Errorf("config set repo requires --from NAME with multiple sources")
	}
	if from == "" {
		for name := range saved.Sources {
			from = name
		}
	}
	if from != "" {
		entry, exists := saved.Sources[from]
		if !exists {
			return fmt.Errorf("unknown source %q", from)
		}
		entry.Repo = absolute
		saved.Sources[from] = entry
	} else {
		saved.Repo = absolute
	}
	if err := saveConfiguration(home, saved); err != nil {
		return err
	}
	fmt.Fprintf(out, "✓ Saved repo %s\n", absolute)
	return nil
}

func runSource(opts options, args []string, out io.Writer) error {
	saved, err := readConfiguration(opts.home)
	if err != nil {
		return err
	}
	if len(args) == 1 && args[0] == "list" {
		for _, name := range sourceNames(saved) {
			entry := saved.Sources[name]
			fmt.Fprintf(out, "%s\n  source: %s\n  namespace: %s\n  repo: %s\n", name, entry.Source, entry.Namespace, entry.Repo)
		}
		return nil
	}
	if len(args) == 2 {
		switch args[0] {
		case "add":
			if !opts.sourceOverride {
				return fmt.Errorf("source add requires --source URL/path")
			}
			return putSource(opts, saved, args[1], true, out)
		case "remove":
			if _, exists := saved.Sources[args[1]]; !exists {
				return fmt.Errorf("unknown source %q", args[1])
			}
			delete(saved.Sources, args[1])
			if err := saveConfiguration(opts.home, saved); err != nil {
				return err
			}
			fmt.Fprintf(out, "Removed source %s from configuration only; installations, baselines and caches retained.\n", args[1])
			return nil
		}
	}
	return fmt.Errorf("usage: skills-manager source add NAME --source URL/path [--namespace NS] [--repo PATH] | source list | source remove NAME")
}

func saveConfiguration(home string, saved configuration) error {
	if err := validateConfiguration(saved); err != nil {
		return err
	}
	if saved.Sources != nil {
		saved.Source, saved.Namespace, saved.Repo = "", "", ""
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	path := configurationPath(home)
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
	return nil
}
