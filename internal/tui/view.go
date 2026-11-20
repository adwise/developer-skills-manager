package tui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Catalog text is data, not terminal control sequences. Keep each field on one line.
func displayText(s string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(s))), " ")
}

func field(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
}

func (m InstallerModel) paint(s, style string) string {
	if !m.color || s == "" {
		return s
	}
	return "\x1b[" + style + "m" + s + "\x1b[0m"
}

func (m InstallerModel) View() string {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		return ""
	}
	if m.review {
		return m.reviewView(w, h)
	}
	if m.playing {
		return fitView(wrapLines(strings.TrimSuffix(m.game.view(), "\n")+"\nctrl+c cancel installer", w), w, h)
	}
	header := []string{m.paint("SKILLS / Manage", "1;36")}
	if len(m.tabs) > 0 {
		strip := m.tabstrip()
		header = append(header, strip)
		if w >= 24 && h >= 10 {
			header = append(header, m.tabDivider(strip))
		}
	}
	queue := fmt.Sprintf("%s  %d install · %d update · %d remove", m.queueSummary(), len(m.Selected()), len(m.SelectedUpdates()), len(m.SelectedRemovals()))
	if w < 60 {
		queue = fmt.Sprintf("%s +%d ↑%d −%d", m.queueSummary(), len(m.Selected()), len(m.SelectedUpdates()), len(m.SelectedRemovals()))
	}
	if n := len(m.SelectedDiscards()); n > 0 {
		queue += fmt.Sprintf(" · %d discard", n)
	}
	if len(m.rows) == 0 {
		lines := append(header, "No skills are available.", queue)
		lines = append(lines, m.helpLines()...)
		if m.status != "" {
			lines = append(lines, m.status)
		}
		return fitView(lines, w, h)
	}
	current := m.rows[m.cursor]
	// Below this size, keep focus and essential keys instead of squeezing panels.
	if h < 7+len(header) || w < 24 {
		lines := []string{"> " + displayText(m.label(current)), displayText(m.rowStatus(current)), queue, "↑/↓ move · space select", "enter review · q cancel"}
		if len(m.tabs) > 0 {
			lines = append([]string{m.tabstrip()}, append(lines[:3], "tab/shift+tab sources", "enter review · q cancel")...)
		}
		return fitView(lines, w, h)
	}

	side := w >= 110 && h >= 14
	listWidth := w
	if side {
		listWidth = w * 3 / 5
	}
	details := m.details(current, w, 2)
	if side {
		details = m.details(current, w-listWidth-3, 4)
	} else if h < 16 {
		details = nil
	}
	footer := append([]string{queue}, m.helpLines()...)
	if m.status != "" {
		footer = append(footer, wrapLines(m.status, w)...)
	}
	// Reserve help independently of feedback; neither overwrites the other.
	pageSize := h - 3 - len(header) - len(footer)
	if pageSize-len(details)-1 < 2 && !side {
		details = nil
	}
	if !side && len(details) > 0 {
		pageSize -= len(details) + 1
	}
	pageSize = max(1, pageSize)
	// Headers occupy viewport lines but never become keyboard-selectable rows.
	type listLine struct {
		index int // -1 denotes a group header; -2 a spacer
		kind  rowKind
		group string
	}
	var list []listLine
	focus := 0
	for i, item := range m.rows {
		if i == 0 || item.kind != m.rows[i-1].kind || (m.activeTab != 0 && item.group != m.rows[i-1].group) {
			if i > 0 {
				list = append(list, listLine{index: -2})
			}
			list = append(list, listLine{index: -1, kind: item.kind, group: item.group})
		}
		if i == m.cursor {
			focus = len(list)
		}
		list = append(list, listLine{index: i, kind: item.kind, group: item.group})
	}
	start := min(max(0, focus-pageSize/2), max(0, len(list)-pageSize))
	// Repeat the current group above scrolled rows when there is room for focus.
	sticky := start > 0 && list[start].index >= 0 && pageSize > 1
	if sticky && focus >= start+pageSize-1 {
		start = focus - pageSize + 2
		sticky = list[start].index >= 0
	}
	visible := list[start:min(len(list), start+pageSize)]
	if sticky {
		visible = append([]listLine{{index: -1, kind: list[start].kind, group: list[start].group}}, list[start:min(len(list), start+pageSize-1)]...)
	}
	first, last := m.cursor+1, m.cursor+1
	for _, entry := range visible {
		if entry.index >= 0 {
			first = min(first, entry.index+1)
			last = max(last, entry.index+1)
		}
	}
	nameWidth := 4
	for _, item := range m.rows {
		nameWidth = max(nameWidth, ansi.StringWidth(displayText(m.label(item))))
	}
	nameWidth = min(nameWidth, max(1, listWidth-20))
	columns := func(name, status string) string {
		return strings.TrimRight(field(displayText(name), nameWidth)+"  "+field(displayText(status), listWidth-nameWidth-8), " ")
	}
	lines := append(header, fmt.Sprintf("Skills %d · Sets %d", m.skillCount(), len(m.rows)-m.skillCount()), m.paint("      "+columns("Name", "Status"), "2"))
	for i := 0; i < pageSize; i++ {
		line := ""
		if i < len(visible) {
			entry := visible[i]
			if entry.index == -1 {
				label := "SKILLS"
				if entry.kind == setRow {
					label = "SETS"
				}
				line = m.paint("── "+label+" "+strings.Repeat("─", max(0, listWidth-ansi.StringWidth(label)-4)), "1;36")
			} else if entry.index >= 0 {
				item := m.rows[entry.index]
				cursor := "  "
				if entry.index == m.cursor {
					cursor = m.paint("> ", "1")
				}
				name := field(displayText(m.label(item)), nameWidth)
				if item.kind == setRow {
					name = m.paint(name, "1;36")
				} else if entry.index == m.cursor {
					name = m.paint(name, "1")
				}
				status := field(displayText(m.compactStatus(item)), listWidth-nameWidth-8)
				line = cursor + m.marker(item) + " " + name + "  " + m.paint(strings.TrimRight(status, " "), m.statusStyle(item))
				line = strings.TrimRight(line, " ")
			}
		}
		if side {
			line += strings.Repeat(" ", max(0, listWidth-ansi.StringWidth(line))) + m.paint(" │", "2")
			if i < len(details) && details[i] != "" {
				line += " " + details[i]
			}
		}
		lines = append(lines, line)
	}
	lines = append(lines, m.paint(fmt.Sprintf("%d–%d of %d", first, last, len(m.rows)), "2"))
	if !side && len(details) > 0 {
		lines = append(lines, m.paint(strings.Repeat("─", w), "2"))
		lines = append(lines, details...)
	}
	lines = append(lines, footer...)
	return fitView(lines, w, h)
}

func fitView(lines []string, width, height int) string {
	lines = lines[:min(len(lines), height)]
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}
	// No trailing newline: it would occupy another terminal row.
	return strings.Join(lines, "\n")
}

func rowLabel(item row) string {
	if item.kind == setRow {
		if item.group != "" {
			name, id, _ := strings.Cut(item.id, "/")
			return name + "/@" + id
		}
		return "@" + item.id
	}
	return item.id
}

func (m InstallerModel) skillCount() int {
	n := 0
	for _, item := range m.rows {
		if item.kind == skillRow {
			n++
		}
	}
	return n
}

func (m InstallerModel) marker(item row) string {
	switch {
	case item.kind == setRow:
		for _, id := range item.skills {
			if m.discards[id] {
				return "[R]"
			}
		}
		for _, id := range item.skills {
			if m.removals[id] {
				return "[-]"
			}
		}
		if m.setCovered(item.skills) {
			return "[x]"
		}
	case m.discards[item.id]:
		return "[R]"
	case m.removals[item.id]:
		return "[-]"
	case m.selected[item.id]:
		return "[x]"
	case m.updates[item.id]:
		if state := m.states[item.id]; state == "local" || state == "both" || strings.HasPrefix(state, "unknown") {
			return "[!]"
		}
		return "[↑]"
	case m.installed[item.id]:
		return "[✓]"
	}
	return "[ ]"
}

func (m InstallerModel) compactStatus(item row) string {
	if item.kind == setRow {
		installed, queued := 0, 0
		for _, id := range item.skills {
			if m.installed[id] {
				installed++
			}
			if m.selected[id] || m.removals[id] || m.discards[id] {
				queued++
			}
		}
		return fmt.Sprintf("set · %d/%d installed · %d queued", installed, len(item.skills), queued)
	}
	if m.discards[item.id] {
		return "discard (queued)"
	}
	status := m.rowStatus(item)
	if strings.HasPrefix(status, "unknown") {
		return "unknown"
	}
	if status == "update available" {
		return "upstream"
	}
	if status != "" {
		return status
	}
	if m.installed[item.id] {
		return "installed"
	}
	return "available"
}

func (m InstallerModel) statusStyle(item row) string {
	if item.kind == setRow {
		return "36"
	}
	status := m.compactStatus(item)
	switch {
	case m.discards[item.id]:
		return "1;35"
	case m.removals[item.id] || status == "both":
		return "1;31"
	case m.selected[item.id]:
		return "1"
	case status == "local" || strings.HasPrefix(status, "unknown"):
		return "1;33"
	case status == "upstream":
		return "1;36"
	case status == "clean" || status == "installed":
		return "32"
	default:
		return "2"
	}
}

func (m InstallerModel) details(item row, width, descriptionLines int) []string {
	action := "space install"
	switch {
	case item.kind == setRow:
		action = "space select installs/updates · r discard installed members · d unavailable (use skill rows)"
	case m.discards[item.id]:
		action = "r undo discard · enter review"
	case m.removals[item.id]:
		action = "d undo removal"
	case m.selected[item.id]:
		action = "space deselect"
	case m.installed[item.id] && m.updates[item.id]:
		action = "space update · r discard and update · d remove"
	case m.installed[item.id]:
		action = "r discard and update · d remove"
	}
	state := m.rowStatus(item)
	if state == "" {
		state = m.compactStatus(item)
	}
	if original := m.states[item.id]; original != "" && original != state {
		state += " · " + original
	}
	description := displayText(item.description)
	if description == "" {
		description = "No description provided."
	}
	wrapped := wrapLines(description, width)
	if len(wrapped) > descriptionLines {
		wrapped[descriptionLines-1] = ansi.Truncate(strings.Join(wrapped[descriptionLines-1:], " "), width, "…")
		wrapped = wrapped[:descriptionLines]
	}
	for len(wrapped) < descriptionLines {
		wrapped = append(wrapped, "")
	}
	lines := append([]string{m.paint("DETAIL / "+displayText(m.label(item)), "1")}, wrapped...)
	lines = append(lines, wrapLines("Status: "+displayText(state), width)...)
	return append(lines, wrapLines("Queue: "+action, width)...)
}

// Wrap whole controls rather than truncating a universal one-line legend.
func (m InstallerModel) helpLines() []string {
	controls := []string{"space select install/update", "d remove", "r discard/update", "enter review"}
	if len(m.tabs) > 0 {
		controls = append(controls, "tab/shift+tab sources")
	}
	controls = append(controls, "b blackjack", "q/esc/ctrl+c cancel")
	var lines []string
	line := ""
	for _, control := range controls {
		if line != "" && ansi.StringWidth(line+" · "+control) > m.width {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " · "
		}
		line += control
	}
	return append(lines, line)
}
