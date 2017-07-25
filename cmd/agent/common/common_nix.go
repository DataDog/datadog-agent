// +build freebsd netbsd openbsd solaris dragonfly linux

package common

import (
	"path/filepath"
)

const defaultConfPath = "/etc/dd-agent"

// DistPath holds the path to the folder containing distribution files
var distPath = filepath.Join(_here, "dist")

// GetDistPath returns the fully qualified path to the 'dist' directory
func GetDistPath() string {
	return distPath
}
