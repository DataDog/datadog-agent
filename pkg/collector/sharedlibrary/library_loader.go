// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharedlibrary

import (
	"fmt"
	"path"
	"runtime"
	"unsafe"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

/*
#include <stdlib.h>

#include "ffi.h"

void SubmitMetricSo(char *, metric_type_t, char *, double, char **, char *, bool);
void SubmitServiceCheckSo(char *, char *, int, char **, char *, char *);
void SubmitEventSo(char *, event_t *);
void SubmitHistogramBucketSo(char *, char *, long long, float, float, int, char *, char **, bool);
void SubmitEventPlatformEventSo(char *, char *, int, char *);

// the callbacks are aggregated in this file as it's the only one which uses it
const aggregator_t aggregator = {
	SubmitMetricSo,
	SubmitServiceCheckSo,
	SubmitEventSo,
	SubmitHistogramBucketSo,
	SubmitEventPlatformEventSo,
};

const aggregator_t *get_aggregator() {
	return &aggregator;
}
*/
import "C"

func getLibExtension() string {
	switch runtime.GOOS {
	case "linux", "freebsd":
		return ".so"
	case "darwin":
		return ".dylib"
	case "windows":
		return ".dll"
	default:
		return ".so"
	}
}

// store handles for loading, running checks
type libraryHandles struct {
	lib unsafe.Pointer
	run *C.run_function_t
}

// libraryLoader is an interface that handles opening, running and closing shared libraries
type libraryLoader interface {
	Load(name string) (libraryHandles, error)
	Run(runHandle *C.run_function_t, checkID string, initConfig string, instanceConfig string) error
	Close(libHandle unsafe.Pointer) error
}

// SharedLibraryLoader is an interface to load/close shared libraries and run their `Run` symbol
type sharedLibraryLoader struct {
	libraryFolder string
	aggregator    *C.aggregator_t
}

// Load looks for a shared library with the corresponding name and check if it has a `Run` symbol.
// If that's the case, then the method will return handles for both.
func (l *sharedLibraryLoader) Load(name string) (libraryHandles, error) {
	var cErr *C.char

	// the prefix "libdatadog-agent-" is required to avoid possible name conflicts with other shared libraries in the include path
	libPath := path.Join(l.libraryFolder, "libdatadog-agent-"+name+getLibExtension())

	cLibPath := C.CString(libPath)
	defer C.free(unsafe.Pointer(cLibPath))

	cLibHandles := C.load_shared_library(cLibPath, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))

		return libraryHandles{}, fmt.Errorf("failed to load shared library %q", libPath)
	}

	return (libraryHandles)(cLibHandles), nil
}

func (l *sharedLibraryLoader) Run(runHandle *C.run_function_t, checkID string, initConfig string, instanceConfig string) error {
	cID := C.CString(checkID)
	defer C.free(unsafe.Pointer(cID))

	cInitConfig := C.CString(initConfig)
	defer C.free(unsafe.Pointer(cInitConfig))

	cInstanceConfig := C.CString(instanceConfig)
	defer C.free(unsafe.Pointer(cInstanceConfig))

	var cErr *C.char

	C.run_shared_library(runHandle, cID, cInitConfig, cInstanceConfig, l.aggregator, &cErr)
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
		return fmt.Errorf("Close failed: %s", C.GoString(cErr))
	}

	return nil
}

func createNewDefaultSharedLibraryLoader() *sharedLibraryLoader {
	return &sharedLibraryLoader{
		libraryFolder: pkgconfigsetup.Datadog().GetString("additional_checksd"),
		aggregator:    C.get_aggregator(),
	}
}

// mock of the sharedLibraryLoader
type mockSharedLibraryLoader struct{}

func (ml *mockSharedLibraryLoader) Load(_ string) (libraryHandles, error) {
	return libraryHandles{}, nil
}

func (ml *mockSharedLibraryLoader) Run(_ *C.run_function_t, _ string, _ string, _ string) error {
	return nil
}

func (ml *mockSharedLibraryLoader) Close(_ unsafe.Pointer) error {
	return nil
}
