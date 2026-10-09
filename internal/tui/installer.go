package tui

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
)

type rowKind uint8

const (
	setRow rowKind = iota
	skillRow
)

type row struct {
	kind        rowKind
	id          string
	description string
	skills      []string
}

// InstallerSelection contains the installs, updates, and removals selected in the TUI.
type InstallerSelection struct {
	Installs []string
	Updates  []string
	Removals []string
}

// InstallerModel is the Bubble Tea model for the multi-select installer.
type InstallerModel struct {
	rows      []row
	installed map[string]bool
	updates   map[string]bool
	selected  map[string]bool
	removals  map[string]bool
	cursor    int
	width     int
	height    int
	status    string
	complete  bool
	cancelled bool
}

func NewInstallerModel(skills []skill.Metadata, sets []source.SkillSet, installed map[string]bool) InstallerModel {
	return NewInstallerModelWithUpdates(skills, sets, installed, nil)
}

func NewInstallerModelWithUpdates(skills []skill.Metadata, sets []source.SkillSet, installed, updates map[string]bool) InstallerModel {
	rows := make([]row, 0, len(skills)+len(sets))
	for _, set := range sets {
		rows = append(rows, row{
			kind:        setRow,
			id:          set.Name,
			description: set.Description,
			skills:      append([]string(nil), set.Skills...),
		})
	}
	for _, item := range skills {
		rows = append(rows, row{kind: skillRow, id: item.ID, description: item.Description})
	}
	return InstallerModel{
		rows:      rows,
		installed: installed,
		updates:   updates,
		selected:  make(map[string]bool),
		removals:  make(map[string]bool),
		width:     100,
		height:    24,
	}
}

func (m InstallerModel) Init() tea.Cmd { return nil }

func (m InstallerModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
	case tea.KeyMsg:
		switch message.String() {
		case "ctrl+c", "esc", "q":
			m.cancelled = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor+1 < len(m.rows) {
				m.cursor++
			}
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			if len(m.rows) > 0 {
				m.cursor = len(m.rows) - 1
			}
		case " ":
			m.toggleCurrent()
			m.status = ""
		case "d", "delete":
			m.toggleRemoval()
			m.status = ""
		case "enter":
			if len(m.Selected()) == 0 && len(m.SelectedUpdates()) == 0 && len(m.SelectedRemovals()) == 0 {
				m.status = "Select at least one skill to install, update, or remove."
				break
			}
			m.complete = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m InstallerModel) View() string {
	var output strings.Builder
	output.WriteString("Manage skills\n\n")
	if len(m.rows) == 0 {
		output.WriteString("No skills are available.\n\nq quit\n")
		return output.String()
	}

	pageSize := m.height - 9
	if pageSize < 3 {
		pageSize = 3
	}
	start := 0
	if m.cursor >= pageSize {
		start = m.cursor - pageSize + 1
	}
	end := start + pageSize
	if end > len(m.rows) {
		end = len(m.rows)
	}
	if end-start < pageSize && start > 0 {
		start = end - pageSize
		if start < 0 {
			start = 0
		}
	}

	previousKind := rowKind(255)
	for index := start; index < end; index++ {
		item := m.rows[index]
		if item.kind != previousKind {
			if index != start {
				output.WriteByte('\n')
			}
			if item.kind == setRow {
				output.WriteString("Skill sets\n")
			} else {
				output.WriteString("Skills\n")
			}
			previousKind = item.kind
		}

		cursor := "  "
		if index == m.cursor {
			cursor = "> "
		}
		mark := "[ ]"
		label := item.id
		if item.kind == setRow {
			label = "@" + label
			if m.setCovered(item.skills) {
				mark = "[x]"
			}
		} else if m.removals[item.id] {
			mark = "[-]"
		} else if m.selected[item.id] {
			mark = "[x]"
		} else if m.updates[item.id] {
			mark = "[↑]"
		} else if m.installed[item.id] {
			mark = "[✓]"
		}
		line := fmt.Sprintf("%s%s %s", cursor, mark, label)
		if item.kind == skillRow && m.removals[item.id] {
			line += " — remove"
		} else if item.kind == skillRow && m.updates[item.id] {
			line += " — update available"
		}
		if item.description != "" {
			line += " — " + strings.Join(strings.Fields(item.description), " ")
		}
		output.WriteString(truncate(line, m.width))
		output.WriteByte('\n')
	}

	if start > 0 || end < len(m.rows) {
		fmt.Fprintf(&output, "\n%d–%d of %d\n", start+1, end, len(m.rows))
	}
	if m.status != "" {
		output.WriteString("\n" + m.status + "\n")
	}
	output.WriteString("\n↑/↓ move • space select • d remove • enter apply • q cancel\n")
	return output.String()
}

func (m InstallerModel) Selected() []string {
	selected := make([]string, 0, len(m.selected))
	for id, yes := range m.selected {
		if yes && !m.installed[id] {
			selected = append(selected, id)
		}
	}
	sort.Strings(selected)
	return selected
}

func (m InstallerModel) SelectedUpdates() []string {
	selected := make([]string, 0, len(m.selected))
	for id, yes := range m.selected {
		if yes && m.installed[id] && m.updates[id] {
			selected = append(selected, id)
		}
	}
	sort.Strings(selected)
	return selected
}

func (m InstallerModel) SelectedRemovals() []string {
	selected := make([]string, 0, len(m.removals))
	for id, yes := range m.removals {
		if yes && m.installed[id] {
			selected = append(selected, id)
		}
	}
	sort.Strings(selected)
	return selected
}

func (m InstallerModel) Cancelled() bool { return m.cancelled }

func (m *InstallerModel) toggleCurrent() {
	if len(m.rows) == 0 {
		return
	}
	item := m.rows[m.cursor]
	if item.kind == skillRow {
		if m.selectable(item.id) {
			delete(m.removals, item.id)
			m.selected[item.id] = !m.selected[item.id]
		}
		return
	}

	selectSet := !m.setCovered(item.skills)
	for _, id := range item.skills {
		if !m.selectable(id) {
			continue
		}
		if selectSet {
			delete(m.removals, id)
			m.selected[id] = true
		} else {
			delete(m.selected, id)
		}
	}
}

func (m *InstallerModel) toggleRemoval() {
	if len(m.rows) == 0 {
		return
	}
	item := m.rows[m.cursor]
	if item.kind != skillRow || !m.installed[item.id] {
		return
	}
	if m.removals[item.id] {
		delete(m.removals, item.id)
		return
	}
	delete(m.selected, item.id)
	m.removals[item.id] = true
}

func (m InstallerModel) setCovered(ids []string) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if m.selectable(id) && !m.selected[id] {
			return false
		}
	}
	return true
}

func (m InstallerModel) selectable(id string) bool {
	return !m.installed[id] || m.updates[id]
}

func RunInstaller(input io.Reader, output io.Writer, skills []skill.Metadata, sets []source.SkillSet, installed map[string]bool) ([]string, error) {
	selection, err := RunInstallerWithUpdates(input, output, skills, sets, installed, nil)
	return selection.Installs, err
}

func RunInstallerWithUpdates(input io.Reader, output io.Writer, skills []skill.Metadata, sets []source.SkillSet, installed, updates map[string]bool) (InstallerSelection, error) {
	program := tea.NewProgram(
		NewInstallerModelWithUpdates(skills, sets, installed, updates),
		tea.WithInput(input),
		tea.WithOutput(output),
		tea.WithAltScreen(),
	)
	result, err := program.Run()
	if err != nil {
		return InstallerSelection{}, err
	}
	model, ok := result.(InstallerModel)
	if !ok {
		return InstallerSelection{}, fmt.Errorf("installer returned an unexpected model")
	}
	if model.Cancelled() || !model.complete {
		return InstallerSelection{}, nil
	}
	return InstallerSelection{
		Installs: model.Selected(),
		Updates:  model.SelectedUpdates(),
		Removals: model.SelectedRemovals(),
	}, nil
}

func truncate(value string, width int) string {
	if width <= 1 || utf8.RuneCountInString(value) <= width {
		return value
	}
	runes := []rune(value)
	return string(runes[:width-1]) + "…"
}
