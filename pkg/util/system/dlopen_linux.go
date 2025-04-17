// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && cgo && !static

package system

// #cgo LDFLAGS: -ldl
// #include <stdlib.h>
// #include <dlfcn.h>
import "C"

import (
	"fmt"
	"unsafe"
)

// CheckLibraryExists checks if a library is available on the system by trying it to
// open with dlopen. It returns an error if the library is not found. This is
// the most direct way to check for a library's presence on Linux, as there are
// multiple sources for paths for library searches, so it's better to use the
// same mechanism that the loader uses.
func CheckLibraryExists(libname string) error {
	cname := C.CString(libname)
	defer C.free(unsafe.Pointer(cname))

	// Lazy: resolve undefined symbols as they are needed, avoid loading everything at once
	handle := C.dlopen(cname, C.RTLD_LAZY)
	if handle == nil {
		e := C.dlerror()
		var errstr string
		if e != nil {
			errstr = C.GoString(e)
		}

		return fmt.Errorf("could not locate %s: %s", libname, errstr)
	}

	defer C.dlclose(handle)
	return nil
}
