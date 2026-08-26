//go:build !windows

package main

// attachConsole is a no-op outside Windows, where a process launched from a
// terminal always inherits usable standard streams.
func attachConsole() {}
