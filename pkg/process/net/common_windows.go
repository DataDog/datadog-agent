// +build windows

package net

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

const (
	statusURL      = "http://localhost:3333/status"
	connectionsURL = "http://localhost:3333/connections"
	statsURL       = "http://localhost:3333/debug/stats"
	netType        = "tcp"
)

// SetSystemProbePath sets where the System probe is listening for connections
// This needs to be called before GetRemoteSystemProbeUtil.
func SetSystemProbePath(path string) {
	globalSocketPath = path
}

// CheckPath is used in conjunction with calling the stats endpoint, since we are calling this
// From the main agent and want to ensure the probe listener exists
func CheckPath() error {
	if globalSocketPath == "" {
		return fmt.Errorf("remote tracer has no path defined")
	}
	return ebpf.ErrNotImplemented
}
