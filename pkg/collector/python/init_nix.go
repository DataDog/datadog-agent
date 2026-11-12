// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && !windows

package python

import (
	"unsafe"

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

	if !pkgconfigsetup.Datadog().GetBool("c_core_dump") {
		return nil
	}

	var coreDumpErr *C.char
	if C.enable_core_dumps(&coreDumpErr) == 0 {
		log.Errorf("Unable to enable C core dumps: %s", C.GoString(coreDumpErr))
		if coreDumpErr != nil {
			C.call_free(unsafe.Pointer(coreDumpErr))
		}
	}

	return nil
}
