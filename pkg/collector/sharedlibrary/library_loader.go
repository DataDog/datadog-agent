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

	_ "github.com/DataDog/datadog-agent/pkg/collector/aggregator" // import submit functions
)

/*
#include <stdlib.h>

#cgo CFLAGS: -I "${SRCDIR}/../../../rtloader/include"
#include "ffi.h"

// functions from the Python package
extern void SubmitMetric(char *, metric_type_t, char *, double, char **, char *, bool);
extern void SubmitServiceCheck(char *, char *, int, char **, char *, char *);
extern void SubmitEvent(char *, event_t *);
extern void SubmitHistogramBucket(char *, char *, long long, float, float, int, char *, char **, bool);
extern void SubmitEventPlatformEvent(char *, char *, int, char *);

// the callbacks are aggregated in this file as it's the only one which uses it
const aggregator_t aggregator = {
	SubmitMetric,
	SubmitServiceCheck,
	SubmitEvent,
	SubmitHistogramBucket,
	SubmitEventPlatformEvent,
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

// libraryHandles stores everything needed for a shared library check
type libraryHandles struct {
	lib     unsafe.Pointer
	run     *C.run_function_t
	version *C.version_function_t
}

// libraryLoader is an interface to load/close checks' shared libraries and call their symbols
type libraryLoader interface {
	Load(name string) (libraryHandles, error)
	Close(libHandle unsafe.Pointer) error
	Run(runPtr *C.run_function_t, checkID string, initConfig string, instanceConfig string) error
	Version(versionPtr *C.version_function_t) (string, error)
}

type sharedLibraryLoader struct {
	folderPath string
	aggregator *C.aggregator_t
}

// Load looks for a shared library with the corresponding name and check if it has a `Run` symbol.
// If that's the case, then the method will return handles for both.
func (l *sharedLibraryLoader) Load(name string) (libraryHandles, error) {
	// the prefix "libdatadog-agent-" is required to avoid possible name conflicts with other shared libraries in the include path
	libPath := path.Join(l.folderPath, "libdatadog-agent-"+name+getLibExtension())

	cLibPath := C.CString(libPath)
	defer C.free(unsafe.Pointer(cLibPath))

	var cErr *C.char

	cLibHandles := C.load_shared_library(cLibPath, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return libraryHandles{}, fmt.Errorf("failed to load shared library at %q: %s", libPath, C.GoString(cErr))
	}

	return (libraryHandles)(cLibHandles), nil
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

func (l *sharedLibraryLoader) Run(runPtr *C.run_function_t, checkID string, initConfig string, instanceConfig string) error {
	cID := C.CString(checkID)
	defer C.free(unsafe.Pointer(cID))

	cInitConfig := C.CString(initConfig)
	defer C.free(unsafe.Pointer(cInitConfig))

	cInstanceConfig := C.CString(instanceConfig)
	defer C.free(unsafe.Pointer(cInstanceConfig))

	var cErr *C.char

	C.run_shared_library(runPtr, cID, cInitConfig, cInstanceConfig, l.aggregator, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return fmt.Errorf("Run failed: %s", C.GoString(cErr))
	}

	return nil
}

func (l *sharedLibraryLoader) Version(versionPtr *C.version_function_t) (string, error) {
	var cErr *C.char

	cLibVersion := C.get_version_shared_library(versionPtr, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return "", fmt.Errorf("Failed to get version: %s", C.GoString(cErr))
	}

	return C.GoString(cLibVersion), nil

}

func newSharedLibraryLoader(folderPath string) *sharedLibraryLoader {
	return &sharedLibraryLoader{
		folderPath: folderPath,
		aggregator: C.get_aggregator(),
	}
}

// mock of sharedLibraryLoader
type mockSharedLibraryLoader struct{}

func (ml *mockSharedLibraryLoader) Load(_ string) (libraryHandles, error) {
	return libraryHandles{}, nil
}

func (ml *mockSharedLibraryLoader) Close(_ unsafe.Pointer) error {
	return nil
}

func (ml *mockSharedLibraryLoader) Run(_ *C.run_function_t, _ string, _ string, _ string) error {
	return nil
}

func (ml *mockSharedLibraryLoader) Version(_ *C.version_function_t) (string, error) {
	return "mock_version", nil
}
