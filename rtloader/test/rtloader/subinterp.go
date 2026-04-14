// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

package testrtloader

/*
#include "rtloader_mem.h"
#include "datadog_agent_rtloader.h"

static inline void si_call_free(void* ptr) {
    _free(ptr);
}
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
)

// hasSubinterpreterSupport checks whether rtloader was compiled with
// -DENABLE_SUBINTERPRETERS=ON. Returns true if RTLOADER_HAS_SUBINTERPRETERS
// is defined, false otherwise.
func hasSubinterpreterSupport() bool {
	return C.has_subinterpreter_support(rtloader) == 1
}

// runTwoIsolationChecks creates two instances of isolation_check and runs each
// once. Returns both run results as strings.
//
// isolation_check has a module-level global `run_count` that starts at 0 and
// increments on each run(). If sub-interpreters are working, each check runs
// in its own interpreter with its own copy of the module, so both return "1".
// Without sub-interpreters, they share the module, so the second returns "2".
func runTwoIsolationChecks() (string, string, error) {
	var module1 *C.rtloader_pyobject_t
	var class1 *C.rtloader_pyobject_t
	var check1 *C.rtloader_pyobject_t
	var module2 *C.rtloader_pyobject_t
	var class2 *C.rtloader_pyobject_t
	var check2 *C.rtloader_pyobject_t

	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	// Create first check instance
	classStr1 := helpers.TrackedCString("isolation_check")
	defer C.si_call_free(classStr1)
	ret := C.get_class(rtloader, (*C.char)(classStr1), &module1, &class1)
	if ret != 1 {
		C.release_gil(rtloader, state)
		runtime.UnlockOSThread()
		return "", "", fmt.Errorf("get_class failed for check 1: %s", C.GoString(C.get_error(rtloader)))
	}

	emptyStr1 := helpers.TrackedCString("")
	defer C.si_call_free(emptyStr1)
	checkID1 := helpers.TrackedCString("isolation:check1")
	defer C.si_call_free(checkID1)
	configStr1 := helpers.TrackedCString("{}")
	defer C.si_call_free(configStr1)
	nameStr1 := helpers.TrackedCString("isolation_check")
	defer C.si_call_free(nameStr1)
	providerStr1 := helpers.TrackedCString("test")
	defer C.si_call_free(providerStr1)

	ret = C.get_check(rtloader, class1, (*C.char)(emptyStr1), (*C.char)(configStr1), (*C.char)(checkID1), (*C.char)(nameStr1), (*C.char)(providerStr1), &check1)
	if ret != 1 {
		C.release_gil(rtloader, state)
		runtime.UnlockOSThread()
		return "", "", fmt.Errorf("get_check failed for check 1: %s", C.GoString(C.get_error(rtloader)))
	}

	// Create second check instance
	classStr2 := helpers.TrackedCString("isolation_check")
	defer C.si_call_free(classStr2)
	ret = C.get_class(rtloader, (*C.char)(classStr2), &module2, &class2)
	if ret != 1 {
		C.release_gil(rtloader, state)
		runtime.UnlockOSThread()
		return "", "", fmt.Errorf("get_class failed for check 2: %s", C.GoString(C.get_error(rtloader)))
	}

	emptyStr2 := helpers.TrackedCString("")
	defer C.si_call_free(emptyStr2)
	checkID2 := helpers.TrackedCString("isolation:check2")
	defer C.si_call_free(checkID2)
	configStr2 := helpers.TrackedCString("{}")
	defer C.si_call_free(configStr2)
	nameStr2 := helpers.TrackedCString("isolation_check")
	defer C.si_call_free(nameStr2)
	providerStr2 := helpers.TrackedCString("test")
	defer C.si_call_free(providerStr2)

	ret = C.get_check(rtloader, class2, (*C.char)(emptyStr2), (*C.char)(configStr2), (*C.char)(checkID2), (*C.char)(nameStr2), (*C.char)(providerStr2), &check2)
	if ret != 1 {
		C.release_gil(rtloader, state)
		runtime.UnlockOSThread()
		return "", "", fmt.Errorf("get_check failed for check 2: %s", C.GoString(C.get_error(rtloader)))
	}

	// Run check 1
	result1Str := C.run_check(rtloader, check1)
	defer C.si_call_free(unsafe.Pointer(result1Str))
	res1 := C.GoString(result1Str)

	// Run check 2
	result2Str := C.run_check(rtloader, check2)
	defer C.si_call_free(unsafe.Pointer(result2Str))
	res2 := C.GoString(result2Str)

	// Clean up: decref check instances, modules, and classes.
	// For sub-interpreter checks, decref also destroys the sub-interpreter.
	C.rtloader_decref(rtloader, check1)
	C.rtloader_decref(rtloader, check2)
	C.rtloader_decref(rtloader, module1)
	C.rtloader_decref(rtloader, module2)
	C.rtloader_decref(rtloader, class1)
	C.rtloader_decref(rtloader, class2)

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	return res1, res2, nil
}
