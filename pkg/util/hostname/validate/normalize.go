// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package validate

/*
#cgo LDFLAGS: -L${SRCDIR}/../rustlib/target/release -ldd_dll_hostname
#include <stdlib.h>
extern char* dd_normalize_host(const char* input);
extern char* dd_clean_hostname_dir(const char* input);
extern void dd_dll_free(char*);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// NormalizeHost applies a liberal policy on host names.
func NormalizeHost(host string) (string, error) {
	// Explicitly reject null runes to mirror previous Go behavior
	for i := 0; i < len(host); i++ {
		if host[i] == 0 { // '\x00'
			return "", fmt.Errorf("hostname cannot contain null character")
		}
	}

	cIn := C.CString(host)
	defer C.free(unsafe.Pointer(cIn))
	out := C.dd_normalize_host(cIn)
	if out == nil {
		return "", fmt.Errorf("invalid hostname")
	}
	defer C.dd_dll_free(out)
	return C.GoString(out), nil
}

// CleanHostnameDir returns a hostname normalized to be uses as a directory name. Used by the Flare logic.
func CleanHostnameDir(hostname string) string {
	cIn := C.CString(hostname)
	defer C.free(unsafe.Pointer(cIn))
	out := C.dd_clean_hostname_dir(cIn)
	if out == nil {
		return ""
	}
	defer C.dd_dll_free(out)
	return C.GoString(out)
}
