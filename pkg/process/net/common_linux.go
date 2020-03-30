// +build linux

package net

import (
	"fmt"
	"os"
)

const (
	statusURL      = "http://unix/status"
	connectionsURL = "http://unix/connections"
	statsURL       = "http://unix/debug/stats"
	netType        = "unix"
)

// SetSystemProbePath provides a unix socket path location to be used by the remote system probe.
// This needs to be called before GetRemoteSystemProbeUtil.
func SetSystemProbePath(path string) {
	globalSocketPath = path
}

// CheckPath is used in conjunction with calling the stats endpoint, since we are calling this
// From the main agent and want to ensure the socket exists
func CheckPath() error {
	if globalSocketPath == "" {
		return fmt.Errorf("remote tracer has no path defined")
	}

	if _, err := os.Stat(globalSocketPath); err != nil {
		return fmt.Errorf("socket path does not exist: %v", err)
	}
	return nil
}
