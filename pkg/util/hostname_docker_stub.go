// +build freebsd netbsd openbsd solaris dragonfly

package util

func getContainerHostname() (bool, string) {
	return false, ""
}
