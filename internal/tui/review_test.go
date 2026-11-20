package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestReviewTransitions(t *testing.T) {
	for _, kind := range []string{"empty", "install", "update", "remove", "discard", "mixed"} {
		t.Run(kind, func(t *testing.T) {
			m := NewInstallerModelWithUpdates(nil, nil, map[string]bool{"two/update": true, "one/remove": true}, map[string]bool{"two/update": true})
			if kind == "install" || kind == "mixed" {
				m.selected["one/install"] = true
			}
			if kind == "update" || kind == "mixed" {
				m.selected["two/update"] = true
			}
			if kind == "remove" || kind == "mixed" {
				m.removals["one/remove"] = true
			}
			if kind == "discard" || kind == "mixed" {
				m.discards["two/discard"] = true
			}
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(InstallerModel)
			if cmd != nil || m.complete || !m.review {
				t.Fatal("enter must only open review")
			}
			before := m.queueCount()
			next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(InstallerModel)
			if cmd != nil || m.complete {
				t.Fatal("enter applied queue")
			}
			next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			m = next.(InstallerModel)
			if m.review || m.cancelled || m.queueCount() != before {
				t.Fatal("back lost queue")
			}
			next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(InstallerModel)
			next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
			m = next.(InstallerModel)
			if (before > 0) != m.complete || (before > 0) != (cmd != nil) {
				t.Fatal("unexpected confirmation outcome")
			}
		})
	}
	m := previewModel()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(InstallerModel)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = next.(InstallerModel)
	if !m.cancelled || m.complete || cmd == nil {
		t.Fatal("cancel did not abort")
	}
}

func TestReviewSnapshotAndScroll(t *testing.T) {
	m := NewInstallerModelWithUpdates(nil, nil, map[string]bool{"team/update": true, "personal/remove": true}, map[string]bool{"team/update": true})
	m.selected["team/install"] = true
	m.selected["team/update"] = true
	m.removals["personal/remove"] = true
	m.discards["personal/discard"] = true
	m.review = true
	m.width, m.height = 80, 24
	view := m.View()
	for _, s := range []string{"INSTALL · 1", "UPDATE · 1", "REMOVE · 1", "DISCARD + UPDATE · 1", "team/install", "personal/discard"} {
		if !strings.Contains(view, s) {
			t.Fatal(view)
		}
	}
	path := filepath.Join("..", "..", "docs", "tui", "review.txt")
	if os.Getenv("UPDATE_TUI_SNAPSHOTS") == "1" {
		if err := os.WriteFile(path, []byte(view+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if view+"\n" != string(want) {
		t.Fatalf("snapshot differs:\n%s", view)
	}
	m.height = 8
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = next.(InstallerModel)
	if !strings.Contains(m.View(), "personal/discard") || !strings.Contains(m.View(), "c confirm") {
		t.Fatal(m.View())
	}
	for _, w := range []int{1, 10, 40, 80} {
		for _, h := range []int{1, 3, 8, 24} {
			m.width, m.height = w, h
			assertFits(t, m.View(), w, h)
		}
	}
}
