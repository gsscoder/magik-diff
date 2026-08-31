//go:build windows

package watch

import (
	"os/exec"
	"syscall"
)

// hideConsole prevents a console window from briefly flashing when git (a
// console-subsystem executable) is spawned from this GUI process, which has
// no console of its own.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
