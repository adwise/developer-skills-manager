package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func tabKey(m InstallerModel, key tea.KeyType) (InstallerModel, tea.Cmd) {
	updated, cmd := m.Update(tea.KeyMsg{Type: key})
	return updated.(InstallerModel), cmd
}

func tabModel() InstallerModel {
	return NewSourceInstallerModel([]SourceGroup{
		{Name: "work", Namespace: "managed", Skills: []skill.Metadata{{ID: "nested/review"}, {ID: "clean"}}, Sets: []source.SkillSet{{Name: "dev", Skills: []string{"nested/review", "clean"}}}, Installed: map[string]bool{"clean": true}},
		{Name: "personal", Namespace: "personal", Skills: []skill.Metadata{{ID: "nested/review"}}, Sets: []source.SkillSet{{Name: "dev", Skills: []string{"nested/review"}}}, Installed: map[string]bool{"nested/review": true}, Updates: map[string]bool{"nested/review": true}, States: map[string]string{"nested/review": "both"}},
		{Name: "empty", Namespace: "empty"},
	})
}

func TestTabsNavigationSelectionsAndApply(t *testing.T) {
	m := tabModel()
	m, _ = tabKey(m, tea.KeyShiftTab)
	if m.activeTab != 3 || len(m.rows) != 0 {
		t.Fatal("backward wrap")
	}
	m, _ = tabKey(m, tea.KeyTab)
	if m.activeTab != 0 {
		t.Fatal("forward wrap")
	}
	m, _ = tabKey(m, tea.KeyEnd)
	allCursor := m.cursor
	m, _ = tabKey(m, tea.KeyTab)
	m.height = 32
	view := m.View()
	if !strings.Contains(view, "\n\n── SKILLS") {
		t.Fatal("missing space after sets", view)
	}
	m, _ = tabKey(m, tea.KeySpace) // filtered set
	if !reflect.DeepEqual(m.Selected(), []string{"work/nested/review"}) {
		t.Fatal(m.Selected())
	}
	m, _ = tabKey(m, tea.KeyEnd)
	m.toggleRemoval() // installed clean item
	m, _ = tabKey(m, tea.KeyTab)
	m, _ = tabKey(m, tea.KeySpace) // same set/skill IDs, different source
	if !reflect.DeepEqual(m.SelectedUpdates(), []string{"personal/nested/review"}) {
		t.Fatal(m.SelectedUpdates())
	}
	if !reflect.DeepEqual(m.SelectedRemovals(), []string{"work/clean"}) {
		t.Fatal(m.SelectedRemovals())
	}
	m, _ = tabKey(m, tea.KeySpace) // deselect only this source's set
	if len(m.SelectedUpdates()) != 0 || len(m.Selected()) != 1 || len(m.SelectedRemovals()) != 1 {
		t.Fatal("set deselection crossed source boundary")
	}
	m, _ = tabKey(m, tea.KeySpace)
	if strings.Contains(m.View(), "personal/@dev") || !strings.Contains(m.View(), "@dev") {
		t.Fatal(m.View())
	}
	m, _ = tabKey(m, tea.KeyShiftTab)
	if m.cursor != 2 {
		t.Fatal("filtered cursor lost")
	}
	m, _ = tabKey(m, tea.KeyShiftTab)
	if m.cursor != allCursor {
		t.Fatal("overview cursor lost")
	}
	before := m.View()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = updated.(InstallerModel)
	m, _ = tabKey(m, tea.KeyTab)
	if m.activeTab != 0 || !m.playing {
		t.Fatal("blackjack changed tabs")
	}
	m, _ = tabKey(m, tea.KeyEsc)
	if m.View() != before {
		t.Fatal("blackjack changed catalog state")
	}
	m, _ = tabKey(m, tea.KeyShiftTab)
	if !strings.Contains(m.View(), "Queue (all)  1 install · 1 update · 1 remove") {
		t.Fatal(m.View())
	}
	m, cmd := tabKey(m, tea.KeyEnter) // empty tab reviews everything
	if cmd != nil || m.complete || !m.review || len(m.Selected()) != 1 || len(m.SelectedUpdates()) != 1 || len(m.SelectedRemovals()) != 1 {
		t.Fatal("global review lost selections")
	}
}

func TestTabsZeroOneAndEmpty(t *testing.T) {
	for _, groups := range [][]SourceGroup{nil, {{Name: "empty"}}} {
		m := NewSourceInstallerModel(groups)
		for i := 0; i < 5; i++ {
			m, _ = tabKey(m, tea.KeyTab)
			m, _ = tabKey(m, tea.KeyEnd)
			m, _ = tabKey(m, tea.KeySpace)
			m.toggleRemoval()
			if !strings.Contains(m.View(), "No skills are available.") {
				t.Fatal(m.View())
			}
			var cmd tea.Cmd
			m, cmd = tabKey(m, tea.KeyEnter)
			if cmd != nil || m.complete || !m.review {
				t.Fatal("empty review")
			}
			m, _ = tabKey(m, tea.KeyEsc)
		}
	}
	m := NewSourceInstallerModel([]SourceGroup{{Name: "only", Skills: []skill.Metadata{{ID: "one"}}}})
	m, _ = tabKey(m, tea.KeyTab)
	if m.activeTab != 1 || m.label(m.rows[0]) != "one" {
		t.Fatal("single source tab")
	}
	m, _ = tabKey(m, tea.KeyTab)
	if m.activeTab != 0 {
		t.Fatal("single source wrap")
	}
}

func TestTabstripAppearance(t *testing.T) {
	m := tabModel()
	m.width, m.height = 80, 24
	m.color = false
	strip := m.tabstrip()
	if !strings.Contains(strip, "╭[All sources]╮") || !strings.Contains(strip, "╭ work ╮") {
		t.Fatalf("plain tabs: %q", strip)
	}
	divider := m.tabDivider(strip)
	if !strings.HasPrefix(divider, "╯             ╰") || ansi.StringWidth(divider) != m.width {
		t.Fatalf("divider: %q", divider)
	}
	m.color = true
	colored := m.tabstrip()
	if !strings.Contains(colored, "\x1b[1;36m╭[All sources]╮\x1b[0m") || !strings.Contains(colored, "\x1b[2m╭ work ╮\x1b[0m") || ansi.Strip(colored) != strip {
		t.Fatalf("colored tabs: %q", colored)
	}
	m, _ = tabKey(m, tea.KeyTab)
	if !strings.Contains(ansi.Strip(m.tabstrip()), "╭[work]╮") {
		t.Fatal("active tab did not change")
	}
}

func TestManyTabsResponsive(t *testing.T) {
	var groups []SourceGroup
	for i := 0; i < 30; i++ {
		groups = append(groups, SourceGroup{Name: fmt.Sprintf("界面-%02d", i), Skills: []skill.Metadata{{ID: "nested/👩🏽‍💻"}}})
	}
	for _, width := range []int{0, 1, 2, 10, 23, 24, 40, 80, 120} {
		for _, height := range []int{0, 1, 3, 7, 8, 9, 16, 24, 32} {
			m := NewSourceInstallerModel(groups)
			m.width, m.height = width, height
			for i := 0; i < len(m.tabs); i++ {
				for _, color := range []bool{false, true} {
					m.color = color
					view := m.View()
					assertFits(t, view, width, height)
					if width >= 24 && height > 0 && !strings.Contains(ansi.Strip(view), "["+m.tabs[m.activeTab].name+"]") {
						t.Fatalf("active tab hidden: %s", view)
					}
				}
				m, _ = tabKey(m, tea.KeyTab)
			}
		}
	}
}
