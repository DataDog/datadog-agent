// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testcommon

import (
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
)

/*
#include <datadog_agent_rtloader.h>

extern void c_callCgoFree(void *ptr);
extern void cgoFree(void *);

static void initCgoFreeTests(rtloader_t *rtloader) {
	set_cgo_free_cb(rtloader, cgoFree);
}
*/
import "C"

var (
	rtloader      *C.rtloader_t
	cgoFreeCalled bool
	latestFreePtr unsafe.Pointer
)

func setUp() error {
	rtloader = GetRtLoader()
	if rtloader == nil {
		return fmt.Errorf("make failed")
	}

	// Initialize memory tracking
	helpers.InitMemoryTracker()

	C.initCgoFreeTests(rtloader)

	// Updates sys.path so testing Check can be found
	C.add_python_path(rtloader, C.CString("../python"))

	if ok := C.init(rtloader); ok != 1 {
		return fmt.Errorf("`init` failed: %s", C.GoString(C.get_error(rtloader)))
	}

	return nil
}

func callCgoFree(ptr unsafe.Pointer) {
	C.c_callCgoFree(ptr)
}

//export cgoFree
func cgoFree(ptr unsafe.Pointer) {
	cgoFreeCalled = true
	latestFreePtr = ptr
}
