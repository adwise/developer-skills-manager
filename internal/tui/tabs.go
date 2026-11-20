package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Tabs own only the visible rows and focus; action maps remain global.
type sourceTab struct {
	name   string
	rows   []row
	cursor int
}

func (m *InstallerModel) switchTab(delta int) {
	if len(m.tabs) < 2 {
		return
	}
	// Copy before updating: Bubble Tea models are values.
	m.tabs = append([]sourceTab(nil), m.tabs...)
	m.tabs[m.activeTab].cursor = m.cursor
	m.activeTab = (m.activeTab + delta + len(m.tabs)) % len(m.tabs)
	tab := m.tabs[m.activeTab]
	m.rows = tab.rows
	m.cursor = min(tab.cursor, max(0, len(m.rows)-1))
}

func (m InstallerModel) label(item row) string {
	if m.activeTab == 0 {
		return rowLabel(item)
	}
	_, id, _ := strings.Cut(item.id, "/")
	if item.kind == setRow {
		return "@" + id
	}
	return id
}

// Keep the active tab visible, dropping whole tabs on the left as needed.
// Edge chevrons indicate that more tabs are available in either direction.
func (m InstallerModel) tabstrip() string {
	labels := make([]string, len(m.tabs))
	for i, tab := range m.tabs {
		label := " " + displayText(tab.name) + " "
		style := "2"
		if i == m.activeTab {
			label = "[" + displayText(tab.name) + "]"
			style = "1;36"
		}
		labels[i] = m.paint("╭"+label+"╮", style)
	}
	start := 0
	for start < m.activeTab && ansi.StringWidth(strings.Join(labels[start:m.activeTab+1], " ")) > m.width-4 {
		start++
	}
	prefix := ""
	if start > 0 {
		prefix = "‹ "
	}
	text := prefix + strings.Join(labels[start:], " ")
	if ansi.StringWidth(text) > m.width {
		if m.width <= 2 {
			return ansi.Truncate(labels[m.activeTab], m.width, "")
		}
		text = ansi.Truncate(text, m.width-2, "") + " ›"
	}
	return text
}

// The open section of the divider joins the active tab to the content below.
func (m InstallerModel) tabDivider(strip string) string {
	plain := ansi.Strip(strip)
	active := "╭[" + displayText(m.tabs[m.activeTab].name) + "]╮"
	pos := strings.Index(plain, active)
	if pos < 0 {
		return m.paint(strings.Repeat("─", m.width), "2")
	}
	left := ansi.StringWidth(plain[:pos])
	width := ansi.StringWidth(active)
	return m.paint(strings.Repeat("─", left), "2") +
		m.paint("╯"+strings.Repeat(" ", max(0, width-2))+"╰", "1;36") +
		m.paint(strings.Repeat("─", max(0, m.width-left-width)), "2")
}

func (m InstallerModel) queueSummary() string {
	scope := "Queue"
	if len(m.tabs) > 0 {
		scope = "Queue (all)"
	}
	return scope
}
