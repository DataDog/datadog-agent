package main

/*
#include "libdd_rc.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func main() {
	appName := []byte("datadog-agent")
	version := []byte("bazel-windows-msvc-smoke")
	ctx := C.rc_init(
		(*C.uint8_t)(unsafe.Pointer(&appName[0])),
		C.uint32_t(len(appName)),
		(*C.uint8_t)(unsafe.Pointer(&version[0])),
		C.uint32_t(len(version)),
	)
	if ctx == nil {
		panic("rc_init returned a nil context")
	}
	C.rc_free(ctx)
	fmt.Println("MinGW cgo host completed the MSVC rc-x509 FFI lifecycle")
}
