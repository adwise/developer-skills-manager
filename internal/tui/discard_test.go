package tui

import (
	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
)

func TestDiscardQueueAcrossTabsAndConfirmation(t *testing.T) {
	m := NewSourceInstallerModel([]SourceGroup{
		{Name: "one", Skills: []skill.Metadata{{ID: "example"}}, Sets: []source.SkillSet{{Name: "work", Skills: []string{"example"}}}, Installed: map[string]bool{"example": true}},
		{Name: "two", Skills: []skill.Metadata{{ID: "example"}}, Installed: map[string]bool{"example": true}},
	})
	key := func(k string) {
		t.Helper()
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		if k == "enter" {
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		}
		if k == "esc" {
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		}
		next, _ := m.Update(msg)
		m = next.(InstallerModel)
	}
	m.switchTab(1) // source one, set row
	key("r")
	m.switchTab(1)
	key("r")
	if got := m.SelectedDiscards(); len(got) != 2 || got[0] != "one/example" || got[1] != "two/example" {
		t.Fatal(got)
	}
	if !strings.Contains(m.View(), "DISCARD") {
		t.Fatal(m.View())
	}
	key("enter")
	if m.complete || !m.review {
		t.Fatal("no confirmation screen")
	}
	for _, text := range []string{"DISCARD + UPDATE", "one/example", "two/example", "Dependency diffs", "separate confirmation", "without backup"} {
		if !strings.Contains(m.View(), text) {
			t.Fatal(m.View())
		}
	}
	key("esc")
	if m.review || m.cancelled {
		t.Fatal("back cancelled queue")
	}
	key("enter")
	key("c")
	if !m.complete {
		t.Fatal("did not proceed to full preview")
	}
}
