// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build python

package python

/*
#include <stdint.h>
*/
import "C"

import (
	"runtime/cgo"
	"unsafe"
)

//export remoteQueryStreamEmitBridge
func remoteQueryStreamEmitBridge(eventJSON *C.char, userdata unsafe.Pointer) C.int {
	h := cgo.Handle(uintptr(userdata))
	emit := h.Value().(func(string) error)
	if err := emit(C.GoString(eventJSON)); err != nil {
		return 1
	}
	return 0
}
