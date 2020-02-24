// +build windows

package ebpf

// TODO Determine which windows versions we will support, and potentially remove kernelCode from parameters list
func verifyOSVersion(kernelCode uint32, platform string, exclusionList []string) (bool, string) {
	return true, ""
}

func CurrentKernelVersion() (uint32, error) {
	return 1, nil
}
