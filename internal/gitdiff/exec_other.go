//go:build !windows

package gitdiff

import "os/exec"

// hideConsole is a no-op on non-Windows platforms: spawning a console
// executable from a GUI process does not create a visible window there.
func hideConsole(cmd *exec.Cmd) {}
