package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func expandHome(path string) (string, error) {
	if path == "" {
		return path, nil
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

func canonicalPath(path string) (string, error) {
	expanded, err := expandHome(path)
	if err != nil {
		return "", err
	}
	if expanded == "" {
		return "", nil
	}

	info, err := os.Stat(expanded)
	if err == nil && info.IsDir() {
		resolved, err := filepath.EvalSymlinks(expanded)
		if err == nil {
			expanded = resolved
		}
		return filepath.Abs(expanded)
	}

	parent := filepath.Dir(expanded)
	parentAbs, err := filepath.Abs(parent)
	if err != nil {
		return "", err
	}
	resolvedParent, err := filepath.EvalSymlinks(parentAbs)
	if err == nil {
		parentAbs = resolvedParent
	}
	return filepath.Join(parentAbs, filepath.Base(expanded)), nil
}

// readHarnessExtraMounts reads <harnessPath>/extra-mounts.txt: one host
// directory per line, '#' comments and blank lines ignored (mirroring
// apt-packages.txt). Each entry is ~-expanded and canonicalised host-side, so
// the harness file stays portable (e.g. `~/dotscripts` resolves per user). A
// missing file yields no extra mounts. Each entry must be an existing
// directory. Paths are returned deduplicated, preserving first-seen order.
func readHarnessExtraMounts(harnessPath string) ([]string, error) {
	file := filepath.Join(harnessPath, "extra-mounts.txt")
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	seen := map[string]bool{}
	var mounts []string
	for _, raw := range strings.Split(string(data), "\n") {
		line := raw
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		resolved, err := canonicalPath(line)
		if err != nil {
			return nil, fmt.Errorf("extra-mounts.txt: %q: %w", line, err)
		}
		if err := ensureDirectory(resolved, fmt.Sprintf("extra mount %q", line)); err != nil {
			return nil, err
		}
		if seen[resolved] {
			continue
		}
		seen[resolved] = true
		mounts = append(mounts, resolved)
	}
	return mounts, nil
}

func ensureDirectory(path string, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s does not exist: %s", label, path)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory: %s", label, path)
	}
	return nil
}

func ensureSubdirectory(path string, parent string, pathLabel string, parentLabel string) error {
	relativePath, err := filepath.Rel(parent, path)
	if err != nil {
		return err
	}
	if relativePath == "." {
		return fmt.Errorf("%s must be inside %s, not equal to it: %s", pathLabel, parentLabel, path)
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("%s must be inside %s: %s is outside %s", pathLabel, parentLabel, path, parent)
	}
	return nil
}
