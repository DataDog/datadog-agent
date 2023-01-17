// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testuutil

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"unsafe"

	common "github.com/DataDog/datadog-agent/rtloader/test/common"
	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
)

/*
#include "rtloader_mem.h"
#include "datadog_agent_rtloader.h"

extern void getSubprocessOutput(char **, char **, char **, char **, int*, char **);

static void init_utilTests(rtloader_t *rtloader) {
   set_cgo_free_cb(rtloader, _free);
   set_get_subprocess_output_cb(rtloader, getSubprocessOutput);
}
*/
import "C"

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
	tmpfile, err = os.CreateTemp("", "testout")
	if err != nil {
		return err
	}

	C.init_utilTests(rtloader)

	// Updates sys.path so testing Check can be found
	C.add_python_path(rtloader, C.CString("../python"))

	if ok := C.init(rtloader); ok != 1 {
		return fmt.Errorf("`init` failed: %s", C.GoString(C.get_error(rtloader)))
	}

	return nil
}

func tearDown() {
	os.Remove(tmpfile.Name())
}

func run(call string) (string, error) {
	tmpfile.Truncate(0)
	code := (*C.char)(helpers.TrackedCString(fmt.Sprintf(`
try:
	import _util
	import sys
	%s
except Exception as e:
	with open(r'%s', 'w') as f:
		f.write("{}: {}\n".format(type(e).__name__, e))
`, call, tmpfile.Name())))
	defer C._free(unsafe.Pointer(code))

	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	ret := C.run_simple_string(rtloader, code) == 1

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	resetTest()
	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	output, err := os.ReadFile(tmpfile.Name())

	return strings.TrimSpace(string(output)), err
}

func charArrayToSlice(array **C.char) (res []string) {
	if array == nil {
		return
	}
	pTags := uintptr(unsafe.Pointer(array))
	ptrSize := unsafe.Sizeof(*array)

	for i := uintptr(0); ; i++ {
		tagPtr := *(**C.char)(unsafe.Pointer(pTags + ptrSize*i))
		if tagPtr == nil {
			return
		}
		tag := C.GoString(tagPtr)
		res = append(res, tag)
	}
}

//export getSubprocessOutput
func getSubprocessOutput(cargs **C.char, cenv **C.char, cstdout **C.char, cstderr **C.char, cretCode *C.int, cexception **C.char) {
	args = charArrayToSlice(cargs)
	env = charArrayToSlice(cenv)
	*cstdout = (*C.char)(helpers.TrackedCString(stdout))
	*cstderr = (*C.char)(helpers.TrackedCString(stderr))
	*cretCode = C.int(retCode)
	if setException {
		*cexception = (*C.char)(helpers.TrackedCString(exception))
	}
}
