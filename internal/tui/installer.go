package tui

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"

	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
)

type rowKind uint8

const (
	setRow rowKind = iota
	skillRow
)

type row struct {
	group       string
	kind        rowKind
	id          string
	description string
	skills      []string
}

// InstallerSelection contains the installs, updates, and removals selected in the TUI.
type InstallerSelection struct {
	Discards []string
	Installs []string
	Updates  []string
	Removals []string
}

// InstallerModel is the Bubble Tea model for the multi-select installer.
type InstallerModel struct {
	rows         []row
	tabs         []sourceTab
	activeTab    int
	installed    map[string]bool
	updates      map[string]bool
	states       map[string]string
	selected     map[string]bool
	removals     map[string]bool
	discards     map[string]bool
	review       bool
	reviewScroll int
	cursor       int
	width        int
	height       int
	status       string
	color        bool
	complete     bool
	cancelled    bool
	playing      bool
	game         blackjack
}

func NewInstallerModel(skills []skill.Metadata, sets []source.SkillSet, installed map[string]bool) InstallerModel {
	return NewInstallerModelWithUpdates(skills, sets, installed, nil)
}

func NewInstallerModelWithUpdates(skills []skill.Metadata, sets []source.SkillSet, installed, updates map[string]bool) InstallerModel {
	return NewInstallerModelWithStatus(skills, sets, installed, updates, nil)
}

func NewInstallerModelWithStatus(skills []skill.Metadata, sets []source.SkillSet, installed, updates map[string]bool, states map[string]string) InstallerModel {
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
		states:    states,
		selected:  make(map[string]bool),
		removals:  make(map[string]bool),
		discards:  make(map[string]bool),
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
		if m.review {
			switch message.String() {
			case "esc", "n":
				m.review = false
			case "ctrl+c", "q":
				m.cancelled = true
				return m, tea.Quit
			case "up", "k":
				m.reviewScroll = max(0, m.reviewScroll-1)
			case "down", "j":
				m.reviewScroll = min(max(0, len(m.reviewLines())-1), m.reviewScroll+1)
			case "home", "g":
				m.reviewScroll = 0
			case "end", "G":
				m.reviewScroll = max(0, len(m.reviewLines())-1)
			case "c":
				if m.queueCount() > 0 {
					m.complete = true
					return m, tea.Quit
				}
			}
			return m, nil
		}
		if m.playing && message.String() != "ctrl+c" {
			switch message.String() {
			case "q", "esc":
				m.playing = false
			default:
				m.game.play(message.String())
			}
			return m, nil
		}
		switch message.String() {
		case "b":
			m.game = newBlackjack()
			m.playing = true
		case "ctrl+c", "esc", "q":
			m.cancelled = true
			return m, tea.Quit
		case "tab":
			m.switchTab(1)
		case "shift+tab":
			m.switchTab(-1)
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
		case "r":
			m.toggleDiscard()
		case "d", "delete":
			m.toggleRemoval()
		case "enter":
			m.review = true
			m.reviewScroll = 0
		}
	}
	return m, nil
}

func (m InstallerModel) rowStatus(item row) string {
	if item.kind != skillRow {
		return ""
	}
	if m.discards[item.id] {
		return "DISCARD local changes and update (queued)"
	}
	if m.removals[item.id] {
		return "remove (queued)"
	}
	if m.selected[item.id] {
		if m.installed[item.id] {
			return "update (queued)"
		}
		return "install (queued)"
	}
	if m.states[item.id] != "" {
		return m.states[item.id]
	}
	if m.updates[item.id] {
		return "update available"
	}
	return ""
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
	m.status = "Selection changed; enter reviews the queue."
	if len(m.rows) == 0 {
		m.status = "No skills to select."
		return
	}
	item := m.rows[m.cursor]
	if item.kind == skillRow {
		if m.selectable(item.id) {
			delete(m.removals, item.id)
			delete(m.discards, item.id)
			m.selected[item.id] = !m.selected[item.id]
		} else {
			m.status = "No update available; r queues discard/update, d queues removal."
		}
		return
	}

	eligible := false
	for _, id := range item.skills {
		eligible = eligible || m.selectable(id)
	}
	if !eligible {
		m.status = "No install or update available in this set."
		return
	}
	selectSet := !m.setCovered(item.skills)
	for _, id := range item.skills {
		if !m.selectable(id) {
			continue
		}
		if selectSet {
			delete(m.removals, id)
			delete(m.discards, id)
			m.selected[id] = true
		} else {
			delete(m.selected, id)
		}
	}
}

func (m *InstallerModel) toggleRemoval() {
	if len(m.rows) == 0 {
		m.status = "No installed skill to remove."
		return
	}
	item := m.rows[m.cursor]
	if item.kind != skillRow {
		m.status = "Sets cannot be removed; focus an installed skill and press d."
		return
	}
	if !m.installed[item.id] {
		m.status = "Not installed; nothing to remove."
		return
	}
	if m.removals[item.id] {
		delete(m.removals, item.id)
		m.status = "Removal unqueued; no files changed."
		return
	}
	m.status = "Removal queued; enter reviews. Local edits remain protected."
	delete(m.selected, item.id)
	delete(m.discards, item.id)
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
	return RunInstallerWithStatus(input, output, skills, sets, installed, updates, nil)
}

func RunInstallerWithStatus(input io.Reader, output io.Writer, skills []skill.Metadata, sets []source.SkillSet, installed, updates map[string]bool, states map[string]string) (InstallerSelection, error) {
	return runInstallerModel(input, output, NewInstallerModelWithStatus(skills, sets, installed, updates, states))
}

func runInstallerModel(input io.Reader, output io.Writer, initial InstallerModel) (InstallerSelection, error) {
	file, ok := output.(*os.File)
	if !ok || !term.IsTerminal(file.Fd()) {
		return InstallerSelection{}, fmt.Errorf("interactive installer requires a terminal; use list, install <name>, update <name>, or uninstall <name>")
	}
	initial.color = terminalColor()
	program := tea.NewProgram(
		initial,
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
		Discards: model.SelectedDiscards(),
		Installs: model.Selected(),
		Updates:  model.SelectedUpdates(),
		Removals: model.SelectedRemovals(),
	}, nil
}

func terminalColor() bool {
	_, noColor := os.LookupEnv("NO_COLOR")
	return !noColor && os.Getenv("TERM") != "dumb"
}

func truncate(value string, width int) string {
	if width <= 0 {
		return value
	}
	return ansi.Truncate(value, width, "…")
}

func wrapLines(text string, width int) []string {
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		line := ""
		for _, word := range strings.Fields(paragraph) {
			if line != "" && width > 0 && ansi.StringWidth(line+" "+word) > width {
				lines = append(lines, line)
				line = ""
			}
			if line != "" {
				line += " "
			}
			line += word
		}
		lines = append(lines, truncate(line, width))
	}
	return lines
}
