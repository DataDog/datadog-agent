package threeinit

import "fmt"

// #cgo CFLAGS: -I../../include
// #cgo LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #include <datadog_agent_six.h>
//
import "C"

func init3() error {
	six := C.make3()
	if six == nil {
		return fmt.Errorf("`make3` failed")
	}

	ok := C.init(six, nil)
	if ok != 1 {
		return fmt.Errorf("`init` failed")
	}

	if C.is_initialized(six) != 1 {
		return fmt.Errorf("Six not initialized")
	}

	return nil
}
