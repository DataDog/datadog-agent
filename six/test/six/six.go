package testsix

/*
#cgo CFLAGS: -I../../include
#cgo !windows LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl -lstdc++
#cgo windows LDFLAGS: -L../../six/ -ldatadog-agent-six -lstdc++ -static
#include <datadog_agent_six.h>
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	common "github.com/DataDog/datadog-agent/six/test/common"
)

var (
	six     *C.six_t
	tmpfile *os.File
)

func setUp() error {
	six = (*C.six_t)(common.GetSix())
	if six == nil {
		return fmt.Errorf("make failed")
	}

	var err error
	tmpfile, err = ioutil.TempFile("", "testout")
	if err != nil {
		return err
	}

	// Updates sys.path so testing Check can be found
	C.add_python_path(six, C.CString(filepath.Join("..", "python")))

	ok := C.init(six)
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

	C.get_class(six, C.CString("datadog_checks.fake_check"), &module, &class)
	C.get_attr_string(six, module, C.CString("__version__"), &version)
	C.get_check(six, class, C.CString(""), C.CString("[{fake_check: \"/\"}]"), C.CString("checkID"), C.CString("fake_check"), &check)

	return C.GoString(C.run_check(six, check)), nil
}

func getIntegrationList() ([]string, error) {
	cstr := C.GoString(C.get_integration_list(six))
	var out []string
	err := json.Unmarshal([]byte(cstr), &out)
	fmt.Println(cstr)
	fmt.Println(out)
	if err != nil {
		return nil, err
	}

	return out, nil
}

func setModuleAttrString(module string, attr string, value string) {
	C.set_module_attr_string(six, C.CString(module), C.CString(attr), C.CString(value))
}
