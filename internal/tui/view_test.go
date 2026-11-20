package tui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
)

func previewModel() InstallerModel {
	names := []string{"agent-browser", "code-review", "composer-auth", "frontend-figma", "frontend-tailwind", "git-branching", "git-commits", "github-cli", "github-pull-requests", "jira-management", "performance-triage", "web-fetch", "web-search", "workspace-management"}
	var skills []skill.Metadata
	for _, name := range names {
		skills = append(skills, skill.Metadata{ID: name, Description: "Project tools and guidance for " + name + "."})
	}
	skills[4].Description = "Build responsive interfaces using existing components, design tokens, spacing scales, and accessible interaction patterns."
	m := NewInstallerModelWithStatus(skills, []source.SkillSet{{Name: "frontend", Description: "Design and implementation essentials.", Skills: []string{"frontend-figma", "frontend-tailwind"}}}, map[string]bool{"code-review": true, "frontend-tailwind": true, "github-cli": true}, map[string]bool{"code-review": true, "frontend-tailwind": true}, map[string]string{"code-review": "upstream", "frontend-tailwind": "both", "github-cli": "clean"})
	m.selected["agent-browser"] = true
	m.selected["code-review"] = true
	m.removals["github-cli"] = true
	m.cursor = 5
	return m
}

func assertFits(t *testing.T, view string, width, height int) {
	t.Helper()
	if view == "" {
		return
	}
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		t.Fatalf("height %d > %d:\n%s", len(lines), height, view)
	}
	for _, line := range lines {
		if got := ansi.StringWidth(line); got > width {
			t.Fatalf("width %d > %d: %q", got, width, line)
		}
	}
}

// UPDATE_TUI_SNAPSHOTS=1 refreshes these examples from the real View, never a mock.
func TestInstallerSnapshots(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 32}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			base := previewModel()
			var skills []skill.Metadata
			for _, item := range base.rows {
				if item.kind == skillRow {
					skills = append(skills, skill.Metadata{ID: item.id, Description: item.description})
				}
			}
			m := NewSourceInstallerModel([]SourceGroup{
				{Name: "team", Namespace: "skills-manager", Skills: skills, Sets: []source.SkillSet{{Name: "frontend", Description: "Design and implementation essentials.", Skills: []string{"frontend-figma", "frontend-tailwind"}}}, Installed: base.installed, Updates: base.updates, States: base.states},
				{Name: "personal", Namespace: "personal", Skills: []skill.Metadata{{ID: "notes", Description: "Personal note-taking tools."}}},
				{Name: "empty", Namespace: "empty"},
			})
			for id, yes := range base.selected {
				m.selected["team/"+id] = yes
			}
			for id, yes := range base.removals {
				m.removals["team/"+id] = yes
			}
			m.discards["team/frontend-tailwind"] = true
			m.cursor = base.cursor
			m.width, m.height = size[0], size[1]
			view := m.View()
			if !strings.Contains(view, "r discard/update") {
				t.Fatal("source tabs must not hide the discard shortcut")
			}
			assertFits(t, view, m.width, m.height)
			if got := len(strings.Split(view, "\n")); got != m.height {
				t.Fatalf("snapshot uses %d rows, want %d", got, m.height)
			}
			path := filepath.Join("..", "..", "docs", "tui", fmt.Sprintf("%dx%d.txt", m.width, m.height))
			if os.Getenv("UPDATE_TUI_SNAPSHOTS") == "1" {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(view+"\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if view+"\n" != string(want) {
				t.Fatalf("snapshot differs: %s\n%s", path, view)
			}
			m.color = true
			colored := m.View()
			assertFits(t, colored, m.width, m.height)
			if ansi.Strip(colored) != view {
				t.Fatal("color changed layout")
			}
		})
	}
}

func TestInstallerResponsiveBoundsAndFocus(t *testing.T) {
	for _, width := range []int{0, 1, 2, 10, 23, 24, 25, 40, 59, 60, 80, 109, 110, 120, 180} {
		for _, height := range []int{0, 1, 3, 7, 8, 12, 15, 16, 24, 32, 50} {
			m := previewModel()
			m.width, m.height = width, height
			m.rows[5].description = "Unicode 界面 👩🏽‍💻 é and averylongwordwithoutspacesandmoreletters.\n\x1b[31mNo injected color\x1b[0m"
			for _, cursor := range []int{0, 5, len(m.rows) - 1} {
				m.cursor = cursor
				for _, color := range []bool{false, true} {
					m.color = color
					view := m.View()
					assertFits(t, view, width, height)
					if width >= 40 && height >= 8 && !strings.Contains(ansi.Strip(view), "> "+m.marker(m.rows[cursor])+" "+rowLabel(m.rows[cursor])) {
						t.Fatalf("lost focus at %dx%d:\n%s", width, height, view)
					}
					if !color && strings.Contains(view, "\x1b") {
						t.Fatal("ANSI in plain view")
					}
				}
			}
			m.rows = nil
			assertFits(t, m.View(), width, height)
			m.playing, m.game = true, newBlackjack()
			assertFits(t, m.View(), width, height)
		}
	}
}

func TestInstallerNavigationAndSummary(t *testing.T) {
	m := previewModel()
	for _, step := range []struct {
		key    string
		cursor int
	}{{"end", len(m.rows) - 1}, {"down", len(m.rows) - 1}, {"home", 0}, {"up", 0}, {"j", 1}, {"k", 0}} {
		// Rune messages also expose named keys via String for this model.
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(step.key)})
		m = updated.(InstallerModel)
		if m.cursor != step.cursor {
			t.Fatalf("%s: cursor %d", step.key, m.cursor)
		}
	}
	if !strings.Contains(m.View(), "Queue  1 install · 1 update · 1 remove") {
		t.Fatal(m.View())
	}
	m.selected, m.removals = map[string]bool{}, map[string]bool{}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(InstallerModel)
	if cmd != nil || m.complete || !strings.Contains(m.View(), "Nothing queued") {
		t.Fatal(m.View())
	}
	assertFits(t, m.View(), m.width, m.height)
}

func TestStatusColorsAndScrolledHeaders(t *testing.T) {
	m := previewModel()
	m.color = true
	for _, tc := range []struct{ status, style string }{
		{"local", "1;33"}, {"upstream", "1;36"}, {"both", "1;31"},
		{"unknown-different", "1;33"}, {"clean", "32"}, {"", "2"},
	} {
		item := row{kind: skillRow, id: "test"}
		m.states["test"] = tc.status
		if got := m.statusStyle(item); got != tc.style {
			t.Errorf("%q: style %q, want %q", tc.status, got, tc.style)
		}
		m.cursor = 1
		m.rows[1] = item
		if !strings.Contains(m.View(), "\x1b["+tc.style+"m"+m.compactStatus(item)+"\x1b[0m") {
			t.Errorf("missing colored focused status %q", tc.status)
		}
	}
	m.removals["test"] = true
	if m.statusStyle(row{kind: skillRow, id: "test"}) != "1;31" {
		t.Fatal("removal not red")
	}
	m = previewModel()
	m.width, m.height = 80, 12
	for _, cursor := range []int{0, 1, 5, len(m.rows) - 1} {
		m.cursor = cursor
		view := m.View()
		assertFits(t, view, m.width, m.height)
		group := "SKILLS"
		if m.rows[cursor].kind == setRow {
			group = "SETS"
		}
		if !strings.Contains(view, "── "+group+" ") || !strings.Contains(view, "> "+m.marker(m.rows[cursor])+" "+rowLabel(m.rows[cursor])) {
			t.Fatalf("missing group or focus at %d:\n%s", cursor, view)
		}
	}
}

func TestInstallerPlainModesAndEmptyState(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "") // Presence, even empty, disables color.
	if terminalColor() {
		t.Fatal("NO_COLOR ignored")
	}
	modes := previewModel()
	modes.color = terminalColor()
	if strings.Contains(modes.View(), "\x1b") {
		t.Fatal("NO_COLOR view contains ANSI")
	}
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatal(err)
	}
	if !terminalColor() {
		t.Fatal("terminal color disabled")
	}
	t.Setenv("TERM", "dumb")
	if terminalColor() {
		t.Fatal("dumb terminal colored")
	}
	modes.color = terminalColor()
	if strings.Contains(modes.View(), "\x1b") {
		t.Fatal("dumb terminal view contains ANSI")
	}
	var output bytes.Buffer
	_, err := RunInstallerWithStatus(strings.NewReader(""), &output, nil, nil, nil, nil, nil)
	if err == nil || output.Len() != 0 {
		t.Fatal("non-TTY output must not receive terminal escapes")
	}
	m := NewInstallerModel(nil, nil, nil)
	if !strings.Contains(m.View(), "No skills are available.") || strings.Contains(m.View(), "\x1b") {
		t.Fatal(m.View())
	}
}
