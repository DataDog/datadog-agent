// +build windows

package net

const (
	statusURL      = "http://localhost:3333/status"
	connectionsURL = "http://localhost:3333/connections"
	statsURL       = "http://localhost:3333/debug/stats"
	netType        = "tcp"
)

// SetSystemProbePath sets where teh System probe is listening for connections
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
	return nil
}
