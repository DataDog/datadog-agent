// +build !linux_bpf,!windows

package ebpf

// IsTracerSupportedByOS returns whether or not the current kernel version supports tracer functionality
// along with some context on why it's not supported
func IsTracerSupportedByOS(exclusionList []string) (bool, string) {
	return verifyOSVersion(0, "", nil)
}

func verifyOSVersion(kernelCode uint32, platform string, exclusionList []string) (bool, string) {
	return false, "unsupported architecture"
}
