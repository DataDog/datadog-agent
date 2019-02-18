package two

// #cgo CFLAGS: -I../../include
// #cgo LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #include <datadog_agent_six.h>
import "C"

import (
	"fmt"

	common "../common"
)

var six *C.six_t

func setUp() error {
	six = C.make2()
	if six == nil {
		return fmt.Errorf("`make2` failed")
	}

	C.init(six, nil)

	return nil
}

func tearDown() {
	C.destroy(six)
	six = nil
}

func getVersion() string {
	ret := C.GoString(C.get_py_version(six))
	return ret
}

func runString(code string) (string, error) {
	var ret bool
	var err error
	var output []byte
	output, err = common.Capture(func() {
		ret = C.run_simple_string(six, C.CString(code)) == 0
	})

	if err != nil {
		return "", err
	}

	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	return string(output), err
}

func getError() string {
	// following is supposed to raise an error
	C.get_check(six, C.CString("fake_check"), C.CString(""), C.CString("[{fake_check: \"/\"}]"))
	return C.GoString(C.get_error(six))
}

func hasError() bool {
	// following is supposed to raise an error
	C.get_check(six, C.CString("fake_check"), C.CString(""), C.CString("[{fake_check: \"/\"}]"))
	return C.has_error(six) == 1
}
