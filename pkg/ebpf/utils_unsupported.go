// +build !linux_bpf,!windows

package ebpf

// VerifyOSVersion validates whether the OS version is compatbile with eBPF
func VerifyOSVersion(kernelCode uint32, platform string, exclusionList []string) (bool, string) {
	return false, "unsupported architecture"
}

// CurrentKernelVersion is not implemented on this OS for Tracer
func CurrentKernelVersion() (uint32, error) {
	return 0, ErrNotImplemented
}
