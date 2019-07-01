// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build python

package python

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

/*
#include <datadog_agent_rtloader.h>
#cgo !windows LDFLAGS: -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -ldatadog-agent-rtloader -lstdc++ -static
*/
import "C"

var filter *containers.Filter

// IsContainerExcluded returns whether a container should be excluded,
// based on it's name and image name. Exclusion patterns are configured
// via the global options (ac_include/ac_exclude/exclude_pause_container)
//export IsContainerExcluded
func IsContainerExcluded(name, image *C.char) C.int {
	// If init failed, fallback to False
	if filter == nil {
		return 0
	}

	goName := C.GoString(name)
	goImg := C.GoString(image)

	if filter.IsExcluded(goName, goImg) {
		return 1
	}
	return 0
}

// Separated to unit testing
func initContainerFilter() {
	var err error
	if filter, err = containers.GetSharedFilter(); err != nil {
		log.Errorf("Error initializing container filtering: %s", err)
	}
}
