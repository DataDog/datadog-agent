// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package validate provides hostname validation helpers
package validate

/*
#cgo LDFLAGS: -L${SRCDIR}/../rustlib/target/release -ldd_dll_hostname
#include <stdlib.h>
extern int dd_valid_hostname_code(const char* input);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// ValidHostname determines whether the passed string is a valid hostname.
// It delegates to the rustlib validator and returns a descriptive error on failure.
func ValidHostname(hostname string) error {
	cIn := C.CString(hostname)
	defer C.free(unsafe.Pointer(cIn))
	code := C.dd_valid_hostname_code(cIn)
	switch int(code) {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("hostname is empty")
	case 2:
		return fmt.Errorf("%s is a local hostname", hostname)
	case 3:
		return fmt.Errorf("name exceeded the maximum length of 255 characters")
	default:
		return fmt.Errorf("%s is not RFC1123 compliant", hostname)
	}
}
