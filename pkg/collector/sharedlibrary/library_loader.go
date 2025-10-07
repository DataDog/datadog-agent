// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharedlibrary

import (
	"errors"
	"fmt"
	"unsafe"
)

/*
#include <stdlib.h>

#include "shared_library.h"

void SubmitMetricSo(char *, metric_type_t, char *, double, char **, char *, bool);
void SubmitServiceCheckSo(char *, char *, int, char **, char *, char *);
void SubmitEventSo(char *, event_t *);
void SubmitHistogramBucketSo(char *, char *, long long, float, float, int, char *, char **, bool);
void SubmitEventPlatformEventSo(char *, char *, int, char *);

// the callbacks are aggregated in this file as it's the only one which uses it
static const aggregator_t aggregator = {
	SubmitMetricSo,
	SubmitServiceCheckSo,
	SubmitEventSo,
	SubmitHistogramBucketSo,
	SubmitEventPlatformEventSo,
};

static const aggregator_t *get_aggregator() {
	return &aggregator;
}
*/
import "C"

// libraryLoader is an interface that handles opening, running and closing shared libraries
type libraryLoader interface {
	Load(libName string) (C.handles_t, error)
	Run(runHandle *C.run_function_t, checkID string, initConfig string, instanceConfig string, aggregator C.aggregator_t) error
	Close(libHandle *C.void)
}

// SharedLibraryLoader is an interface to load/close shared libraries and run their `Run` symbol
type sharedLibraryLoader struct {}

// Load looks for a shared library with the corresponding name and check if it has a `Run` symbol.
// If that's the case, then the method will return handles for both.
func (l *sharedLibraryLoader) Load(libName string) (C.handles_t, error) {
	var cErr *C.char

	// the prefix "libdatadog-agent-" is required to avoid possible name conflicts with other shared libraries in the include path
	fullName := "libdatadog-agent-" + libName

	cFullName := C.CString(fullName)
	defer C.free(unsafe.Pointer(cFullName))

	libHandles := C.load_shared_library(cFullName, &cErr)
	if cErr != nil {
		err := C.GoString(cErr)
		defer C.free(unsafe.Pointer(cErr))

		// the loading error message can be very verbose (~850 chars)
		if len(err) > 300 {
			err = err[:300] + "..."
		}

		errMsg := fmt.Sprintf("failed to load shared library %q: %s", fullName, err)
		return C.handles_t{}, errors.New(errMsg)
	}

	return libHandles, nil
}

func (l *sharedLibraryLoader) Run(runHandle *C.run_function_t, checkID string, initConfig string, instanceConfig string) error {
	cID := C.CString(checkID)
	defer C.free(unsafe.Pointer(cID))

	cInitConfig := C.CString(initConfig)
	defer C.free(unsafe.Pointer(cInitConfig))

	cInstanceConfig := C.CString(instanceConfig)
	defer C.free(unsafe.Pointer(cInstanceConfig))

	var cErr *C.char
	
	C.run_shared_library(runHandle, cID, cInitConfig, cInstanceConfig, C.get_aggregator(), &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return fmt.Errorf("Run failed: %s", C.GoString(cErr))
	}

	return nil
}

func (l *sharedLibraryLoader) Close(libHandle unsafe.Pointer) error {
	var cErr *C.char

	C.close_shared_library(libHandle, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return fmt.Errorf("Run failed: %s", C.GoString(cErr))
	}

	return nil

}

type mockSharedLibraryLoader struct {}

func (ml *mockSharedLibraryLoader) Load(_libName string) (C.handles_t, error) {
	return C.handles_t{}, nil
}
