// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && !windows

package python

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/config"
)

/*
#cgo !windows LDFLAGS: -ldatadog-agent-rtloader -ldl

#include <datadog_agent_rtloader.h>
#include <rtloader_mem.h>
*/
import "C"

// Any platform-specific initialization belongs here.
func initializePlatform() error {
	// Setup crash handling specifics - *NIX-only
	if config.Datadog.GetBool("c_stacktrace_collection") {
		var cCoreDump int

		if config.Datadog.GetBool("c_core_dump") {
			cCoreDump = 1
		}

		var handlerErr *C.char = nil
		if C.handle_crashes(C.int(cCoreDump), &handlerErr) == 0 {
			log.Errorf("Unable to install crash handler, C-land stacktraces and dumps will be unavailable: %s", C.GoString(handlerErr))
			if handlerErr != nil {
				C._free(unsafe.Pointer(handlerErr))
			}
		}
	}

	return nil
}
