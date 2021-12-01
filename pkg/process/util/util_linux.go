// +build linux

package util

import (
	"os"
	"path/filepath"
	"strconv"
)

// GetRootNSPID returns the current PID from the root namespace
func GetRootNSPID() (int, error) {
	pidPath := filepath.Join(GetProcRoot(), "self")
	pidStr, err := os.Readlink(pidPath)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(pidStr)
}
