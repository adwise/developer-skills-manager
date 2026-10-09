package source

import "fmt"

// ResolveDependencies returns each requested skill and its dependencies once,
// with dependencies before their dependents.
func (c CatalogData) ResolveDependencies(ids []string) ([]string, error) {
	available := make(map[string]bool, len(c.Skills))
	for _, item := range c.Skills {
		available[item.ID] = true
	}
	visiting := make(map[string]bool)
	done := make(map[string]bool)
	var resolved []string
	var visit func(string) error
	visit = func(id string) error {
		if !available[id] {
			return fmt.Errorf("skill %q is not available", id)
		}
		if visiting[id] {
			return fmt.Errorf("dependency cycle involving skill %q", id)
		}
		if done[id] {
			return nil
		}
		visiting[id] = true
		for _, dependency := range c.Dependencies[id] {
			if err := visit(dependency); err != nil {
				return fmt.Errorf("dependency of %q: %w", id, err)
			}
		}
		delete(visiting, id)
		done[id] = true
		resolved = append(resolved, id)
		return nil
	}
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return resolved, nil
}
