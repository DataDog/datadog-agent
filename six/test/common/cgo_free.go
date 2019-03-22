package testcommon

import (
	"fmt"
	"unsafe"
)

/*
#cgo CFLAGS: -I../../include
#cgo !windows LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
#cgo windows LDFLAGS: -L../../six/ -ldatadog-agent-six -lstdc++ -static
#include <datadog_agent_six.h>

extern void c_callCgoFree(void *ptr);
extern void cgoFree(void *);

static void initCgoFreeTests(six_t *six) {
	set_cgo_free_cb(six, cgoFree);
}
*/
import "C"

var (
	six           *C.six_t
	cgoFreeCalled bool
	latestFreePtr unsafe.Pointer
)

func setUp() error {
	six = GetSix()
	if six == nil {
		return fmt.Errorf("make failed")
	}

	C.initCgoFreeTests(six)

	// Updates sys.path so testing Check can be found
	C.add_python_path(six, C.CString("../python"))

	if ok := C.init(six); ok != 1 {
		return fmt.Errorf("`init` failed: %s", C.GoString(C.get_error(six)))
	}

	C.ensure_gil(six)
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
