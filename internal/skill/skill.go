package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	validName     = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)
	markdownLink  = regexp.MustCompile(`\]\(([^)]+)\)`)
	frontmatterKV = regexp.MustCompile(`^([A-Za-z0-9_-]+):\s*(.*)$`)
)

type Metadata struct {
	// ID is the slash-separated path used to address a skill in a catalog. For
	// example, a skill in skills/noah/test has ID "noah/test", while Name is
	// the name from its SKILL.md ("test").
	ID          string
	Name        string
	Description string
	Path        string
}

func IsValidName(name string) bool {
	return validName.MatchString(name)
}

func IsValidID(id string) bool {
	if id == "" || strings.Contains(id, `\\`) {
		return false
	}
	for _, part := range strings.Split(id, "/") {
		if !IsValidName(part) {
			return false
		}
	}
	return true
}

func Validate(dir string) (Metadata, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Metadata{}, err
	}

	info, err := os.Stat(abs)
	if err != nil {
		return Metadata{}, fmt.Errorf("read skill %q: %w", dir, err)
	}
	if !info.IsDir() {
		return Metadata{}, fmt.Errorf("skill path %q is not a directory", dir)
	}

	name := filepath.Base(abs)
	if !IsValidName(name) {
		return Metadata{}, fmt.Errorf("invalid skill directory name %q; use lowercase letters, digits, and hyphens", name)
	}

	skillFile := filepath.Join(abs, "SKILL.md")
	metadata, err := parseMetadata(skillFile)
	if err != nil {
		return Metadata{}, err
	}
	if metadata.Name != name {
		return Metadata{}, fmt.Errorf("SKILL.md name %q must match directory %q", metadata.Name, name)
	}
	metadata.ID = metadata.Name
	metadata.Path = abs

	err = filepath.WalkDir(abs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("path escapes skill package: %q", path)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not allowed in skill packages: %s", filepath.ToSlash(rel))
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported file type in skill package: %s", filepath.ToSlash(rel))
		}
		if entry.Type().IsRegular() && strings.EqualFold(filepath.Ext(path), ".md") {
			if err := validateMarkdownLinks(abs, path); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Metadata{}, err
	}

	return metadata, nil
}

func Discover(root string) ([]Metadata, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if _, err := os.ReadDir(rootAbs); err != nil {
		return nil, fmt.Errorf("read skills directory %q: %w", root, err)
	}

	var skills []Metadata
	seen := make(map[string]string)
	var visit func(string) error
	visit = func(dir string) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}

		// A directory containing SKILL.md is a package boundary. Do not treat
		// directories inside the package as additional catalog entries.
		for _, entry := range entries {
			if entry.Name() != "SKILL.md" || entry.IsDir() {
				continue
			}
			metadata, err := Validate(dir)
			if err != nil {
				rel, _ := filepath.Rel(rootAbs, dir)
				return fmt.Errorf("validate %s: %w", filepath.ToSlash(rel), err)
			}
			rel, err := filepath.Rel(rootAbs, dir)
			if err != nil {
				return err
			}
			metadata.ID = filepath.ToSlash(rel)
			if !IsValidID(metadata.ID) {
				return fmt.Errorf("invalid skill path %q; every folder must use lowercase letters, digits, and hyphens", metadata.ID)
			}
			if other, exists := seen[metadata.ID]; exists {
				return fmt.Errorf("duplicate skill ID %q in %s and %s", metadata.ID, other, metadata.Path)
			}
			seen[metadata.ID] = metadata.Path
			skills = append(skills, metadata)
			return nil
		}

		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") || !entry.IsDir() {
				continue
			}
			if err := visit(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(rootAbs); err != nil {
		return nil, err
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].ID < skills[j].ID })
	return skills, nil
}

func Checksum(dir string) (string, error) {
	if _, err := Validate(dir); err != nil {
		return "", err
	}

	var paths []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)

	hash := sha256.New()
	for _, path := range paths {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if _, err := io.WriteString(hash, filepath.ToSlash(rel)+"\x00"); err != nil {
			return "", err
		}
		if info.Mode()&0o111 != 0 {
			if _, err := io.WriteString(hash, "executable\x00"); err != nil {
				return "", err
			}
		} else if _, err := io.WriteString(hash, "regular\x00"); err != nil {
			return "", err
		}
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if _, err := io.WriteString(hash, "\x00"); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func parseMetadata(path string) (Metadata, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Metadata{}, fmt.Errorf("SKILL.md is required")
		}
		return Metadata{}, fmt.Errorf("read SKILL.md: %w", err)
	}
	lines := strings.Split(strings.ReplaceAll(string(contents), "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return Metadata{}, fmt.Errorf("SKILL.md must start with YAML frontmatter")
	}

	closingLine := -1
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			closingLine = index
			break
		}
	}
	if closingLine == -1 {
		return Metadata{}, fmt.Errorf("SKILL.md frontmatter is not closed with ---")
	}

	values := make(map[string]string)
	for index := 1; index < closingLine; index++ {
		line := lines[index]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		match := frontmatterKV.FindStringSubmatch(line)
		if match == nil {
			return Metadata{}, fmt.Errorf("invalid SKILL.md frontmatter line %q", line)
		}
		key := match[1]
		value := strings.TrimSpace(match[2])
		if value == ">" || value == "|" {
			var block []string
			for index+1 < closingLine {
				next := lines[index+1]
				if strings.TrimSpace(next) != "" && !strings.HasPrefix(next, " ") && !strings.HasPrefix(next, "\t") {
					break
				}
				index++
				block = append(block, strings.TrimSpace(next))
			}
			separator := "\n"
			if value == ">" {
				separator = " "
			}
			values[key] = strings.TrimSpace(strings.Join(block, separator))
			continue
		}
		values[key] = yamlScalar(value)
	}
	if !IsValidName(values["name"]) {
		return Metadata{}, fmt.Errorf("SKILL.md has an invalid or missing name")
	}
	if strings.TrimSpace(values["description"]) == "" {
		return Metadata{}, fmt.Errorf("SKILL.md has a missing description")
	}

	return Metadata{Name: values["name"], Description: strings.TrimSpace(values["description"])}, nil
}

func yamlScalar(value string) string {
	if len(value) >= 2 {
		if value[0] == '\'' && value[len(value)-1] == '\'' {
			return strings.ReplaceAll(value[1:len(value)-1], "''", "'")
		}
		if value[0] == '"' && value[len(value)-1] == '"' {
			return strings.ReplaceAll(strings.ReplaceAll(value[1:len(value)-1], `\"`, `"`), `\\`, `\`)
		}
	}
	if comment := strings.Index(value, " #"); comment >= 0 {
		value = value[:comment]
	}
	return strings.TrimSpace(value)
}

func validateMarkdownLinks(root, document string) error {
	contents, err := os.ReadFile(document)
	if err != nil {
		return err
	}
	for _, match := range markdownLink.FindAllStringSubmatch(string(contents), -1) {
		target := strings.TrimSpace(strings.SplitN(match[1], " ", 2)[0])
		target = strings.Trim(target, "<>")
		if target == "" || strings.HasPrefix(target, "#") || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "data:") {
			continue
		}
		target = strings.SplitN(target, "#", 2)[0]
		if target == "" {
			continue
		}
		if filepath.IsAbs(target) {
			return fmt.Errorf("absolute reference is not allowed in %s: %s", filepath.Base(document), target)
		}
		resolved := filepath.Clean(filepath.Join(filepath.Dir(document), filepath.FromSlash(target)))
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("reference escapes skill package in %s: %s", filepath.Base(document), target)
		}
		if _, err := os.Stat(resolved); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("referenced file does not exist in %s: %s", filepath.Base(document), target)
			}
			return err
		}
	}
	return nil
}
