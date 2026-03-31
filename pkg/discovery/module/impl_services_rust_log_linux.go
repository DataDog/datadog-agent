// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Log bridge from the Rust discovery library to the Go logging system.
// Kept in a separate file because CGo restricts files with //export to
// declarations-only in their C preamble (no static function definitions).

//go:build dd_discovery_rust && cgo

package module

/*
#include <stddef.h>
#include <stdint.h>
*/
import "C"

import "github.com/DataDog/datadog-agent/pkg/util/log"

// goDiscoveryLog is called by the Rust library to forward log records to Go.
// level: 0=error, 1=warn, 2=info, 3=debug, 4=trace.
// msg is a UTF-8 string valid only for the duration of this call.
//
//export goDiscoveryLog
func goDiscoveryLog(level C.uint8_t, msg *C.char, msgLen C.size_t) {
	s := C.GoStringN(msg, C.int(msgLen))
	switch uint8(level) {
	case 0:
		log.Errorf("[dd_discovery] %s", s)
	case 1:
		log.Warnf("[dd_discovery] %s", s)
	case 2:
		log.Infof("[dd_discovery] %s", s)
	case 3:
		log.Debugf("[dd_discovery] %s", s)
	default:
		log.Tracef("[dd_discovery] %s", s)
	}
}
