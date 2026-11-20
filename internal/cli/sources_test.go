package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adwise/developer-skills-manager/internal/manager"
	"github.com/adwise/developer-skills-manager/internal/source"
)

func fixtureFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
}
func fixtureSkill(t *testing.T, repo, id, body string) string {
	t.Helper()
	path := filepath.Join(repo, "skills", id, "SKILL.md")
	fixtureFile(t, path, "---\nname: "+filepath.Base(id)+"\ndescription: Fixture.\n---\n"+body+"\n")
	return path
}
func fixtureRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if output, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", output, err)
	}
	return repo
}
func multiFixture(t *testing.T) (string, string, string, func(bool, ...string) string) {
	t.Helper()
	isolateEnvironment(t)
	t.Setenv("SKILLS_MANAGER_CACHE", filepath.Join(t.TempDir(), "cache"))
	home := os.Getenv("SKILLS_MANAGER_HOME")
	a, b := fixtureRepo(t), fixtureRepo(t)
	fixtureSkill(t, a, "review", "a original")
	fixtureSkill(t, b, "review", "b original")
	run := func(ok bool, args ...string) string {
		t.Helper()
		var out, stderr bytes.Buffer
		code := RunWithIO(args, strings.NewReader(""), &out, &stderr)
		if (code == 0) != ok {
			t.Fatalf("%v returned %d\n%s\n%s", args, code, &out, &stderr)
		}
		return out.String() + stderr.String()
	}
	run(true, "source", "add", "adwise", "--source", a, "--namespace", "skills-manager", "--repo", a)
	run(true, "source", "add", "personal", "--source", b, "--repo", b)
	return home, a, b, run
}

func TestMultiSourceIsolationAndResolution(t *testing.T) {
	home, a, b, run := multiFixture(t)
	if text := run(false, "install", "review"); !strings.Contains(text, "ambiguous") {
		t.Fatal(text)
	}
	fixtureSkill(t, a, "dependency", "dependency a")
	fixtureSkill(t, b, "dependency", "dependency b")
	fixtureFile(t, filepath.Join(a, "skills.json"), `{"skillSets":[{"name":"work","skills":["review"]}],"dependencies":{"review":["dependency"]}}`)
	fixtureFile(t, filepath.Join(b, "skills.json"), `{"skillSets":[{"name":"work","skills":["review"]}]}`)
	run(false, "install", "@work")
	run(true, "install", "adwise/@work", "personal/review")
	for _, ns := range []string{"skills-manager", "personal"} {
		if _, err := os.Stat(filepath.Join(home, ".agents", "skills", ns, "review", "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "personal", "dependency")); !os.IsNotExist(err) {
		t.Fatal("cross-source dependency installed")
	}
	text := run(true, "status")
	if strings.Count(text, "clean") != 3 || !strings.Contains(text, "Source adwise") || !strings.Contains(text, "Source personal") {
		t.Fatal(text)
	}
	run(true, "status", "review", "--from", "personal")
	run(false, "status", "review")
	run(false, "contribute", "--all", "--repo", a)
	run(false, "config", "set", "repo", a)
	run(true, "config", "set", "repo", b, "--from", "personal")
	run(false, "source", "add", "duplicate", "--source", b, "--namespace", "personal")
	run(false, "source", "add", "../bad", "--source", b)
	run(false, "source", "add", "bad", "--source", b, "--namespace", "one/two")
	run(true, "source", "remove", "personal")
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "personal", "review", "SKILL.md")); err != nil {
		t.Fatal("remove deleted installation", err)
	}
	if strings.Contains(run(true, "source", "list"), "personal") {
		t.Fatal("removed config retained")
	}
}

func TestMultiNestedAliasesAndAmbiguousPrefixes(t *testing.T) {
	home, a, _, run := multiFixture(t)
	fixtureFile(t, filepath.Join(a, "skills.json"), `{"externalSkills":[{"id":"personal/review","path":"skills/review"},{"id":"other/review","path":"skills/review"}]}`)
	// personal/review is both a qualified ID and a literal nested ID in adwise.
	run(false, "install", "personal/review")
	run(true, "install", "adwise/personal/review", "adwise/other/review")
	run(true, "install", "review", "--from", "personal")
	text := run(true, "status", "--from", "adwise")
	if !strings.Contains(text, "personal/review:") || !strings.Contains(text, "other/review:") || strings.Count(text, "clean") != 2 {
		t.Fatal(text)
	}
	run(true, "update", "adwise/personal/review")
	run(true, "uninstall", "personal/review", "--from", "adwise")
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "personal", "review", "SKILL.md")); err != nil {
		t.Fatal("removed wrong source", err)
	}
}

func TestLegacyMigrationPreservesFilesAndBaseline(t *testing.T) {
	for _, namespace := range []string{"", "team"} {
		t.Run("namespace-"+namespace, func(t *testing.T) {
			isolateEnvironment(t)
			home, repo := os.Getenv("SKILLS_MANAGER_HOME"), fixtureRepo(t)
			fixtureSkill(t, repo, "review", "original")
			ns := namespace
			if ns == "" {
				ns = "skills-manager"
			}
			mgr := manager.Manager{Repository: source.Repository{Source: repo}, GlobalDir: filepath.Join(home, ".agents", "skills", ns), StateDir: filepath.Join(home, ".agents", ".skills-manager", ns), Out: io.Discard}
			if err := mgr.Install("review"); err != nil {
				t.Fatal(err)
			}
			before := make(map[string]string)
			if err := filepath.WalkDir(filepath.Join(home, ".agents"), func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !entry.IsDir() {
					data, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					before[path] = string(data)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(sourceConfiguration{Source: repo, Namespace: namespace, Repo: repo})
			fixtureFile(t, configurationPath(home), string(data))
			saved, err := readConfiguration(home)
			if err != nil || saved.Sources["default"].Namespace != ns {
				t.Fatalf("%+v %v", saved, err)
			}
			raw, _ := os.ReadFile(configurationPath(home))
			var parsed map[string]json.RawMessage
			if err := json.Unmarshal(raw, &parsed); err != nil {
				t.Fatal(err)
			}
			if len(parsed) != 1 || parsed["sources"] == nil {
				t.Fatalf("not migrated: %s", raw)
			}
			for path, text := range before {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != text {
					t.Fatalf("migration changed %s", path)
				}
			}
			var out, stderr bytes.Buffer
			if code := RunWithIO([]string{"status"}, strings.NewReader(""), &out, &stderr); code != 0 || !strings.Contains(out.String(), "clean") {
				t.Fatalf("%s %s", &out, &stderr)
			}
			if _, err := readConfiguration(home); err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(configurationPath(home))
			if !bytes.Equal(raw, after) {
				t.Fatal("migration is not idempotent")
			}
		})
	}
}

func TestLegacyMigrationKeepsUnknownBaselineProtected(t *testing.T) {
	isolateEnvironment(t)
	home, repo := os.Getenv("SKILLS_MANAGER_HOME"), fixtureRepo(t)
	fixtureSkill(t, repo, "review", "upstream")
	path := filepath.Join(home, ".agents", "skills", "skills-manager", "review", "SKILL.md")
	local := "---\nname: review\ndescription: Fixture.\n---\nlegacy local edit\n"
	fixtureFile(t, path, local)
	data, _ := json.Marshal(map[string]string{"source": repo})
	fixtureFile(t, configurationPath(home), string(data))
	var out, stderr bytes.Buffer
	if RunWithIO([]string{"status"}, strings.NewReader(""), &out, &stderr) != 0 || !strings.Contains(out.String(), "unknown") {
		t.Fatalf("%s %s", &out, &stderr)
	}
	if RunWithIO([]string{"update"}, strings.NewReader(""), &out, &stderr) == 0 {
		t.Fatal("unknown edits overwritten")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != local {
		t.Fatal("legacy installation changed")
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", ".skills-manager")); !os.IsNotExist(err) {
		t.Fatal("invented baseline")
	}
}

func TestMultiBackwardCompatibleOverridesAndInit(t *testing.T) {
	home, a, b, run := multiFixture(t)
	run(false, "init", "--source", a)
	run(true, "init", "--source", a, "--from", "adwise")
	run(false, "init", "--source", a, "--from", "adwise", "--namespace", "elsewhere")
	run(false, "available", "--source", a)
	run(true, "available", "--source", b, "--from", "adwise")
	saved, err := readConfiguration(home)
	if err != nil || saved.Sources["adwise"].Source != a || saved.Sources["adwise"].Repo != a {
		t.Fatal("override persisted", saved, err)
	}
	t.Setenv("SKILLS_MANAGER_SOURCE", b)
	run(false, "available")
	run(true, "available", "--from", "personal")
	t.Setenv("SKILLS_MANAGER_SOURCE", "")
	run(true, "source", "remove", "personal")
	run(true, "init", "--source", a)
	saved, err = readConfiguration(home)
	if err != nil || len(saved.Sources) != 1 || saved.Sources["adwise"].Namespace != "skills-manager" {
		t.Fatal(saved, err)
	}
}

func TestMultiContributePreflightRoutingAndProtection(t *testing.T) {
	home, a, b, run := multiFixture(t)
	run(true, "install", "adwise/review", "personal/review")
	installedA := filepath.Join(home, ".agents", "skills", "skills-manager", "review", "SKILL.md")
	installedB := filepath.Join(home, ".agents", "skills", "personal", "review", "SKILL.md")
	localA := "---\nname: review\ndescription: Fixture.\n---\na edited\n"
	localB := strings.ReplaceAll(localA, "a edited", "b edited")
	fixtureFile(t, installedA, localA)
	fixtureFile(t, installedB, localB)
	run(false, "update")
	run(false, "uninstall", "adwise/review")
	originalB, _ := os.ReadFile(filepath.Join(b, "skills", "review", "SKILL.md"))
	fixtureSkill(t, b, "review", "conflict")
	var preview, diag bytes.Buffer
	if code := RunWithIO([]string{"contribute", "--all", "--yes"}, strings.NewReader(""), &preview, &diag); code != 3 || !strings.Contains(preview.String(), `conflict "SKILL.md"`) {
		t.Fatalf("exit %d: %s %s", code, &preview, &diag)
	}
	applied, _ := os.ReadFile(filepath.Join(a, "skills", "review", "SKILL.md"))
	if string(applied) != localA {
		t.Fatal("second source conflict blocked first source")
	}
	fixtureFile(t, filepath.Join(b, "skills", "review", "SKILL.md"), string(originalB))
	if text := run(true, "contribute", "--all"); !strings.Contains(text, "Cancelled") {
		t.Fatal(text)
	}
	run(true, "contribute", "--all", "--yes")
	for path, want := range map[string]string{filepath.Join(a, "skills", "review", "SKILL.md"): localA, filepath.Join(b, "skills", "review", "SKILL.md"): localB} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("wrong routing: %s %s %v", path, data, err)
		}
	}
	if text := run(true, "status"); strings.Count(text, "both") != 2 {
		t.Fatal("contribute advanced baseline", text)
	}
	run(true, "update")
	if text := run(true, "status"); strings.Count(text, "clean") != 2 {
		t.Fatal(text)
	}
}

func TestMultiUpdateUnavailableAndProtectedSourcePreflight(t *testing.T) {
	home, a, b, run := multiFixture(t)
	run(true, "install", "adwise/review", "personal/review")
	installed := filepath.Join(home, ".agents", "skills", "skills-manager", "review", "SKILL.md")
	before, _ := os.ReadFile(installed)
	fixtureSkill(t, a, "review", "upstream a")
	fixtureFile(t, filepath.Join(home, ".agents", "skills", "personal", "review", "SKILL.md"), "---\nname: review\ndescription: Fixture.\n---\nlocal b\n")
	run(false, "update")
	after, _ := os.ReadFile(installed)
	if !bytes.Equal(before, after) {
		t.Fatal("updated before all protections checked")
	}
	if err := os.Rename(filepath.Join(b, "skills"), filepath.Join(b, "unavailable")); err != nil {
		t.Fatal(err)
	}
	run(false, "update")
	run(false, "status")
	run(false, "tui")
	after, _ = os.ReadFile(installed)
	if !bytes.Equal(before, after) {
		t.Fatal("updated despite unavailable source")
	}
	run(true, "update", "--from", "adwise")
}

type confirmationReader func([]byte) (int, error)

func (fn confirmationReader) Read(p []byte) (int, error) { return fn(p) }

func TestMultiContributionRechecksEverySourceAfterConfirmation(t *testing.T) {
	home, a, _, run := multiFixture(t)
	run(true, "install", "adwise/review", "personal/review")
	for _, ns := range []string{"skills-manager", "personal"} {
		fixtureFile(t, filepath.Join(home, ".agents", "skills", ns, "review", "data.bin"), "\x00initial")
	}
	before, _ := os.ReadFile(filepath.Join(a, "skills", "review", "SKILL.md"))
	var out, stderr bytes.Buffer
	reader := confirmationReader(func(p []byte) (int, error) {
		// Binary diff output alone cannot detect this change: plans retain checksums.
		fixtureFile(t, filepath.Join(home, ".agents", "skills", "personal", "review", "data.bin"), "\x00changed")
		return copy(p, "y\n"), io.EOF
	})
	if RunWithIO([]string{"contribute", "--all"}, reader, &out, &stderr) == 0 || !strings.Contains(stderr.String(), "changed during confirmation") {
		t.Fatalf("%s %s", &out, &stderr)
	}
	after, _ := os.ReadFile(filepath.Join(a, "skills", "review", "SKILL.md"))
	if !bytes.Equal(before, after) {
		t.Fatal("first source changed")
	}
	if _, err := os.Stat(filepath.Join(a, "skills", "review", "data.bin")); !os.IsNotExist(err) {
		t.Fatal("first source written before rechecks")
	}
}

func TestSourceConfigurationRejectsUnsafeAndUnknownFields(t *testing.T) {
	for _, text := range []string{
		`{"sources":{"../bad":{"source":"x","namespace":"good"}}}`,
		`{"sources":{"good":{"source":"x","namespace":"../bad"}}}`,
		`{"sources":{"one":{"source":"x","namespace":"same"},"two":{"source":"y","namespace":"same"}}}`,
		`{"sources":{"one":{"source":"x","namespace":"good","installPath":"/tmp/elsewhere"}}}`,
		`{"sources":{},"source":"legacy"}`,
		`{"source":"x","namespace":"../bad"}`,
	} {
		home := t.TempDir()
		fixtureFile(t, configurationPath(home), text)
		if _, err := readConfiguration(home); err == nil {
			t.Fatalf("accepted %s", text)
		}
		data, _ := os.ReadFile(configurationPath(home))
		if string(data) != text {
			t.Fatal("invalid configuration was rewritten")
		}
	}
}

func TestNamespaceSymlinkAndReassignedProvenance(t *testing.T) {
	home, a, b, run := multiFixture(t)
	run(true, "install", "adwise/review")
	installed := filepath.Join(home, ".agents", "skills", "skills-manager", "review", "SKILL.md")
	fixtureFile(t, installed, "---\nname: review\ndescription: Fixture.\n---\nlocal\n")
	run(true, "source", "remove", "adwise")
	run(true, "source", "add", "other", "--source", b, "--namespace", "skills-manager", "--repo", a)
	if text := run(false, "contribute", "other/review", "--yes"); !strings.Contains(text, "another source") {
		t.Fatal(text)
	}
	run(false, "update", "--from", "other")
	// Never follow a namespace symlink to write installations elsewhere.
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(home, ".agents", "skills", "personal")); err != nil {
		t.Fatal(err)
	}
	run(false, "install", "review", "--from", "personal")
	if _, err := os.Stat(filepath.Join(target, "review")); !os.IsNotExist(err) {
		t.Fatal("escaped namespace")
	}
}

func TestSourceCachesAreIsolatedAndLegacyUntouched(t *testing.T) {
	base := filepath.Join(t.TempDir(), "repository")
	fixtureFile(t, filepath.Join(base, "sentinel"), "legacy")
	a, b := sourceCache(base, "adwise", "https://example.test/a"), sourceCache(base, "personal", "https://example.test/a")
	if a == b || a == base || b == base || sourceCache(base, "adwise", "https://example.test/b") == a {
		t.Fatal("cache collision")
	}
	data, err := os.ReadFile(filepath.Join(base, "sentinel"))
	if err != nil || string(data) != "legacy" {
		t.Fatal("changed legacy cache")
	}
}
