// +build two

package testcommon

// #include <datadog_agent_rtloader.h>
//
import "C"

// UsingTwo states whether we're using Two as backend
const UsingTwo bool = true

// GetRtLoader returns a RtLoader instance using Two
func GetRtLoader() *C.rtloader_t {
	var err *C.char = nil
	return C.make2(nil, &err)
}
