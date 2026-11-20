package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
	tea "github.com/charmbracelet/bubbletea"
)

func press(m InstallerModel, key string) InstallerModel {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return next.(InstallerModel)
}

func TestRemovalKeyQueueAndFeedback(t *testing.T) {
	for _, state := range []string{"clean", "local", "both", "unknown"} {
		t.Run(state, func(t *testing.T) {
			m := NewInstallerModelWithStatus([]skill.Metadata{{ID: "example"}}, nil, map[string]bool{"example": true}, map[string]bool{"example": true}, map[string]string{"example": state})
			for _, prior := range []string{"", " ", "r"} {
				if prior != "" {
					m = press(m, prior)
				}
				m = press(m, "d")
				if got := m.SelectedRemovals(); len(got) != 1 || got[0] != "example" {
					t.Fatal(got)
				}
				if len(m.SelectedUpdates())+len(m.SelectedDiscards()) != 0 {
					t.Fatal("conflicting queue")
				}
				for _, text := range []string{"[-] example", "1 remove", "Removal queued", "Local edits remain protected"} {
					if !strings.Contains(m.View(), text) {
						t.Fatal(m.View())
					}
				}
				m = press(m, "d")
				if len(m.SelectedRemovals()) != 0 || !strings.Contains(m.View(), "Removal unqueued") {
					t.Fatal(m.View())
				}
			}
		})
	}
	for _, set := range []bool{false, true} {
		m := NewInstallerModel([]skill.Metadata{{ID: "example"}}, nil, nil)
		if set {
			m = NewInstallerModel(nil, []source.SkillSet{{Name: "work", Skills: []string{"example"}}}, map[string]bool{"example": true})
		}
		m = press(m, "d")
		want := "Not installed"
		if set {
			want = "Sets cannot be removed"
		}
		if len(m.SelectedRemovals()) != 0 || !strings.Contains(m.View(), want) {
			t.Fatal(m.View())
		}
	}
	m := NewInstallerModel([]skill.Metadata{{ID: "example"}}, nil, nil)
	m = press(m, "r")
	if len(m.SelectedDiscards()) != 0 || !strings.Contains(m.View(), "nothing to discard/update") {
		t.Fatal(m.View())
	}
}

func TestHelpControlsSurviveFeedbackAndWidths(t *testing.T) {
	for _, width := range []int{40, 60, 80, 100, 120} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := NewSourceInstallerModel([]SourceGroup{{Name: "one", Skills: []skill.Metadata{{ID: "example"}}, Installed: map[string]bool{"example": true}}})
			m.width, m.height = width, 24
			m = press(m, "d")
			view := m.View()
			assertFits(t, view, width, 24)
			for _, control := range []string{"space select install/update", "d remove", "r discard/update", "enter review", "tab/shift+tab sources", "b blackjack", "q/esc/ctrl+c cancel"} {
				if !strings.Contains(view, control) {
					t.Fatalf("missing %s:\n%s", control, view)
				}
			}
			m = press(m, "tab")
			if len(m.SelectedRemovals()) != 1 || !strings.Contains(m.View(), "[-] example") {
				t.Fatal(m.View())
			}
		})
	}
}

func TestReviewKeepsControlsForLargeQueue(t *testing.T) {
	m := NewInstallerModel(nil, nil, nil)
	m.width, m.height = 40, 24
	for i := 0; i < 100; i++ {
		m.discards[fmt.Sprintf("skill-%d", i)] = true
	}
	m.review = true
	view := m.View()
	assertFits(t, view, 40, 24)
	for _, text := range []string{"DISCARD + UPDATE", "c confirm", "esc/n back", "q/ctrl+c cancel", "without backup"} {
		if !strings.Contains(view, text) {
			t.Fatal(view)
		}
	}
}
