// +build freebsd linux

package pidfile

import (
	"os"
	"path/filepath"
	"strconv"
)

// isProcess searches for the PID under /proc
func isProcess(pid int) bool {
	_, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid)))
	return err == nil
}

// Path returns a suitable location for the pidfile under Linux
func Path() string {
	return "/var/run/datadog/datadog-agent.pid"
}
