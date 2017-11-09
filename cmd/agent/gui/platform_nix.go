// +build freebsd netbsd openbsd solaris dragonfly linux

package gui

import (
	"fmt"
	"os/exec"
)

// opens a browser window at the specified URL
func open(url string) error {
	return exec.Command("xdg-open", url).Start()
}

func restart() error {
	return fmt.Errorf("restarting the agent is not implemented on non-windows platforms")
}
