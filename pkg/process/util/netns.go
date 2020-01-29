// +build linux

package util

import (
	"fmt"
	"os"
	"syscall"
)

// IsRootNS determines whether the current thread is attached to the root network namespace
func IsRootNS(procRoot string) bool {
	rootPath := fmt.Sprintf("%s/1/ns/net", procRoot)
	rootNS, err := os.Open(rootPath)
	if err != nil {
		return false
	}
	defer rootNS.Close()

	currentPath := fmt.Sprintf("/proc/%d/task/%d/ns/net", os.Getpid(), syscall.Gettid())
	currentNS, err := os.Open(currentPath)
	if err != nil {
		return false
	}
	defer currentNS.Close()

	var s1, s2 syscall.Stat_t
	if err := syscall.Fstat(int(rootNS.Fd()), &s1); err != nil {
		return false
	}
	if err := syscall.Fstat(int(currentNS.Fd()), &s2); err != nil {
		return false
	}
	return s1.Dev == s2.Dev && s1.Ino == s2.Ino
}
