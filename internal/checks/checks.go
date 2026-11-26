// Package checks defines the built-in LLM diff checks.
package checks

import "slices"

// Check is a single built-in LLM check: a named prompt run against a diff.
type Check struct {
	Name        string
	Description string
	Prompt      string
}

// builtins is the catalog of built-in checks, in the order the UI shows
// their buttons.
var builtins = []Check{languageConsistency}

// List returns every built-in check, in button order. The result is a copy,
// so a caller can modify it without affecting the catalog.
func List() []Check {
	return slices.Clone(builtins)
}

// Find returns the built-in check named name, or false if none matches.
func Find(name string) (Check, bool) {
	i := slices.IndexFunc(builtins, func(c Check) bool { return c.Name == name })
	if i == -1 {
		return Check{}, false
	}
	return builtins[i], true
}
