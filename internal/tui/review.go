package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (m InstallerModel) queueCount() int {
	return len(m.Selected()) + len(m.SelectedUpdates()) + len(m.SelectedRemovals()) + len(m.SelectedDiscards())
}

func (m InstallerModel) reviewLines() []string {
	groups := []struct {
		name, style string
		ids         []string
	}{
		{"INSTALL", "1;32", m.Selected()},
		{"UPDATE", "1;36", m.SelectedUpdates()},
		{"REMOVE", "1;31", m.SelectedRemovals()},
		{"DISCARD + UPDATE", "1;35", m.SelectedDiscards()},
	}
	var lines []string
	for _, group := range groups {
		if len(group.ids) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.paint(fmt.Sprintf("%s · %d", group.name, len(group.ids)), group.style))
		for _, id := range group.ids {
			lines = append(lines, strings.Split(ansi.Hardwrap("  "+displayText(id), max(1, m.width), true), "\n")...)
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "Nothing queued. Go back to select skills.")
	}
	return lines
}

func (m InstallerModel) reviewView(w, h int) string {
	body := m.reviewLines()
	header := []string{m.paint("SKILLS / Review queue", "1;36"), fmt.Sprintf("%d queued across all sources", m.queueCount())}
	if h >= 12 {
		header = append(header, m.paint(strings.Repeat("─", w), "2"))
	}
	var note []string
	if len(m.SelectedDiscards()) > 0 {
		note = wrapLines("Discard: local changes will be permanently overwritten without backup. Dependency diffs and a separate confirmation follow.", w)
	} else if m.queueCount() > 0 {
		note = wrapLines("Confirm to proceed; protections still apply.", w)
	}
	controls := "c confirm · esc/n back · q/ctrl+c cancel"
	if m.queueCount() == 0 {
		controls = "esc/n back · q/ctrl+c cancel"
	}
	footer := wrapLines(controls, w)
	if len(body) > 1 {
		footer = append(wrapLines("↑/↓ scroll · home/end jump", w), footer...)
	}
	if h >= 12 {
		footer = append([]string{m.paint(strings.Repeat("─", w), "2")}, footer...)
	}
	// Keep controls at the bottom even on small terminals.
	if len(footer) >= h {
		return fitView(footer[len(footer)-h:], w, h)
	}
	room := h - len(footer)
	if len(header) > room {
		header = header[:room]
	}
	room -= len(header)
	if len(note) > max(0, room-1) {
		note = note[:max(0, room-1)]
	}
	room -= len(note)
	start := min(m.reviewScroll, max(0, len(body)-max(1, room)))
	end := min(len(body), start+room)
	lines := append(header, body[start:end]...)
	// Reserve the full list viewport, even when the queue contains only one item.
	lines = append(lines, make([]string, room-(end-start))...)
	lines = append(lines, note...)
	lines = append(lines, footer...)
	return fitView(lines, w, h)
}
