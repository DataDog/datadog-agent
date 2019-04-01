// +build two

package testcommon

// #cgo CFLAGS: -I../../include
// #cgo !windows LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #cgo windows LDFLAGS: -L../../six/ -ldatadog-agent-six -lstdc++ -static
// #include <datadog_agent_six.h>
//
import "C"

// UsingTwo states whether we're using Two as backend
const UsingTwo bool = true

// GetSix returns a Six instance using Two
func GetSix() *C.six_t {
	return C.make2(nil)
}
