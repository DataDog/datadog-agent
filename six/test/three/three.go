package three

// #cgo CFLAGS: -I../../include
// #cgo LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #include <datadog_agent_six.h>
import "C"

import (
	"fmt"

	common "../common"
)

func get3() *C.six_t {
	six := C.make3()
	C.init(six, nil)
	return six
}

func init3() error {
	six := C.make3()
	if six == nil {
		return fmt.Errorf("`make3` failed")
	}

	C.init(six, nil)
	if C.is_initialized(six) != 1 {
		return fmt.Errorf("Six not initialized")
	}

	return nil
}

func getVersion() string {
	six := get3()
	ret := C.GoString(C.get_py_version(six))
	C.destroy3(six)
	return ret
}

func runString(code string) (string, error) {
	var ret bool
	var err error
	var output []byte
	output, err = common.Capture(func() {
		six := get3()
		ret = C.run_simple_string(six, C.CString(code)) == 0
		C.destroy3(six)
	})

	if err != nil {
		return "", err
	}

	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	return string(output), err
}
