package tui

import (
	"io"
	"sort"

	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
)

// SourceGroup contains source-local IDs. Qualification happens only at the UI
// boundary; sets never select an identically named skill from another source.
type SourceGroup struct {
	Name      string
	Namespace string
	Skills    []skill.Metadata
	Sets      []source.SkillSet
	Installed map[string]bool
	Updates   map[string]bool
	States    map[string]string
}

func NewSourceInstallerModel(groups []SourceGroup) InstallerModel {
	m := NewInstallerModelWithStatus(nil, nil, make(map[string]bool), make(map[string]bool), make(map[string]string))
	m.tabs = []sourceTab{{name: "All sources"}}
	for _, group := range groups {
		start := len(m.rows)
		prefix := group.Name + "/"
		label := group.Name + " (" + group.Namespace + ")"
		for _, set := range group.Sets {
			ids := make([]string, 0, len(set.Skills))
			for _, id := range set.Skills {
				ids = append(ids, prefix+id)
			}
			m.rows = append(m.rows, row{group: label, kind: setRow, id: prefix + set.Name, description: set.Description, skills: ids})
		}
		for _, item := range group.Skills {
			m.rows = append(m.rows, row{group: label, kind: skillRow, id: prefix + item.ID, description: item.Description})
		}
		m.tabs = append(m.tabs, sourceTab{name: group.Name, rows: m.rows[start:]})
		for id, value := range group.Installed {
			m.installed[prefix+id] = value
		}
		for id, value := range group.Updates {
			m.updates[prefix+id] = value
		}
		for id, value := range group.States {
			m.states[prefix+id] = value
		}
	}
	// Keep source tabs in their original order while sorting an independent
	// overview by kind and qualified label.
	m.rows = append([]row(nil), m.rows...)
	sort.SliceStable(m.rows, func(i, j int) bool {
		if m.rows[i].kind != m.rows[j].kind {
			return m.rows[i].kind < m.rows[j].kind
		}
		return rowLabel(m.rows[i]) < rowLabel(m.rows[j])
	})
	m.tabs[0].rows = m.rows
	return m
}

func RunSourceInstaller(input io.Reader, output io.Writer, groups []SourceGroup) (InstallerSelection, error) {
	return runInstallerModel(input, output, NewSourceInstallerModel(groups))
}
