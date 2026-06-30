// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build python

package python

/*
#include <stdint.h>
#include <stdlib.h>
*/
import "C"

import (
	"runtime/cgo"
	"unsafe"

	checkbase "github.com/DataDog/datadog-agent/pkg/collector/check"
)

//export remoteQueryStreamEmitBridge
func remoteQueryStreamEmitBridge(eventType *C.char, metadataJSON *C.char, payload *C.uint8_t, payloadLen C.size_t, userdata unsafe.Pointer) C.int {
	h := cgo.Handle(uintptr(userdata))
	emit := h.Value().(func(checkbase.RemoteQueryStreamEvent) error)
	event := checkbase.RemoteQueryStreamEvent{
		Type:         C.GoString(eventType),
		MetadataJSON: C.GoString(metadataJSON),
	}
	if payloadLen > 0 {
		event.Payload = C.GoBytes(unsafe.Pointer(payload), C.int(payloadLen))
	} else {
		event.Payload = []byte{}
	}
	if err := emit(event); err != nil {
		return 1
	}
	return 0
}
