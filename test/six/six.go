package testsix

// #cgo CFLAGS: -I../../include
// #cgo !windows LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl -lstdc++
// #cgo windows LDFLAGS: -L../../six/ -ldatadog-agent-six -lstdc++ -static
// #include <datadog_agent_six.h>
//
import "C"

import (
	"fmt"
	"io/ioutil"
	"os"
)

var (
	six     *C.six_t
	tmpfile *os.File
)

func setUp() error {
	if _, ok := os.LookupEnv("TESTING_TWO"); ok {
		six = C.make2()
		if six == nil {
			return fmt.Errorf("`make2` failed")
		}
	} else {
		six = C.make3()
		if six == nil {
			return fmt.Errorf("`make3` failed")
		}
	}

	var err error
	tmpfile, err = ioutil.TempFile("", "testout")
	if err != nil {
		return err
	}

	// Updates sys.path so testing Check can be found
	C.add_python_path(six, C.CString("../python"))

	ok := C.init(six, nil)
	if ok != 1 {
		return fmt.Errorf("`init` failed: %s", C.GoString(C.get_error(six)))
	}
	C.ensure_gil(six)
	return nil
}

func tearDown() {
	os.Remove(tmpfile.Name())
}

func getVersion() string {
	ret := C.GoString(C.get_py_version(six))
	return ret
}

func runString(code string) (string, error) {
	tmpfile.Truncate(0)
	ret := C.run_simple_string(six, C.CString(code)) == 1
	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	output, err := ioutil.ReadFile(tmpfile.Name())
	return string(output), err
}

func getError() string {
	// following is supposed to raise an error
	C.get_class(six, C.CString("foo"), nil, nil)
	return C.GoString(C.get_error(six))
}

func hasError() bool {
	// following is supposed to raise an error
	C.get_class(six, C.CString("foo"), nil, nil)
	ret := C.has_error(six) == 1
	C.clear_error(six)
	return ret
}

func getFakeCheck() (string, error) {
	var module *C.six_pyobject_t
	var class *C.six_pyobject_t
	var check *C.six_pyobject_t
	var version *C.char

	// class
	ret := C.get_class(six, C.CString("fake_check"), &module, &class)
	if ret != 1 || module == nil || class == nil {
		return "", fmt.Errorf(C.GoString(C.get_error(six)))
	}

	// version
	ret = C.get_attr_string(six, module, C.CString("__version__"), &version)
	if ret != 1 || version == nil {
		return "", fmt.Errorf(C.GoString(C.get_error(six)))
	}

	// check instance
	ret = C.get_check(six, class, C.CString(""), C.CString("[{fake_check: \"/\"}]"), C.CString("checkID"), C.CString("fake_check"), &check)
	if ret != 1 || check == nil {
		return "", fmt.Errorf(C.GoString(C.get_error(six)))
	}

	return C.GoString(version), nil
}

func runFakeCheck() (string, error) {
	var module *C.six_pyobject_t
	var class *C.six_pyobject_t
	var check *C.six_pyobject_t
	var version *C.char

	//C.get_class(six, C.CString("datadog_checks.directory"), &module, &class)
	C.get_class(six, C.CString("datadog_checks.fake_check"), &module, &class)
	C.get_attr_string(six, module, C.CString("__version__"), &version)
	C.get_check(six, class, C.CString(""), C.CString("[{fake_check: \"/\"}]"), C.CString("checkID"), C.CString("fake_check"), &check)

	return C.GoString(C.run_check(six, check)), nil
}
