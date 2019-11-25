// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build python
// +build !windows

package python

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/config"
)

/*
#cgo !windows LDFLAGS: -ldatadog-agent-rtloader -ldl

#include <datadog_agent_rtloader.h>
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

		if C.handle_crashes(rtloader, C.int(cCoreDump)) == 0 {
			log.Errorf("Unable to install crash handler, C-land stacktraces and dumps will be unavailable")
		}
	}

	return nil
}
