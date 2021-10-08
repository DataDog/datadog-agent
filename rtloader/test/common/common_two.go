// +build two

package testcommon

// #include <datadog_agent_rtloader.h>
//
import "C"

import "unsafe"

// UsingTwo states whether we're using Two as backend
const UsingTwo bool = true

// GetRtLoader returns a RtLoader instance using Two
func GetRtLoader() *C.rtloader_t {
	var err *C.char = nil

	executablePath := C.CString("/folder/mock_python_interpeter_bin_path")
	defer C.free(unsafe.Pointer(executablePath))

	return C.make2(nil, executablePath, &err)
}
