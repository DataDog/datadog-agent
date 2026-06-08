// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"fmt"
	"unsafe"
)

/*
#include <stdlib.h>

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"

static inline void call_free(void* ptr) {
    _free(ptr);
}
*/
import "C"

// DiscoverConfig calls a Python integration's discovery bridge for a service
// JSON payload and returns the raw discovery result JSON.
func DiscoverConfig(integrationName string, serviceJSON string) (string, error) {
	if err := ensurePythonRuntime(); err != nil {
		return "", err
	}

	cleanup, err := preparePythonLoaderRuntime()
	if err != nil {
		return "", err
	}
	defer cleanup()

	loadedClass, err := loadPythonCheckClass(integrationName)
	if err != nil {
		return "", err
	}
	defer loadedClass.decref()

	cServiceJSON := TrackedCString(serviceJSON)
	defer C.call_free(unsafe.Pointer(cServiceJSON))

	discoveryResult := C.discover_config(rtloader, loadedClass.class, cServiceJSON)
	if discoveryResult == nil {
		if err := getRtLoaderError(); err != nil {
			return "", fmt.Errorf("could not discover configs for python check %s: %w", integrationName, err)
		}
		return "", fmt.Errorf("could not discover configs for python check %s", integrationName)
	}
	defer C.rtloader_free(rtloader, unsafe.Pointer(discoveryResult))

	return C.GoString(discoveryResult), nil
}
