// +build linux

package kernel

import (
	"fmt"
	"os"
)

// IsDebugFSMounted would test the existence of file /sys/kernel/debug/tracing/kprobe_events to determine if debugfs is mounted or not
// returns a boolean and a possible error message
func IsDebugFSMounted() (bool, error) {
	_, err := os.Stat("/sys/kernel/debug/tracing/kprobe_events")

	if err != nil {
		if os.IsPermission(err) {
			return false, fmt.Errorf("eBPF not supported, does not have permission to access debugfs")
		} else if os.IsNotExist(err) {
			return false, fmt.Errorf("debugfs is not mounted and is needed for eBPF-based checks, run \"sudo mount -t debugfs none /sys/kernel/debug\" to mount debugfs")
		} else {
			return false, fmt.Errorf("an error occurred while accessing debugfs: %w", err)
		}
	}
	return true, nil
}
