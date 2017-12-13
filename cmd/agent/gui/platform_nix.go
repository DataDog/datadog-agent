// +build freebsd netbsd openbsd solaris dragonfly linux

package gui

import (
	"fmt"
)

func restartEnabled() bool {
	return false
}

func restart() error {
	return fmt.Errorf("restarting the agent is not implemented on non-windows platforms")
}
