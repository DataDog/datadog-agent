package util

import "fmt"

// GetNetNsInoFromPid gets the network namespace inode number for the given
// `pid`
func GetNetNsInoFromPid(procRoot string, pid int) (uint32, error) {
	return 0, fmt.Errorf("not supported")
}
