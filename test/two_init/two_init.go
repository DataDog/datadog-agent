package two_init

import "fmt"

// #cgo CFLAGS: -I../../include
// #cgo LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #include <datadog_agent_six.h>
//
import "C"

func init2() error {
	six := C.make2()
	if six == nil {
		return fmt.Errorf("`make2` failed")
	}

	C.init(six, nil)
	if C.is_initialized(six) != 1 {
		return fmt.Errorf("Six not initialized")
	}

	return nil
}
