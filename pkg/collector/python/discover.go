// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// RunDiscover invokes the Python `discover(service)` classmethod on the
// integration's check class via the rtloader bridge. integrationName is the
// integration's module name (e.g. "krakend"). serviceJSON is the
// JSON-serialized listeners.Service projection that the Python bridge
// helper deserializes.
//
// Returns the JSON-encoded result from Python (typically a list of dicts,
// or "null"). Returns an error if the rtloader is not initialised, the
// check class cannot be loaded, or the bridge call fails.
func RunDiscover(integrationName, serviceJSON string) (string, error) {
	if rtloader == nil {
		return "", ErrNotInitialized
	}

	glock, err := newStickyLock()
	if err != nil {
		return "", fmt.Errorf("python discover: GIL acquire: %w", err)
	}
	defer glock.unlock()

	// Resolve the integration's check class. Mirror the loader.go convention
	// of trying `<wheelNamespace>.<name>` first, then plain `<name>`.
	candidates := []string{
		fmt.Sprintf("%s.%s", wheelNamespace, integrationName),
		integrationName,
	}

	var checkModule *C.rtloader_pyobject_t
	var checkClass *C.rtloader_pyobject_t
	for _, name := range candidates {
		cName := TrackedCString(name)
		res := C.get_class(rtloader, cName, &checkModule, &checkClass)
		C.call_free(unsafe.Pointer(cName))
		if res != 0 {
			break
		}
	}
	if checkClass == nil {
		if rterr := getRtLoaderError(); rterr != nil {
			return "", fmt.Errorf("python discover: load class %q: %w", integrationName, rterr)
		}
		return "", fmt.Errorf("python discover: integration %q has no Python check class", integrationName)
	}
	defer C.rtloader_decref(rtloader, checkClass)
	if checkModule != nil {
		defer C.rtloader_decref(rtloader, checkModule)
	}

	cJSON := TrackedCString(serviceJSON)
	defer C.call_free(unsafe.Pointer(cJSON))

	cResult := C.run_discover(rtloader, checkClass, cJSON)
	if cResult == nil {
		if rterr := getRtLoaderError(); rterr != nil {
			return "", fmt.Errorf("python discover: %s.discover: %w", integrationName, rterr)
		}
		return "", errors.New("python discover: rtloader returned NULL")
	}
	defer C.rtloader_free(rtloader, unsafe.Pointer(cResult))

	result := C.GoString(cResult)
	log.Debugf("python discover: %s returned %d bytes", integrationName, len(result))
	return result, nil
}
