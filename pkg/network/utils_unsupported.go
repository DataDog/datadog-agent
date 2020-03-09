// +build !linux_bpf,!windows

package network

func verifyOSVersion(kernelCode uint32, platform string, exclusionList []string) (bool, string) {
	return false, "unsupported architecture"
}
