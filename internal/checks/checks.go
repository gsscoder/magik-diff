// Package checks handles loading and parsing user-defined LLM checks:
// .md files with a YAML frontmatter block followed by a prompt body.
package checks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"mdiff/internal/config"
)

const checksDirName = "checks"

// Check is a single user-defined LLM check parsed from a .md file.
type Check struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Color       string `yaml:"color"`
	Prompt      string `yaml:"-"`
}

// Dir returns the directory checks are loaded from and installed into,
// using the same XDG_CONFIG_HOME / ~/.config/mdiff base resolution as
// internal/config.
func Dir() (string, error) {
	base, err := config.BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, checksDirName), nil
}

// ParseFile parses a single check file at path: a YAML frontmatter block
// delimited by "---" lines, followed by the prompt body.
func ParseFile(path string) (Check, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Check{}, err
	}

	frontmatter, prompt, err := splitFrontmatter(string(data))
	if err != nil {
		return Check{}, fmt.Errorf("%s: %w", path, err)
	}

	var c Check
	if err := yaml.Unmarshal([]byte(frontmatter), &c); err != nil {
		return Check{}, fmt.Errorf("%s: invalid frontmatter: %w", path, err)
	}
	c.Prompt = prompt

	return c, nil
}

// splitFrontmatter splits raw into its YAML frontmatter block and the
// trimmed remainder, given content of the form:
//
//	---
//	<frontmatter>
//	---
//	<prompt body>
func splitFrontmatter(raw string) (frontmatter, body string, err error) {
	content := strings.TrimSpace(raw)
	if !strings.HasPrefix(content, "---") {
		return "", "", fmt.Errorf("missing frontmatter delimiter")
	}
	content = content[len("---"):]
	content = strings.TrimPrefix(content, "\r\n")
	content = strings.TrimPrefix(content, "\n")

	end := strings.Index(content, "\n---")
	if end == -1 {
		return "", "", fmt.Errorf("missing closing frontmatter delimiter")
	}

	frontmatter = content[:end]
	rest := content[end+len("\n---"):]
	// Skip to the end of the closing delimiter's line.
	if nl := strings.IndexByte(rest, '\n'); nl != -1 {
		rest = rest[nl+1:]
	} else {
		rest = ""
	}

	return frontmatter, strings.TrimSpace(rest), nil
}

// List reads and parses every *.md file in the checks directory, returning
// them sorted by Name. A missing checks directory is not an error; it
// yields an empty slice. A malformed check file is skipped rather than
// aborting the whole listing.
func List() ([]Check, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Check{}, nil
		}
		return nil, err
	}

	result := []Check{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		c, err := ParseFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		result = append(result, c)
	}

	slices.SortFunc(result, func(a, b Check) int {
		return strings.Compare(a.Name, b.Name)
	})

	return result, nil
}
