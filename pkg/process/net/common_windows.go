// +build windows

package net

import "fmt"

const (
	connectionsURL = "http://localhost:3333/connections"
	statsURL       = "http://localhost:3333/debug/stats"
	// procStatsURL is not used in windows, the value is added to avoid compilation error in windows
	procStatsURL = "http://localhost:3333/proc/stats"
	netType      = "tcp"
)

// CheckPath is used to make sure the globalSocketPath has been set before attempting to connect
func CheckPath() error {
	if globalSocketPath == "" {
		return fmt.Errorf("remote tracer has no path defined")
	}
	return nil
}
