// +build windows

package ebpf

// TODO Determine which windows versions we will support, and potentially remove kernelCode from parameters list
func verifyOSVersion(kernelCode uint32, platform string, exclusionList []string) (bool, string) {
	return true, ""
}

// TODO determine if we will keep this function. Also, add an error that will not cause failure to set up system probe
// CurrentKernelVersion returns the current kernel version of the OS
func CurrentKernelVersion() (uint32, error) {
	return 1, nil
}
