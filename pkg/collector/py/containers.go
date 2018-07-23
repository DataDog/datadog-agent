// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython

package py

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// #cgo pkg-config: python-2.7
// #cgo linux CFLAGS: -std=gnu99
// #include "api.h"
// #include "containers.h"
// #include <Python.h>
import "C"

var filter *containers.Filter

// IsContainerExcluded returns whether a container should be excluded,
// based on it's name and image name. Exclusion patterns are configured
// via the global options (ac_include/ac_exclude/exclude_pause_container)
//export IsContainerExcluded
func IsContainerExcluded(name, image *C.char) int {
	goName := C.GoString(name)
	goImg := C.GoString(image)

	// If init failed, fallback to False
	if filter == nil {
		return 0
	}

	if filter.IsExcluded(goName, goImg) {
		return 1
	} else {
		return 0
	}
}

func initContainers() {
	C.initcontainers()
	initContainerFilter()
}

// Separated to unit testing
func initContainerFilter() {
	var err error
	filter, err = containers.GetSharedFilter()
	if err != nil {
		log.Errorf("Error initializing container filtering: %s", err)
	}
}
