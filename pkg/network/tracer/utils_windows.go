// +build windows

package tracer

// IsTracerSupportedByOS returns whether or not the current kernel version supports tracer functionality
// along with some context on why it's not supported
func IsTracerSupportedByOS(exclusionList []string) (bool, string) {
	return verifyOSVersion(0, "", nil)
}

// TODO Determine which windows versions we will support, and potentially remove kernelCode from parameters list
func verifyOSVersion(kernelCode uint32, platform string, exclusionList []string) (bool, string) {
	return true, ""
}
