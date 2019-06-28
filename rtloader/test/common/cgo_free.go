package testcommon

import (
	"fmt"
	"unsafe"
)

/*
#cgo CFLAGS: -I../../include
#cgo !windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -lstdc++ -static
#include <datadog_agent_rtloader.h>

extern void c_callCgoFree(void *ptr);
extern void cgoFree(void *);

static void initCgoFreeTests(rtloader_t *rtloader) {
	set_cgo_free_cb(rtloader, cgoFree);
}
*/
import "C"

var (
	rtloader           *C.rtloader_t
	cgoFreeCalled bool
	latestFreePtr unsafe.Pointer
)

func setUp() error {
	rtloader = GetRtLoader()
	if rtloader == nil {
		return fmt.Errorf("make failed")
	}

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
