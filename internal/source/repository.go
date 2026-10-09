package source

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/adwise/developer-skills-manager/internal/skill"
)

const manifestFile = "skills.json"

type Repository struct {
	Source   string
	CacheDir string
}

type SkillSet struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Skills      []string `json:"skills"`
}

type CatalogData struct {
	Skills       []skill.Metadata
	Sets         []SkillSet
	Dependencies map[string][]string
}

type externalSkill struct {
	ID         string `json:"id"`
	Repository string `json:"repository,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Path       string `json:"path"`
}

var (
	commitRef           = regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)
	externalPathSegment = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

type manifest struct {
	ExternalSkills []externalSkill     `json:"externalSkills"`
	SkillSets      []SkillSet          `json:"skillSets"`
	Dependencies   map[string][]string `json:"dependencies"`
}

func (r Repository) Open(refresh bool) (string, error) {
	if r.Source == "" {
		return "", fmt.Errorf("no source repository configured; run `skills-manager init --source <url-or-path>`")
	}
	if info, err := os.Stat(r.Source); err == nil && info.IsDir() {
		root, err := filepath.Abs(r.Source)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(root, "skills")); err != nil {
			return "", fmt.Errorf("source %q does not contain a skills directory", r.Source)
		}
		return root, nil
	}

	if r.CacheDir == "" {
		return "", fmt.Errorf("no cache directory configured for remote source %q", r.Source)
	}
	gitDir := filepath.Join(r.CacheDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(r.CacheDir), 0o755); err != nil {
			return "", fmt.Errorf("create repository cache: %w", err)
		}
		if err := runGit("clone", "--depth=1", r.Source, r.CacheDir); err != nil {
			return "", fmt.Errorf("clone skills repository: %w", err)
		}
	} else {
		origin, err := gitOutput("-C", r.CacheDir, "remote", "get-url", "origin")
		if err != nil {
			return "", fmt.Errorf("inspect repository cache: %w", err)
		}
		if !sameRepository(origin, r.Source) {
			return "", fmt.Errorf("cache %q belongs to %q, not %q; choose another --cache directory", r.CacheDir, strings.TrimSpace(origin), r.Source)
		}
		if refresh {
			if err := runGit("-C", r.CacheDir, "fetch", "--depth=1", "origin"); err != nil {
				return "", fmt.Errorf("refresh skills repository: %w", err)
			}
			if err := runGit("-C", r.CacheDir, "reset", "--hard", "FETCH_HEAD"); err != nil {
				return "", fmt.Errorf("refresh skills repository: %w", err)
			}
		}
	}

	if _, err := os.Stat(filepath.Join(r.CacheDir, "skills")); err != nil {
		return "", fmt.Errorf("source %q does not contain a skills directory", r.Source)
	}
	return r.CacheDir, nil
}

func sameRepository(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == right {
		return true
	}

	leftHost, leftPath, leftOK := remoteRepository(left)
	rightHost, rightPath, rightOK := remoteRepository(right)
	return leftOK && rightOK && strings.EqualFold(leftHost, rightHost) && leftPath == rightPath
}

func remoteRepository(raw string) (string, string, bool) {
	var host, repositoryPath string
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", "", false
		}
		host = parsed.Hostname()
		repositoryPath = parsed.Path
	} else {
		// Git also accepts the SCP-like form git@example.com:owner/repository.git.
		separator := strings.IndexByte(raw, ':')
		if separator < 0 || strings.Contains(raw[:separator], "/") {
			return "", "", false
		}
		host = raw[:separator]
		if at := strings.LastIndexByte(host, '@'); at >= 0 {
			host = host[at+1:]
		}
		repositoryPath = raw[separator+1:]
	}

	host = strings.TrimSpace(host)
	repositoryPath = strings.Trim(strings.TrimSpace(repositoryPath), "/")
	repositoryPath = strings.TrimSuffix(repositoryPath, ".git")
	if host == "" || repositoryPath == "" {
		return "", "", false
	}
	return host, repositoryPath, true
}

func (r Repository) Catalog(refresh bool) ([]skill.Metadata, string, error) {
	catalog, root, err := r.Registry(refresh)
	return catalog.Skills, root, err
}

func (r Repository) Registry(refresh bool) (CatalogData, string, error) {
	root, err := r.Open(refresh)
	if err != nil {
		return CatalogData{}, "", err
	}

	skills, err := skill.Discover(filepath.Join(root, "skills"))
	if err != nil {
		return CatalogData{}, root, err
	}
	configuration, err := readManifest(root)
	if err != nil {
		return CatalogData{}, root, err
	}

	seen := make(map[string]string, len(skills)+len(configuration.ExternalSkills))
	for _, item := range skills {
		seen[item.ID] = item.Path
	}
	for _, external := range configuration.ExternalSkills {
		if !skill.IsValidID(external.ID) {
			return CatalogData{}, root, fmt.Errorf("%s has invalid external skill ID %q", manifestFile, external.ID)
		}
		dir, err := r.externalDirectory(root, external)
		if err != nil {
			return CatalogData{}, root, fmt.Errorf("%s external skill %q: %w", manifestFile, external.ID, err)
		}
		metadata, err := skill.Validate(dir)
		if err != nil {
			return CatalogData{}, root, fmt.Errorf("validate external skill %q: %w", external.ID, err)
		}
		parts := strings.Split(external.ID, "/")
		if metadata.Name != parts[len(parts)-1] {
			return CatalogData{}, root, fmt.Errorf("external skill ID %q must end in its SKILL.md name %q", external.ID, metadata.Name)
		}
		if other, exists := seen[external.ID]; exists {
			return CatalogData{}, root, fmt.Errorf("duplicate skill ID %q in %s and %s", external.ID, other, metadata.Path)
		}
		metadata.ID = external.ID
		seen[metadata.ID] = metadata.Path
		skills = append(skills, metadata)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].ID < skills[j].ID })
	for _, item := range skills {
		parts := strings.Split(item.ID, "/")
		for index := 1; index < len(parts); index++ {
			parent := strings.Join(parts[:index], "/")
			if _, exists := seen[parent]; exists {
				return CatalogData{}, root, fmt.Errorf("skill IDs %q and %q conflict because one is a namespace of the other", parent, item.ID)
			}
		}
	}

	sets := configuration.SkillSets
	seenSets := make(map[string]struct{}, len(sets))
	for index := range sets {
		set := &sets[index]
		set.Name = strings.TrimSpace(set.Name)
		set.Description = strings.TrimSpace(set.Description)
		if !skill.IsValidName(set.Name) {
			return CatalogData{}, root, fmt.Errorf("%s has invalid skill set name %q", manifestFile, set.Name)
		}
		if _, exists := seenSets[set.Name]; exists {
			return CatalogData{}, root, fmt.Errorf("%s has duplicate skill set %q", manifestFile, set.Name)
		}
		seenSets[set.Name] = struct{}{}
		if len(set.Skills) == 0 {
			return CatalogData{}, root, fmt.Errorf("skill set %q is empty", set.Name)
		}
		setSkills := make(map[string]struct{}, len(set.Skills))
		for _, id := range set.Skills {
			if _, exists := seen[id]; !exists {
				return CatalogData{}, root, fmt.Errorf("skill set %q references unavailable skill %q", set.Name, id)
			}
			if _, duplicate := setSkills[id]; duplicate {
				return CatalogData{}, root, fmt.Errorf("skill set %q contains duplicate skill %q", set.Name, id)
			}
			setSkills[id] = struct{}{}
		}
	}
	sort.Slice(sets, func(i, j int) bool { return sets[i].Name < sets[j].Name })

	catalog := CatalogData{Skills: skills, Sets: sets, Dependencies: configuration.Dependencies}
	for id, dependencies := range catalog.Dependencies {
		if _, exists := seen[id]; !exists {
			return CatalogData{}, root, fmt.Errorf("dependencies reference unavailable skill %q", id)
		}
		unique := make(map[string]bool)
		for _, dependency := range dependencies {
			if unique[dependency] {
				return CatalogData{}, root, fmt.Errorf("skill %q has duplicate dependency %q", id, dependency)
			}
			unique[dependency] = true
		}
	}
	ids := make([]string, 0, len(skills))
	for _, item := range skills {
		ids = append(ids, item.ID)
	}
	if _, err := catalog.ResolveDependencies(ids); err != nil {
		return CatalogData{}, root, err
	}
	return catalog, root, nil
}

func readManifest(root string) (manifest, error) {
	path := filepath.Join(root, manifestFile)
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return manifest{}, nil
	}
	if err != nil {
		return manifest{}, fmt.Errorf("read %s: %w", manifestFile, err)
	}
	defer file.Close()

	var configuration manifest
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&configuration); err != nil {
		return manifest{}, fmt.Errorf("parse %s: %w", manifestFile, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return manifest{}, fmt.Errorf("parse %s: multiple JSON values are not allowed", manifestFile)
		}
		return manifest{}, fmt.Errorf("parse %s: %w", manifestFile, err)
	}
	return configuration, nil
}

func (r Repository) externalDirectory(root string, external externalSkill) (string, error) {
	cleanPath, err := cleanExternalPath(external.Path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(external.Repository) == "" {
		if strings.TrimSpace(external.Ref) != "" {
			return "", fmt.Errorf("ref requires a repository URL")
		}
		return resolveInside(root, cleanPath)
	}
	if !commitRef.MatchString(external.Ref) {
		return "", fmt.Errorf("ref must be a full 40- or 64-character commit hash")
	}
	if r.CacheDir == "" {
		return "", fmt.Errorf("no cache directory configured for external repository %q", external.Repository)
	}

	key := sha256.Sum256([]byte(strings.TrimSpace(external.Repository) + "\x00" + strings.ToLower(external.Ref) + "\x00" + cleanPath))
	cache := filepath.Join(r.CacheDir+"-external", fmt.Sprintf("%x", key))
	if _, err := os.Stat(filepath.Join(cache, ".git")); os.IsNotExist(err) {
		if _, cacheErr := os.Stat(cache); cacheErr == nil {
			return "", fmt.Errorf("external cache %q exists but is not a Git repository", cache)
		} else if !os.IsNotExist(cacheErr) {
			return "", fmt.Errorf("inspect external cache: %w", cacheErr)
		}
		if err := initializeExternalCache(cache, external, cleanPath); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", fmt.Errorf("inspect external cache: %w", err)
	} else if err := verifyExternalCache(cache, external, cleanPath); err != nil {
		return "", err
	}
	return resolveInside(cache, cleanPath)
}

func cleanExternalPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, `\\`) || pathpkg.IsAbs(value) {
		return "", fmt.Errorf("path must be a slash-separated relative directory")
	}
	clean := pathpkg.Clean(value)
	if clean != value || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path %q is not a clean relative directory", value)
	}
	for _, part := range strings.Split(clean, "/") {
		if !externalPathSegment.MatchString(part) {
			return "", fmt.Errorf("path %q contains unsupported characters", value)
		}
	}
	return clean, nil
}

func initializeExternalCache(cache string, external externalSkill, skillPath string) (returnErr error) {
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		return fmt.Errorf("create external cache directory: %w", err)
	}
	defer func() {
		if returnErr != nil {
			_ = os.RemoveAll(cache)
		}
	}()
	if err := runGit("init", "--quiet", cache); err != nil {
		return fmt.Errorf("initialize external skill cache: %w", err)
	}
	if err := runGit("-C", cache, "remote", "add", "origin", strings.TrimSpace(external.Repository)); err != nil {
		return fmt.Errorf("configure external skill repository: %w", err)
	}
	if err := configureSparseCheckout(cache, skillPath); err != nil {
		return err
	}
	if err := runGit("-C", cache, "fetch", "--depth=1", "--filter=blob:none", "origin", external.Ref); err != nil {
		return fmt.Errorf("fetch external skill repository: %w", err)
	}
	if err := runGit("-C", cache, "checkout", "--quiet", "--detach", "FETCH_HEAD"); err != nil {
		return fmt.Errorf("checkout external skill repository: %w", err)
	}
	return nil
}

func verifyExternalCache(cache string, external externalSkill, skillPath string) error {
	origin, err := gitOutput("-C", cache, "remote", "get-url", "origin")
	if err != nil {
		return fmt.Errorf("inspect external skill cache: %w", err)
	}
	if !sameRepository(origin, external.Repository) {
		return fmt.Errorf("external cache %q belongs to %q, not %q", cache, strings.TrimSpace(origin), external.Repository)
	}
	head, err := gitOutput("-C", cache, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("inspect external skill revision: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(head), external.Ref) {
		return fmt.Errorf("external cache %q is at %s, expected %s", cache, strings.TrimSpace(head), external.Ref)
	}
	if err := configureSparseCheckout(cache, skillPath); err != nil {
		return err
	}
	if err := runGit("-C", cache, "checkout", "--quiet", "--force", "--detach", external.Ref); err != nil {
		return fmt.Errorf("restore external skill checkout: %w", err)
	}
	return nil
}

func configureSparseCheckout(cache, skillPath string) error {
	if err := runGit("-C", cache, "sparse-checkout", "init", "--no-cone"); err != nil {
		return fmt.Errorf("initialize sparse checkout for external skill: %w", err)
	}
	pattern := "/" + skillPath + "/"
	if err := runGit("-C", cache, "sparse-checkout", "set", "--no-cone", pattern); err != nil {
		return fmt.Errorf("configure sparse checkout for external skill: %w", err)
	}
	return nil
}

func resolveInside(root, relative string) (string, error) {
	if strings.TrimSpace(relative) == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("path must be relative to the repository")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved := filepath.Clean(filepath.Join(rootAbs, filepath.FromSlash(relative)))
	if !isChild(rootAbs, resolved) {
		return "", fmt.Errorf("path %q escapes the repository", relative)
	}

	// The lexical check above handles missing paths. If the path exists, also
	// ensure a symlinked parent did not redirect it outside the repository.
	realRoot, rootErr := filepath.EvalSymlinks(rootAbs)
	if rootErr != nil {
		return "", rootErr
	}
	realResolved, resolvedErr := filepath.EvalSymlinks(resolved)
	if resolvedErr == nil && !isChild(realRoot, realResolved) {
		return "", fmt.Errorf("path %q resolves outside the repository", relative)
	}
	if resolvedErr != nil && !os.IsNotExist(resolvedErr) {
		return "", resolvedErr
	}
	return resolved, nil
}

func isChild(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func runGit(args ...string) error {
	command := exec.Command("git", args...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
	}
	return nil
}

func gitOutput(args ...string) (string, error) {
	command := exec.Command("git", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return string(output), nil
}
