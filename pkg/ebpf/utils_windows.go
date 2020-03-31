// +build windows

package ebpf

// TODO Determine which windows versions we will support, and potentially remove kernelCode from parameters list
func VerifyOSVersion(kernelCode uint32, platform string, exclusionList []string) (bool, string) {
	return true, ""
}

// CurrentKernelVersion is not implemented on this OS for Tracer
func CurrentKernelVersion() (uint32, error) {
	return 0, ErrNotImplemented
}
