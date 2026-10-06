// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && cgo && !static

package system

// #cgo LDFLAGS: -ldl
// #include <stdlib.h>
// #include <string.h>
// #include <dlfcn.h>
//
// static int check_library_exists(const char *name, char **error) {
//     *error = NULL;
//
//     void *handle = dlopen(name, RTLD_LAZY);
//     if (handle != NULL) {
//         dlclose(handle);
//         return 1;
//     }
//
//     const char *dl_error = dlerror();
//     if (dl_error != NULL) {
//         size_t error_len = strlen(dl_error);
//         *error = malloc(error_len + 1);
//         if (*error != NULL) {
//             memcpy(*error, dl_error, error_len + 1);
//         }
//     }
//     return 0;
// }
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

	var errorMessage *C.char
	if C.check_library_exists(cname, &errorMessage) != 0 {
		return nil
	}
	defer C.free(unsafe.Pointer(errorMessage))

	var errstr string
	if errorMessage != nil {
		errstr = C.GoString(errorMessage)
	}
	return fmt.Errorf("could not locate %s: %s", libname, errstr)
}
