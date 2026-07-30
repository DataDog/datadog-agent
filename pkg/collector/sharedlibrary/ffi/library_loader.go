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
	"path/filepath"
	"regexp"
	"runtime"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/collector/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

/*
#include <stdlib.h>

#cgo CFLAGS: -I "${SRCDIR}/../../../../rtloader/include"
#include "ffi.h"

// Build the aggregator callback table from pointers owned by the aggregator
// package. The void* to callback-type casts are done here in C so this package
// never references the aggregator's exported symbols in its own cgo link (which
// fails on the MinGW/Windows linker).
const aggregator_t *build_aggregator(void *m, void *sc, void *e, void *h, void *ep) {
	static aggregator_t aggregator;
	aggregator.cb_submit_metric = (cb_submit_metric_t)m;
	aggregator.cb_submit_service_check = (cb_submit_service_check_t)sc;
	aggregator.cb_submit_event = (cb_submit_event_t)e;
	aggregator.cb_submit_histogram_bucket = (cb_submit_histogram_bucket_t)h;
	aggregator.cb_submit_event_platform_event = (cb_submit_event_platform_event_t)ep;
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
	ComputeLibraryPath(name string) (string, error)
}

// validCheckName is an allowlist for shared library check names.
// Only alphanumeric characters, hyphens, and underscores are permitted,
// and the name must start with an alphanumeric character. This prevents
// path traversal via autodiscovery-supplied check names (e.g. container labels).
var validCheckName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func validateLibraryName(name string) error {
	if !validCheckName.MatchString(name) {
		return fmt.Errorf("check name %q must start with an alphanumeric character and contain only alphanumeric characters, hyphens, or underscores", name)
	}
	return nil
}

// isPathConfined reports whether libPath is a direct child of folderPath.
func isPathConfined(libPath, folderPath string) bool {
	return path.Dir(path.Clean(libPath)) == path.Clean(folderPath)
}

// SharedLibraryLoader loads and uses shared libraries
type SharedLibraryLoader struct {
	folderPath string
	aggregator *C.aggregator_t
	permission *filesystem.Permission
}

// Open looks for a shared library with the corresponding name and check if it has the required symbols
func (l *SharedLibraryLoader) Open(path string) (*Library, error) {
	// Check the containing directory first: if it is owned by a trusted user and not
	// world-writable, an attacker cannot stage a replacement library between our
	// permission check and the actual dlopen call (TOCTOU mitigation).
	if err := l.permission.CheckOwnerIsTrusted(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("shared library directory owner check failed: %w", err)
	}

	// Note: there is an inherent TOCTOU race between this check and dlopen below.
	// It is mitigated by the library directory being owned by a trusted user (above).
	if err := l.permission.CheckOwnerAndPermissionsAreRestricted(path); err != nil {
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

// ComputeLibraryPath returns the full expected path of the library, after
// validating that the name cannot escape the configured library folder.
func (l *SharedLibraryLoader) ComputeLibraryPath(name string) (string, error) {
	if err := validateLibraryName(name); err != nil {
		return "", err
	}
	// the prefix "libdatadog-agent-" is required to avoid possible name conflicts with other shared libraries in the include path
	libPath := path.Join(l.folderPath, "libdatadog-agent-"+name+"."+getLibExtension())
	if !isPathConfined(libPath, l.folderPath) {
		return "", errors.New("library path is outside the configured checks directory")
	}
	return libPath, nil
}

// NewSharedLibraryLoader creates a new SharedLibraryLoader
func NewSharedLibraryLoader(folderPath string) (*SharedLibraryLoader, error) {
	permission, err := filesystem.NewPermission()
	if err != nil {
		return nil, err
	}
	cb := aggregator.GetCallbacks()
	return &SharedLibraryLoader{
		folderPath: folderPath,
		aggregator: C.build_aggregator(cb.Metric, cb.ServiceCheck, cb.Event, cb.HistogramBucket, cb.EventPlatformEvent),
		permission: permission,
	}, nil
}
