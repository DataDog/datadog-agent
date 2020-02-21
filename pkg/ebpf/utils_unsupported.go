// +build !linux_bpf,!windows,!linux

package ebpf

// TODO Determine which windows versions we will support, and potentially remove kernelCode from parameters list
func verifyOSVersion(kernelCode uint32, platform string, exclusionList []string) (bool, string) {
	return true, ""
}

// CurrentKernelVersion is not implemented on non-linux systems
func CurrentKernelVersion() (uint32, error) {
	return 0, ErrNotImplemented
}
