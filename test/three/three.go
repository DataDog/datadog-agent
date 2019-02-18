package three

// #cgo CFLAGS: -I../../include
// #cgo LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #include <datadog_agent_six.h>
//
import "C"

import (
	"fmt"

	common "../common"
)

var six *C.six_t

func setUp() error {
	six = C.make3()
	if six == nil {
		return fmt.Errorf("`make3` failed")
	}

	C.init(six, nil)

	// Updates sys.path so Agent for testing can be found
	code := C.CString("import sys; sys.path.insert(0, '../python/')")
	success := C.run_simple_string(six, code)
	if success != 0 {
		return fmt.Errorf("setUp test failed")
	}

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
	C.get_check(six, C.CString("foo"), C.CString(""), C.CString("[{foo: \"/\"}]"))
	return C.GoString(C.get_error(six))
}

func hasError() bool {
	// following is supposed to raise an error
	C.get_check(six, C.CString("foo"), C.CString(""), C.CString("[{foo: \"/\"}]"))
	return C.has_error(six) == 1
}

func getFakeCheck() error {
	ret := C.get_check(six, C.CString("fake_agent"), C.CString(""), C.CString("[{fake_agent: \"/\"}]"))

	if ret == nil {
		return fmt.Errorf(C.GoString(C.get_error(six)))
	}
	return nil
}

func runFakeCheck() (string, error) {
	check := C.get_check(six, C.CString("fake_agent"), C.CString(""), C.CString("[{fake_agent: \"/\"}]"))
	if check == nil {
		return "", fmt.Errorf(C.GoString(C.get_error(six)))
	}

	return C.GoString(C.run_check(six, check)), nil
}

func getCheckClass(moduleName string) error {
	ret := C.get_check_class(six, C.CString(moduleName))
	if ret == nil {
		return fmt.Errorf(C.GoString(C.get_error(six)))
	}
	return nil
}
