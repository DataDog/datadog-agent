package testsix

/*
#cgo CFLAGS: -I../../include
#cgo !windows LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl -lstdc++
#cgo windows LDFLAGS: -L../../six/ -ldatadog-agent-six -lstdc++ -static
#include <datadog_agent_six.h>
*/
import "C"

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	common "github.com/DataDog/datadog-agent/six/test/common"
	yaml "gopkg.in/yaml.v2"
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

	return nil
}

func tearDown() {
	os.Remove(tmpfile.Name())
}

func getPyInfo() (string, string) {
	runtime.LockOSThread()
	state := C.ensure_gil(six)

	info := C.get_py_info(six)

	C.release_gil(six, state)
	runtime.UnlockOSThread()

	return C.GoString(info.version), C.GoString(info.path)
}

func runString(code string) (string, error) {
	tmpfile.Truncate(0)

	runtime.LockOSThread()
	state := C.ensure_gil(six)

	ret := C.run_simple_string(six, C.CString(code)) == 1

	C.release_gil(six, state)
	runtime.UnlockOSThread()

	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	output, err := ioutil.ReadFile(tmpfile.Name())
	return string(output), err
}

func fetchError() error {
	if C.has_error(six) == 1 {
		return fmt.Errorf(C.GoString(C.get_error(six)))
	}
	return nil
}

func getError() string {
	runtime.LockOSThread()
	state := C.ensure_gil(six)

	// following is supposed to raise an error
	C.get_class(six, C.CString("foo"), nil, nil)

	C.release_gil(six, state)
	runtime.UnlockOSThread()

	return C.GoString(C.get_error(six))
}

func hasError() bool {
	runtime.LockOSThread()
	state := C.ensure_gil(six)

	// following is supposed to raise an error
	C.get_class(six, C.CString("foo"), nil, nil)

	C.release_gil(six, state)
	runtime.UnlockOSThread()

	ret := C.has_error(six) == 1
	C.clear_error(six)
	return ret
}

func getFakeCheck() (string, error) {
	var module *C.six_pyobject_t
	var class *C.six_pyobject_t
	var check *C.six_pyobject_t
	var version *C.char

	runtime.LockOSThread()
	state := C.ensure_gil(six)

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
	ret = C.get_check(six, class, C.CString(""), C.CString("{\"fake_check\": \"/\"}"), C.CString("checkID"), C.CString("fake_check"), &check)
	if ret != 1 || check == nil {
		return "", fmt.Errorf(C.GoString(C.get_error(six)))
	}

	C.release_gil(six, state)
	runtime.UnlockOSThread()

	return C.GoString(version), fetchError()
}

func runFakeCheck() (string, error) {
	var module *C.six_pyobject_t
	var class *C.six_pyobject_t
	var check *C.six_pyobject_t
	var version *C.char

	runtime.LockOSThread()
	state := C.ensure_gil(six)

	C.get_class(six, C.CString("fake_check"), &module, &class)
	C.get_attr_string(six, module, C.CString("__version__"), &version)
	C.get_check(six, class, C.CString(""), C.CString("{\"fake_check\": \"/\"}"), C.CString("checkID"), C.CString("fake_check"), &check)

	out, err := C.GoString(C.run_check(six, check)), fetchError()

	C.release_gil(six, state)
	runtime.UnlockOSThread()

	return out, err

}

func runFakeGetWarnings() ([]string, error) {
	var module *C.six_pyobject_t
	var class *C.six_pyobject_t
	var check *C.six_pyobject_t

	runtime.LockOSThread()
	state := C.ensure_gil(six)

	C.get_class(six, C.CString("fake_check"), &module, &class)
	C.get_check(six, class, C.CString(""), C.CString("{\"fake_check\": \"/\"}"), C.CString("checkID"), C.CString("fake_check"), &check)

	warns := C.get_checks_warnings(six, check)

	C.release_gil(six, state)
	runtime.UnlockOSThread()

	if warns == nil {
		return nil, fmt.Errorf("get_checks_warnings return NULL: %s", C.GoString(C.get_error(six)))
	}

	pWarns := uintptr(unsafe.Pointer(warns))
	ptrSize := unsafe.Sizeof(warns)

	warnings := []string{}
	for i := uintptr(0); ; i++ {
		warnPtr := *(**C.char)(unsafe.Pointer(pWarns + ptrSize*i))
		if warnPtr == nil {
			break
		}
		warn := C.GoString(warnPtr)
		warnings = append(warnings, warn)
	}

	return warnings, nil
}

func getIntegrationList() ([]string, error) {
	runtime.LockOSThread()
	state := C.ensure_gil(six)

	cstr := C.GoString(C.get_integration_list(six))

	C.release_gil(six, state)
	runtime.UnlockOSThread()

	var out []string
	err := yaml.Unmarshal([]byte(cstr), &out)
	fmt.Println(cstr)
	fmt.Println(out)
	if err != nil {
		return nil, err
	}

	return out, nil
}

func setModuleAttrString(module string, attr string, value string) {
	runtime.LockOSThread()
	state := C.ensure_gil(six)

	C.set_module_attr_string(six, C.CString(module), C.CString(attr), C.CString(value))

	C.release_gil(six, state)
	runtime.UnlockOSThread()
}
