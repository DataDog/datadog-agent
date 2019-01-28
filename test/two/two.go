package two

// #cgo CFLAGS: -I../../include
// #cgo LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #include <datadog_agent_six.h>
import "C"

import (
	"fmt"

	common "../common"
)

func get2() *C.six_t {
	six := C.make2()
	C.init(six, nil)
	return six
}

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

func getVersion() string {
	six := get2()
	ret := C.GoString(C.get_py_version(six))
	C.destroy2(six)
	return ret
}

func runString(code string) (string, error) {
	var ret bool
	var err error
	var output []byte
	output, err = common.Capture(func() {
		six := get2()
		ret = C.run_simple_string(six, C.CString(code)) == 0
		C.destroy2(six)
	})

	if err != nil {
		return "", err
	}

	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	return string(output), err
}
