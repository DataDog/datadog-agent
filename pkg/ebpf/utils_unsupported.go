// +build !linux_bpf,!windows

package ebpf

func verifyOSVersion(kernelCode uint32, platform string, exclusionList []string) (bool, string) {
	return false, "unsupported architecture"
}
