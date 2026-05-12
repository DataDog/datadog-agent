// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf && cgo

package module

/*
#cgo CFLAGS: -I${SRCDIR}/rust/include
#include "dd_discovery.h"
*/
import "C"

import "math"

//export goDiscoveryLogCallback
func goDiscoveryLogCallback(level C.uint32_t, msg *C.char, msgLen C.size_t) {
	// Reject nil pointers, empty messages (nothing useful to log), and lengths
	// larger than MaxInt32 to prevent overflow when casting msgLen to C.int below.
	if msg == nil || msgLen == 0 || msgLen > math.MaxInt32 {
		return
	}
	dispatchDiscoveryLog(uint32(level), C.GoStringN(msg, C.int(msgLen)))
}
