// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testrtloader

/*
#include "rtloader_mem.h"
#include "datadog_agent_rtloader.h"

static inline void call_free(void* ptr) {
    _free(ptr);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	yaml "gopkg.in/yaml.v2"

	common "github.com/DataDog/datadog-agent/rtloader/test/common"
	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
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
		return errors.New("make failed")
	}

	var err error
	tmpfile, err = os.CreateTemp("", "testout")
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

	codeStr := helpers.TrackedCString(code)
	defer C.call_free(codeStr)

	ret := C.run_simple_string(rtloader, (*C.char)(codeStr)) == 1

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	if !ret {
		return "", errors.New("`run_simple_string` errored")
	}

	output, err := os.ReadFile(tmpfile.Name())
	return string(output), err
}

func fetchError() error {
	if C.has_error(rtloader) == 1 {
		return errors.New(C.GoString(C.get_error(rtloader)))
	}
	return nil
}

func getError() string {
	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	// following is supposed to raise an error
	classStr := helpers.TrackedCString("foo")
	defer C.call_free(classStr)

	C.get_class(rtloader, (*C.char)(classStr), nil, nil)

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	return C.GoString(C.get_error(rtloader))
}

func hasError() bool {
	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	// following is supposed to raise an error
	classStr := helpers.TrackedCString("foo")
	defer C.call_free(classStr)

	C.get_class(rtloader, (*C.char)(classStr), nil, nil)

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
	classStr := helpers.TrackedCString("fake_check")
	defer C.call_free(classStr)

	ret := C.get_class(rtloader, (*C.char)(classStr), &module, &class)
	if ret != 1 || module == nil || class == nil {
		return "", errors.New(C.GoString(C.get_error(rtloader)))
	}

	// version
	verStr := helpers.TrackedCString("__version__")
	defer C.call_free(verStr)

	ret = C.get_attr_string(rtloader, module, (*C.char)(verStr), &version)
	if ret != 1 || version == nil {
		return "", errors.New(C.GoString(C.get_error(rtloader)))
	}
	defer C.call_free(unsafe.Pointer(version))

	// check instance
	emptyStr := helpers.TrackedCString("")
	defer C.call_free(emptyStr)
	checkIDStr := helpers.TrackedCString("checkID")
	defer C.call_free(checkIDStr)
	configStr := helpers.TrackedCString("{\"fake_check\": \"/\"}")
	defer C.call_free(configStr)
	classStr = helpers.TrackedCString("fake_check")
	defer C.call_free(classStr)

	ret = C.get_check(rtloader, class, (*C.char)(emptyStr), (*C.char)(configStr), (*C.char)(checkIDStr), (*C.char)(classStr), &check)
	if ret != 1 || check == nil {
		return "", errors.New(C.GoString(C.get_error(rtloader)))
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

	classStr := helpers.TrackedCString("fake_check")
	defer C.call_free(classStr)
	C.get_class(rtloader, (*C.char)(classStr), &module, &class)

	verStr := helpers.TrackedCString("__version__")
	defer C.call_free(verStr)

	C.get_attr_string(rtloader, module, (*C.char)(verStr), &version)
	defer C.call_free(unsafe.Pointer(version))

	emptyStr := helpers.TrackedCString("")
	defer C.call_free(emptyStr)
	checkIDStr := helpers.TrackedCString("checkID")
	defer C.call_free(checkIDStr)
	configStr := helpers.TrackedCString("{\"fake_check\": \"/\"}")
	defer C.call_free(configStr)
	classStr = helpers.TrackedCString("fake_check")
	defer C.call_free(classStr)

	C.get_check(rtloader, class, (*C.char)(emptyStr), (*C.char)(configStr), (*C.char)(checkIDStr), (*C.char)(classStr), &check)

	checkResultStr := C.run_check(rtloader, check)
	defer C.call_free(unsafe.Pointer(checkResultStr))
	out, err := C.GoString(checkResultStr), fetchError()

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	return out, err
}

func cancelFakeCheck() error {
	var module *C.rtloader_pyobject_t
	var class *C.rtloader_pyobject_t
	var check *C.rtloader_pyobject_t

	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	classStr := helpers.TrackedCString("fake_check")
	defer C.call_free(classStr)
	C.get_class(rtloader, (*C.char)(classStr), &module, &class)

	emptyStr := helpers.TrackedCString("")
	defer C.call_free(emptyStr)
	checkIDStr := helpers.TrackedCString("checkID")
	defer C.call_free(checkIDStr)
	configStr := helpers.TrackedCString("{\"fake_check\": \"/\"}")
	defer C.call_free(configStr)
	classStr = helpers.TrackedCString("fake_check")
	defer C.call_free(classStr)

	C.get_check(rtloader, class, (*C.char)(emptyStr), (*C.char)(configStr), (*C.char)(checkIDStr), (*C.char)(classStr), &check)

	C.cancel_check(rtloader, check)

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	return fetchError()
}

func runFakeGetWarnings() ([]string, error) {
	var module *C.rtloader_pyobject_t
	var class *C.rtloader_pyobject_t
	var check *C.rtloader_pyobject_t

	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	classStr := helpers.TrackedCString("fake_check")
	defer C.call_free(classStr)

	C.get_class(rtloader, (*C.char)(classStr), &module, &class)

	emptyStr := helpers.TrackedCString("")
	defer C.call_free(emptyStr)
	checkIDStr := helpers.TrackedCString("checkID")
	defer C.call_free(checkIDStr)
	configStr := helpers.TrackedCString("{\"fake_check\": \"/\"}")
	defer C.call_free(configStr)
	classStr = helpers.TrackedCString("fake_check")
	defer C.call_free(classStr)

	C.get_check(rtloader, class, (*C.char)(emptyStr), (*C.char)(configStr), (*C.char)(checkIDStr), (*C.char)(classStr), &check)

	warns := C.get_checks_warnings(rtloader, check)

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	if warns == nil {
		return nil, fmt.Errorf("get_checks_warnings return NULL: %s", C.GoString(C.get_error(rtloader)))
	}

	pWarns := uintptr(unsafe.Pointer(warns))
	defer C.call_free(unsafe.Pointer(warns))
	ptrSize := unsafe.Sizeof(warns)

	warnings := []string{}
	for i := uintptr(0); ; i++ {
		warnPtr := *(**C.char)(unsafe.Pointer(pWarns + ptrSize*i))
		if warnPtr == nil {
			break
		}
		defer C.call_free(unsafe.Pointer(warnPtr))

		warn := C.GoString(warnPtr)
		warnings = append(warnings, warn)
	}

	return warnings, nil
}

func getIntegrationList() ([]string, error) {
	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	integrationStr := C.get_integration_list(rtloader)
	defer C.call_free(unsafe.Pointer(integrationStr))

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

	moduleStr := helpers.TrackedCString(module)
	defer C.call_free(moduleStr)
	attrStr := helpers.TrackedCString(attr)
	defer C.call_free(attrStr)
	valueStr := helpers.TrackedCString(value)
	defer C.call_free(valueStr)

	C.set_module_attr_string(rtloader, (*C.char)(moduleStr), (*C.char)(attrStr), (*C.char)(valueStr))

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()
}

func getFakeModuleWithBool() (bool, error) {
	var module *C.rtloader_pyobject_t
	var class *C.rtloader_pyobject_t
	var value C.bool

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	state := C.ensure_gil(rtloader)
	defer C.release_gil(rtloader, state)

	// class
	moduleStr := helpers.TrackedCString("fake_check")
	defer C.call_free(moduleStr)

	// attribute
	attributeStr := helpers.TrackedCString("foo")
	defer C.call_free(attributeStr)

	ret := C.get_class(rtloader, (*C.char)(moduleStr), &module, &class)
	if ret != 1 || module == nil || class == nil {
		return false, errors.New(C.GoString(C.get_error(rtloader)))
	}

	ret = C.get_attr_bool(rtloader, module, (*C.char)(attributeStr), &value)
	if ret != 1 {
		return false, errors.New(C.GoString(C.get_error(rtloader)))
	}

	return value == C.bool(true), nil
}
