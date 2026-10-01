// Package system gathers facts about the computer DevDoctor runs on.
package system

import "runtime"

// Info describes the machine DevDoctor is running on.
type Info struct {
	OS           string // e.g. "windows", "linux", "darwin"
	Architecture string // e.g. "amd64", "arm64"
}

// GetInfo returns basic facts about the current machine.
func GetInfo() Info {
	return Info{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
	}
}
