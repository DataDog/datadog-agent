package pidfile

import (
	"syscall"
)

// isProcess uses `kill -0` to check whether a process is running
func isProcess(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// Path returns a suitable location for the pidfile under OSX
func Path() string {
	return "/var/run/datadog/datadog-agent.pid"
}
