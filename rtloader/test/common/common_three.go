// +build three

package testcommon

// #cgo CFLAGS: -I../../include
// #cgo !windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -ldl
// #cgo windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -lstdc++ -static
// #include <datadog_agent_rtloader.h>
//
import "C"

// UsingTwo states whether we're using Two as backend
const UsingTwo bool = false

// GetRtLoader returns a RtLoader instance using Three
func GetRtLoader() *C.rtloader_t {
	var err *C.char = nil
	return C.make3(nil, &err)
}
