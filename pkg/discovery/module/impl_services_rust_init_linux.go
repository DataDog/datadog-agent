// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build cgo

package module

/*
#cgo CFLAGS:  -I${SRCDIR}/rust/include
#cgo LDFLAGS: -L${SRCDIR}/rust -L${SRCDIR}/rust/target/release -ldd_discovery

#include "dd_discovery.h"

extern void goDiscoveryLogCallback(uint32_t level, const char* msg, size_t msg_len);

// cgo does not allow passing the address of an exported Go function (goDiscoveryLogCallback)
// directly as a function-pointer argument in the cgo preamble.  This thin C wrapper returns
// the pointer so we can pass it to dd_discovery_init_logger without violating that rule.
static dd_log_fn getGoLogCallback(void) {
    return goDiscoveryLogCallback;
}
*/
import "C"

// InitDiscoveryLogger registers the Go logging callback with the Rust library.
// Called explicitly from NewDiscoveryModule to allow a future config variable to gate this call.
func InitDiscoveryLogger() {
	C.dd_discovery_init_logger(C.getGoLogCallback(), C.uint32_t(goLevelToRust()))
}
