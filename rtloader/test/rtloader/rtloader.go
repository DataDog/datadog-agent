package testrtloader

/*
#cgo CFLAGS: -I../../include
#cgo !windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -ldl -lstdc++
#cgo windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -lstdc++ -static
#include <datadog_agent_rtloader.h>
*/
import "C"

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	common "github.com/DataDog/datadog-agent/rtloader/test/common"
	yaml "gopkg.in/yaml.v2"
)

var (
	rtloader     *C.rtloader_t
	tmpfile *os.File
)

func setUp() error {
	rtloader = (*C.rtloader_t)(common.GetRtLoader())
	if rtloader == nil {
		return fmt.Errorf("make failed")
	}

	var err error
	tmpfile, err = ioutil.TempFile("", "testout")
	if err != nil {
		return err
	}

	// Updates sys.path so testing Check can be found
	C.add_python_path(rtloader, C.CString(filepath.Join("..", "python")))

	ok := C.init(rtloader)
	if ok != 1 {
		return fmt.Errorf("`init` failed: %s", C.GoString(C.get_error(rtloader)))
	}

	return nil
}

func tearDown() {
	os.Remove(tmpfile.Name())
}

func getPyInfo() (string, string) {
	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	info := C.get_py_info(rtloader)

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	return C.GoString(info.version), C.GoString(info.path)
}

func runString(code string) (string, error) {
	tmpfile.Truncate(0)

	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	ret := C.run_simple_string(rtloader, C.CString(code)) == 1

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	output, err := ioutil.ReadFile(tmpfile.Name())
	return string(output), err
}

func fetchError() error {
	if C.has_error(rtloader) == 1 {
		return fmt.Errorf(C.GoString(C.get_error(rtloader)))
	}
	return nil
}

func getError() string {
	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	// following is supposed to raise an error
	C.get_class(rtloader, C.CString("foo"), nil, nil)

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	return C.GoString(C.get_error(rtloader))
}

func hasError() bool {
	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	// following is supposed to raise an error
	C.get_class(rtloader, C.CString("foo"), nil, nil)

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	ret := C.has_error(rtloader) == 1
	C.clear_error(rtloader)
	return ret
}

func getFakeCheck() (string, error) {
	var module *C.rtloader_pyobject_t
	var class *C.rtloader_pyobject_t
	var check *C.rtloader_pyobject_t
	var version *C.char

	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	// class
	ret := C.get_class(rtloader, C.CString("fake_check"), &module, &class)
	if ret != 1 || module == nil || class == nil {
		return "", fmt.Errorf(C.GoString(C.get_error(rtloader)))
	}

	// version
	ret = C.get_attr_string(rtloader, module, C.CString("__version__"), &version)
	if ret != 1 || version == nil {
		return "", fmt.Errorf(C.GoString(C.get_error(rtloader)))
	}

	// check instance
	ret = C.get_check(rtloader, class, C.CString(""), C.CString("{\"fake_check\": \"/\"}"), C.CString("checkID"), C.CString("fake_check"), &check)
	if ret != 1 || check == nil {
		return "", fmt.Errorf(C.GoString(C.get_error(rtloader)))
	}

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	return C.GoString(version), fetchError()
}

func runFakeCheck() (string, error) {
	var module *C.rtloader_pyobject_t
	var class *C.rtloader_pyobject_t
	var check *C.rtloader_pyobject_t
	var version *C.char

	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	C.get_class(rtloader, C.CString("fake_check"), &module, &class)
	C.get_attr_string(rtloader, module, C.CString("__version__"), &version)
	C.get_check(rtloader, class, C.CString(""), C.CString("{\"fake_check\": \"/\"}"), C.CString("checkID"), C.CString("fake_check"), &check)

	out, err := C.GoString(C.run_check(rtloader, check)), fetchError()

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	return out, err

}

func runFakeGetWarnings() ([]string, error) {
	var module *C.rtloader_pyobject_t
	var class *C.rtloader_pyobject_t
	var check *C.rtloader_pyobject_t

	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	C.get_class(rtloader, C.CString("fake_check"), &module, &class)
	C.get_check(rtloader, class, C.CString(""), C.CString("{\"fake_check\": \"/\"}"), C.CString("checkID"), C.CString("fake_check"), &check)

	warns := C.get_checks_warnings(rtloader, check)

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	if warns == nil {
		return nil, fmt.Errorf("get_checks_warnings return NULL: %s", C.GoString(C.get_error(rtloader)))
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
	state := C.ensure_gil(rtloader)

	cstr := C.GoString(C.get_integration_list(rtloader))

	C.release_gil(rtloader, state)
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
	state := C.ensure_gil(rtloader)

	C.set_module_attr_string(rtloader, C.CString(module), C.CString(attr), C.CString(value))

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()
}
