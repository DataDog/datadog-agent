// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf && cgo

package module

/*
#cgo CFLAGS:  -I${SRCDIR}/../../discovery/module/rust/include
#cgo LDFLAGS: -L${SRCDIR}/../../discovery/module/rust -L${SRCDIR}/../../discovery/module/rust/target/release -ldd_discovery
#include "dd_discovery.h"
*/
import "C"

import (
	"errors"
	"os"
	"unsafe"
)

// openLogFile validates and opens a log file through libdd_discovery, the
// implementation that system-probe-lite also uses.
func openLogFile(path string, noFollow bool) (*os.File, error) {
	var errBuf [1024]C.char
	var errLen C.size_t
	pathPtr := (*C.char)(unsafe.Pointer(unsafe.StringData(path)))
	fd := C.dd_privileged_logs_open(pathPtr, C.size_t(len(path)), C.bool(noFollow), &errBuf[0], C.size_t(len(errBuf)), &errLen)
	if fd < 0 {
		return nil, errors.New(C.GoStringN(&errBuf[0], C.int(errLen)))
	}
	return os.NewFile(uintptr(fd), path), nil
}
