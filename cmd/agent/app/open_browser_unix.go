// +build freebsd netbsd openbsd solaris dragonfly linux

package app

// opens a browser window at the specified URL
import "os/exec"

func open(url string) error {
	return exec.Command("xdg-open", url).Start()
}
