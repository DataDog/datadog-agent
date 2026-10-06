// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && !windows

package python

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/coredump"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

/*
#cgo !aix,!windows LDFLAGS: -ldatadog-agent-rtloader -ldl
#cgo aix LDFLAGS: -ldl

#include <datadog_agent_rtloader.h>
#include <rtloader_mem.h>

static inline void call_free(void* ptr) {
    _free(ptr);
}
*/
import "C"

// Any platform-specific initialization belongs here.
func initializePlatform() error {
	// Setup crash handling specifics - *NIX-only

	var cCoreDump int
	if cfg := pkgconfigsetup.Datadog(); cfg.GetBool("c_core_dump") {
		if coredump.BoundedEnabled(cfg) {
			// rtloader's handle_crashes sets RLIMIT_CORE to unlimited. With
			// python_lazy_loading (the default), it runs after coredump.Setup and
			// would remove the bound. So rtloader does not touch RLIMIT_CORE
			// here, and we apply the bounded limit instead (RLIMIT_CORE is
			// process-wide, so it covers C crashes too).
			coredump.ApplyBounded(cfg)
		} else {
			cCoreDump = 1
		}
	}

	var cStacktraceCollection int
	if pkgconfigsetup.Datadog().GetBool("c_stacktrace_collection") {
		cStacktraceCollection = 1
	}

	var handlerErr *C.char
	if C.handle_crashes(C.int(cCoreDump), C.int(cStacktraceCollection), &handlerErr) == 0 {
		log.Errorf("Unable to install crash handler, C-land stacktraces and dumps will be unavailable: %s", C.GoString(handlerErr))
		if handlerErr != nil {
			C.call_free(unsafe.Pointer(handlerErr))
		}
	}

	return nil
}
