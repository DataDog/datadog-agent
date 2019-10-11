package testrtloader

/*
#cgo CFLAGS: -I../../include -I../../common
#cgo !windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -ldl -lstdc++
#cgo windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -lstdc++ -static

#include "rtloader_mem.h"
#include "datadog_agent_rtloader.h"
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
	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
	yaml "gopkg.in/yaml.v2"
)

var (
	rtloader *C.rtloader_t
	tmpfile  *os.File
)

func setUp() error {
	// Initialize memory tracking
	helpers.InitMemoryTracker()

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
	defer C.free_py_info(rtloader, info)

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	return C.GoString(info.version), C.GoString(info.path)
}

func runString(code string) (string, error) {
	tmpfile.Truncate(0)

	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	codeStr := (*C.char)(helpers.TrackedCString(code))
	defer C._free(unsafe.Pointer(codeStr))

	ret := C.run_simple_string(rtloader, codeStr) == 1

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
	classStr := (*C.char)(helpers.TrackedCString("foo"))
	defer C._free(unsafe.Pointer(classStr))

	C.get_class(rtloader, classStr, nil, nil)

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	return C.GoString(C.get_error(rtloader))
}

func hasError() bool {
	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	// following is supposed to raise an error
	classStr := (*C.char)(helpers.TrackedCString("foo"))
	defer C._free(unsafe.Pointer(classStr))

	C.get_class(rtloader, classStr, nil, nil)

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
	classStr := (*C.char)(helpers.TrackedCString("fake_check"))
	defer C._free(unsafe.Pointer(classStr))

	ret := C.get_class(rtloader, classStr, &module, &class)
	if ret != 1 || module == nil || class == nil {
		return "", fmt.Errorf(C.GoString(C.get_error(rtloader)))
	}

	// version
	verStr := (*C.char)(helpers.TrackedCString("__version__"))
	defer C._free(unsafe.Pointer(verStr))

	ret = C.get_attr_string(rtloader, module, verStr, &version)
	if ret != 1 || version == nil {
		return "", fmt.Errorf(C.GoString(C.get_error(rtloader)))
	}
	defer C._free(unsafe.Pointer(version))

	// check instance
	emptyStr := (*C.char)(helpers.TrackedCString(""))
	defer C._free(unsafe.Pointer(emptyStr))
	checkIdStr := (*C.char)(helpers.TrackedCString("checkID"))
	defer C._free(unsafe.Pointer(checkIdStr))
	configStr := (*C.char)(helpers.TrackedCString("{\"fake_check\": \"/\"}"))
	defer C._free(unsafe.Pointer(configStr))
	classStr = (*C.char)(helpers.TrackedCString("fake_check"))
	defer C._free(unsafe.Pointer(classStr))

	ret = C.get_check(rtloader, class, emptyStr, configStr, checkIdStr, classStr, &check)
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

	classStr := (*C.char)(helpers.TrackedCString("fake_check"))
	defer C._free(unsafe.Pointer(classStr))
	C.get_class(rtloader, classStr, &module, &class)

	verStr := (*C.char)(helpers.TrackedCString("__version__"))
	defer C._free(unsafe.Pointer(verStr))

	C.get_attr_string(rtloader, module, verStr, &version)
	defer C._free(unsafe.Pointer(version))

	emptyStr := (*C.char)(helpers.TrackedCString(""))
	defer C._free(unsafe.Pointer(emptyStr))
	checkIdStr := (*C.char)(helpers.TrackedCString("checkID"))
	defer C._free(unsafe.Pointer(checkIdStr))
	configStr := (*C.char)(helpers.TrackedCString("{\"fake_check\": \"/\"}"))
	defer C._free(unsafe.Pointer(configStr))
	classStr = (*C.char)(helpers.TrackedCString("fake_check"))
	defer C._free(unsafe.Pointer(classStr))

	ret := C.is_check_init_deprecated(rtloader, class)
	if ret != 0 {
		return "", fmt.Errorf(C.GoString(C.get_error(rtloader)))
	}
	C.get_check(rtloader, class, emptyStr, configStr, checkIdStr, classStr, &check)

	checkResultStr := C.run_check(rtloader, check)
	defer C._free(unsafe.Pointer(checkResultStr))
	out, err := C.GoString(checkResultStr), fetchError()

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

	classStr := (*C.char)(helpers.TrackedCString("fake_check"))
	defer C._free(unsafe.Pointer(classStr))

	C.get_class(rtloader, classStr, &module, &class)

	emptyStr := (*C.char)(helpers.TrackedCString(""))
	defer C._free(unsafe.Pointer(emptyStr))
	checkIdStr := (*C.char)(helpers.TrackedCString("checkID"))
	defer C._free(unsafe.Pointer(checkIdStr))
	configStr := (*C.char)(helpers.TrackedCString("{\"fake_check\": \"/\"}"))
	defer C._free(unsafe.Pointer(configStr))
	classStr = (*C.char)(helpers.TrackedCString("fake_check"))
	defer C._free(unsafe.Pointer(classStr))

	C.get_check(rtloader, class, emptyStr, configStr, checkIdStr, classStr, &check)

	warns := C.get_checks_warnings(rtloader, check)

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	if warns == nil {
		return nil, fmt.Errorf("get_checks_warnings return NULL: %s", C.GoString(C.get_error(rtloader)))
	}

	pWarns := uintptr(unsafe.Pointer(warns))
	defer C._free(unsafe.Pointer(pWarns))
	ptrSize := unsafe.Sizeof(warns)

	warnings := []string{}
	for i := uintptr(0); ; i++ {
		warnPtr := *(**C.char)(unsafe.Pointer(pWarns + ptrSize*i))
		if warnPtr == nil {
			break
		}
		defer C._free(unsafe.Pointer(warnPtr))

		warn := C.GoString(warnPtr)
		warnings = append(warnings, warn)
	}

	return warnings, nil
}

func getIntegrationList() ([]string, error) {
	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	integrationStr := C.get_integration_list(rtloader)
	defer C._free(unsafe.Pointer(integrationStr))

	cstr := C.GoString(integrationStr)

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

	moduleStr := (*C.char)(helpers.TrackedCString(module))
	defer C._free(unsafe.Pointer(moduleStr))
	attrStr := (*C.char)(helpers.TrackedCString(attr))
	defer C._free(unsafe.Pointer(attrStr))
	valueStr := (*C.char)(helpers.TrackedCString(value))
	defer C._free(unsafe.Pointer(valueStr))

	C.set_module_attr_string(rtloader, moduleStr, attrStr, valueStr)

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()
}
