package tui

import (
	"strings"
	"testing"

	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
	"github.com/charmbracelet/x/ansi"
)

func TestDiscardCompactStatusAndSemanticColor(t *testing.T) {
	m := NewInstallerModelWithStatus([]skill.Metadata{{ID: "example"}}, []source.SkillSet{{Name: "work", Skills: []string{"example"}}}, map[string]bool{"example": true}, nil, map[string]string{"example": "local"})
	m.cursor = 1
	m = press(m, "r")
	item := m.rows[1]
	if m.compactStatus(item) != "discard (queued)" || m.statusStyle(item) != "1;35" {
		t.Fatal("discard must be compact and magenta")
	}
	if m.statusStyle(m.rows[0]) != "36" {
		t.Fatal("set must retain its color")
	}
	if m.marker(item) != "[R]" || !strings.Contains(m.rowStatus(item), "DISCARD local changes") {
		t.Fatal("lost detailed discard warning")
	}
	plain := m.View()
	if !strings.Contains(plain, "[R] example  discard") || strings.Contains(plain, "\x1b") {
		t.Fatal(plain)
	}
	m.color = true
	colored := m.View()
	if !strings.Contains(colored, "\x1b[1;35mdiscard (queued)\x1b[0m") || ansi.Strip(colored) != plain {
		t.Fatal("color changed text/layout", colored)
	}
	t.Setenv("NO_COLOR", "")
	m.color = terminalColor()
	if m.View() != plain {
		t.Fatal("NO_COLOR changed discard distinction")
	}
	m = press(m, "d")
	if m.compactStatus(item) != "remove (queued)" || m.statusStyle(item) != "1;31" {
		t.Fatal("remove must remain red")
	}
}

func TestSetQueueAndDisabledSelectionFeedback(t *testing.T) {
	m := NewInstallerModel([]skill.Metadata{{ID: "example"}}, []source.SkillSet{{Name: "work", Skills: []string{"example"}}}, map[string]bool{"example": true})
	m = press(m, " ")
	if !strings.Contains(m.View(), "No install or update available") {
		t.Fatal(m.View())
	}
	m = press(m, "down")
	m = press(m, " ")
	if !strings.Contains(m.View(), "No update available") {
		t.Fatal(m.View())
	}
	m = press(m, "d")
	if m.marker(m.rows[0]) != "[-]" || !strings.Contains(m.compactStatus(m.rows[0]), "1 queued") {
		t.Fatal("set hides removal")
	}
	m = press(m, "r")
	if m.marker(m.rows[0]) != "[R]" {
		t.Fatal("set hides discard")
	}
	m = press(m, "r")
	if !strings.Contains(m.View(), "Discard unqueued") {
		t.Fatal(m.View())
	}
}
