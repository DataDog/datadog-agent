// +build windows

package ebpf

func verifyOSVersion(kernelCode uint32, platform string, exclusionList []string) (bool, string) {
	return true, ""
}
