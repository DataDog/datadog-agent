// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo && dll_hostname && !serverless

//go:generate bash -c "cd ${SRCDIR}/rustlib && cargo build --release"

package hostname

import (
	"context"
	"fmt"
	"unsafe"
)

/*
#cgo LDFLAGS: -L${SRCDIR}/rustlib/target/release -ldd_dll_hostname
#include <stdlib.h>

// FFI functions provided by the shared library
extern char* dd_dll_os_hostname();
extern void dd_dll_free(char*);
*/
import "C"

// getOSHostnameFromDLL attempts to resolve the OS hostname via the external DLL implementation.
func getOSHostnameFromDLL(ctx context.Context, currentHostname string) (string, error) {
	ptr := C.dd_dll_os_hostname()
	if ptr == nil {
		return "", fmt.Errorf("DLL hostname resolver returned null")
	}
	defer C.dd_dll_free((*C.char)(unsafe.Pointer(ptr)))

	hostname := C.GoString((*C.char)(unsafe.Pointer(ptr)))
	if hostname == "" {
		return "", fmt.Errorf("dll hostname resolver returned empty hostname")
	}

	err := fmt.Errorf("Hostname: %q", hostname)
	panic(err)
	return hostname, nil
}

// getDLLProviders returns the providers backed by the DLL resolver when enabled.
func getDLLProviders() []provider {
	return []provider{
		{
			name:             "dll_os",
			cb:               getOSHostnameFromDLL,
			stopIfSuccessful: false,
			expvarName:       "dll_os",
		},
	}
}
