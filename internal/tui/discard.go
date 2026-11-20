package tui

import "sort"

func (m InstallerModel) SelectedDiscards() []string {
	var ids []string
	for id, yes := range m.discards {
		if yes {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (m *InstallerModel) toggleDiscard() {
	if len(m.rows) == 0 {
		m.status = "No installed skills to discard/update."
		return
	}
	item := m.rows[m.cursor]
	ids := item.skills
	if item.kind == skillRow {
		ids = []string{item.id}
	}
	enable := false
	eligible := false
	for _, id := range ids {
		eligible = eligible || m.installed[id]
		if m.installed[id] && !m.discards[id] {
			enable = true
		}
	}
	if !eligible {
		m.status = "Not installed; nothing to discard/update."
		return
	}
	m.status = "Discard unqueued; no files changed."
	if enable {
		m.status = "Discard queued; enter reviews. Permanent replacement, no backup."
	}
	for _, id := range ids {
		if !m.installed[id] {
			continue
		}
		delete(m.selected, id)
		delete(m.removals, id)
		if enable {
			m.discards[id] = true
		} else {
			delete(m.discards, id)
		}
	}
}
