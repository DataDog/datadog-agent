// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

// Package ffi handles shared libraries through Cgo
package ffi

import (
	"errors"
	"fmt"
	"path"
	"runtime"
	"unsafe"

	_ "github.com/DataDog/datadog-agent/pkg/collector/aggregator" // import submit functions
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

/*
#include <stdlib.h>

#cgo CFLAGS: -I "${SRCDIR}/../../../../rtloader/include"
#include "ffi.h"

// functions from the aggregator package
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

const aggregator_t *get_aggregator(void) {
	return &aggregator;
}
*/
import "C"

func getLibExtension() string {
	switch runtime.GOOS {
	case "linux", "freebsd":
		return "so"
	case "windows":
		return "dll"
	case "darwin":
		return "dylib"
	default:
		return "so"
	}
}

// Library stores everything needed for using the shared libraries' symbols
type Library struct {
	handle  unsafe.Pointer
	run     *C.run_function_t
	version *C.version_function_t
}

// LibraryLoader is an interface for loading and using libraries
type LibraryLoader interface {
	Open(name string) (*Library, error)
	Close(lib *Library) error
	Run(lib *Library, checkID string, initConfig string, instanceConfig string) error
	Version(lib *Library) (string, error)
	ComputeLibraryPath(name string) string
}

// SharedLibraryLoader loads and uses shared libraries
type SharedLibraryLoader struct {
	folderPath string
	aggregator *C.aggregator_t
	permission *filesystem.Permission
}

// Open looks for a shared library with the corresponding name and check if it has the required symbols
func (l *SharedLibraryLoader) Open(path string) (*Library, error) {
	if err := l.permission.CheckOwnerAndPermissions(path); err != nil {
		return nil, err
	}

	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	var cErr *C.char

	cLib := C.load_shared_library(cPath, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return nil, errors.New(C.GoString(cErr))
	}

	return (*Library)(&cLib), nil
}

// Close closes the shared library
func (l *SharedLibraryLoader) Close(lib *Library) error {
	if lib == nil {
		return errors.New("Pointer to 'Library' struct is NULL")
	}

	var cErr *C.char

	C.close_shared_library(lib.handle, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return fmt.Errorf("Close failed: %s", C.GoString(cErr))
	}

	return nil
}

// Run calls the `Run` symbol of the shared library to execute the check's implementation
func (l *SharedLibraryLoader) Run(lib *Library, checkID string, initConfig string, instanceConfig string) error {
	if lib == nil {
		return errors.New("Pointer to 'Library' struct is NULL")
	}

	cID := C.CString(checkID)
	defer C.free(unsafe.Pointer(cID))

	cInitConfig := C.CString(initConfig)
	defer C.free(unsafe.Pointer(cInitConfig))

	cInstanceConfig := C.CString(instanceConfig)
	defer C.free(unsafe.Pointer(cInstanceConfig))

	var cErr *C.char

	C.run_shared_library(lib.run, cID, cInitConfig, cInstanceConfig, l.aggregator, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return fmt.Errorf("Failed to run check: %s", C.GoString(cErr))
	}

	return nil
}

// Version calls the `Version` symbol to retrieve the check version
func (l *SharedLibraryLoader) Version(lib *Library) (string, error) {
	if lib == nil {
		return "", errors.New("Pointer to 'Library' struct is NULL")
	}

	var cErr *C.char

	cLibVersion := C.get_version_shared_library(lib.version, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return "", fmt.Errorf("Failed to get version: %s", C.GoString(cErr))
	}

	return C.GoString(cLibVersion), nil

}

// ComputeLibraryPath returns the full expected path of the library
func (l *SharedLibraryLoader) ComputeLibraryPath(name string) string {
	// the prefix "libdatadog-agent-" is required to avoid possible name conflicts with other shared libraries in the include path
	return path.Join(l.folderPath, "libdatadog-agent-"+name+"."+getLibExtension())
}

// NewSharedLibraryLoader creates a new SharedLibraryLoader
func NewSharedLibraryLoader(folderPath string) (*SharedLibraryLoader, error) {
	permission, err := filesystem.NewPermission()
	if err != nil {
		return nil, err
	}
	return &SharedLibraryLoader{
		folderPath: folderPath,
		aggregator: C.get_aggregator(),
		permission: permission,
	}, nil
}
