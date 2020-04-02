// +build windows

package net

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

const (
	statusURL      = "http://localhost:3333/status"
	connectionsURL = "http://localhost:3333/connections"
	statsURL       = "http://localhost:3333/debug/stats"
	netType        = "tcp"
)

// CheckPath is used in conjunction with calling the stats endpoint, since we are calling this
// From the main agent and want to ensure the probe listener exists
func CheckPath() error {
	return ebpf.ErrNotImplemented
}
