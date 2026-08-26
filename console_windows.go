//go:build windows

package main

import (
	"os"
	"syscall"
)

// attachParentConsole is the ATTACH_PARENT_PROCESS argument to AttachConsole.
const attachParentConsole = ^uintptr(0) // (DWORD)-1

// attachConsole makes CLI output visible when mdiff is run from a terminal.
//
// Wails links the release binary with -H windowsgui (PE subsystem 2), so
// Windows allocates no console for it and leaves the standard handles null.
// Writes to os.Stdout then vanish, which is why `mdiff --help` printed nothing
// in cmd.exe and PowerShell while redirected output (a pipe or a file) worked
// fine: redirection supplies a real handle, a bare console does not.
//
// Streams that already have a usable handle are left untouched, so redirecting
// output still writes to the pipe or file rather than to the terminal.
func attachConsole() {
	outDead := !stdHandleUsable(syscall.STD_OUTPUT_HANDLE)
	errDead := !stdHandleUsable(syscall.STD_ERROR_HANDLE)
	if !outDead && !errDead {
		return
	}

	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole")
	if ret, _, _ := proc.Call(attachParentConsole); ret == 0 {
		// Nothing to attach to: already own a console, or launched with no
		// parent console at all (double-clicked).
		return
	}

	con, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	if err != nil {
		return
	}
	if outDead {
		os.Stdout = con
	}
	if errDead {
		os.Stderr = con
	}
}

// stdHandleUsable reports whether the given standard handle is one we can
// actually write to, as opposed to the null handle a GUI-subsystem process is
// started with.
func stdHandleUsable(kind int) bool {
	h, err := syscall.GetStdHandle(kind)
	return err == nil && h != 0 && h != syscall.InvalidHandle
}
