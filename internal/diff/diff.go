package diff

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type operation struct {
	kind byte
	line string
}

func Directories(installed, available, name string) (string, error) {
	oldFiles, err := files(installed)
	if err != nil {
		return "", err
	}
	newFiles, err := files(available)
	if err != nil {
		return "", err
	}

	paths := make(map[string]struct{}, len(oldFiles)+len(newFiles))
	for path := range oldFiles {
		paths[path] = struct{}{}
	}
	for path := range newFiles {
		paths[path] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)

	var output strings.Builder
	for _, path := range ordered {
		oldData, oldExists := oldFiles[path]
		newData, newExists := newFiles[path]
		oldExecutable, newExecutable := false, false
		if oldExists {
			info, err := os.Stat(filepath.Join(installed, filepath.FromSlash(path)))
			if err != nil {
				return "", err
			}
			oldExecutable = info.Mode()&0o111 != 0
		}
		if newExists {
			info, err := os.Stat(filepath.Join(available, filepath.FromSlash(path)))
			if err != nil {
				return "", err
			}
			newExecutable = info.Mode()&0o111 != 0
		}
		if oldExists == newExists && bytes.Equal(oldData, newData) && oldExecutable == newExecutable {
			continue
		}
		fmt.Fprintf(&output, "--- before/%s/%s\n+++ after/%s/%s\n", name, path, name, path)
		if !oldExists {
			output.WriteString("File added\n")
		}
		if !newExists {
			output.WriteString("File deleted\n")
		}
		if oldExecutable != newExecutable {
			fmt.Fprintf(&output, "Executable: %t → %t\n", oldExecutable, newExecutable)
		}
		if bytes.IndexByte(oldData, 0) >= 0 || bytes.IndexByte(newData, 0) >= 0 {
			output.WriteString("Binary files differ\n")
			continue
		}
		oldLines := splitLines(string(oldData))
		newLines := splitLines(string(newData))
		fmt.Fprintf(&output, "@@ -1,%d +1,%d @@\n", len(oldLines), len(newLines))
		for _, op := range lineDiff(oldLines, newLines) {
			output.WriteByte(op.kind)
			output.WriteString(op.line)
			output.WriteByte('\n')
		}
	}
	return output.String(), nil
}

func files(root string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return result, nil
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(rel)] = contents
		return nil
	})
	return result, err
}

func splitLines(value string) []string {
	value = strings.TrimSuffix(value, "\n")
	if value == "" {
		return nil
	}
	return strings.Split(value, "\n")
}

func lineDiff(oldLines, newLines []string) []operation {
	lcs := make([][]int, len(oldLines)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(newLines)+1)
	}
	for i := len(oldLines) - 1; i >= 0; i-- {
		for j := len(newLines) - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var operations []operation
	for i, j := 0, 0; i < len(oldLines) || j < len(newLines); {
		switch {
		case i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j]:
			operations = append(operations, operation{' ', oldLines[i]})
			i++
			j++
		case j < len(newLines) && (i == len(oldLines) || lcs[i][j+1] > lcs[i+1][j]):
			operations = append(operations, operation{'+', newLines[j]})
			j++
		default:
			operations = append(operations, operation{'-', oldLines[i]})
			i++
		}
	}
	return operations
}
