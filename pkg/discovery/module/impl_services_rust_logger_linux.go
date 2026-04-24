// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build cgo

package module

/*
#cgo CFLAGS: -I${SRCDIR}/rust/include
#include "dd_discovery.h"
*/
import "C"

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//export goDiscoveryLogCallback
func goDiscoveryLogCallback(level C.uint32_t, msg *C.char, msgLen C.size_t) {
	// Reject nil pointers, empty messages (nothing useful to log), and messages
	// larger than MaxInt32 to prevent overflow when casting msgLen to C.int below.
	if msg == nil || msgLen == 0 || msgLen > math.MaxInt32 {
		return
	}
	// Check the level before allocating the Go string to avoid heap work for dropped records.
	if !log.ShouldLog(rustLevelToGoLevel(uint32(level))) {
		return
	}
	handleDiscoveryLog(uint32(level), C.GoStringN(msg, C.int(msgLen)))
}
