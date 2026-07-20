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

	collectoraggregator "github.com/DataDog/datadog-agent/pkg/collector/aggregator"
	metricsevent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#include <stdlib.h>
#include <stdint.h>

#include "ffi.h"

// run_check_with_id passes check_id as the void* ctx argument expected by the
// check_run ABI, performing the char* → void* cast in C to avoid go vet
// warnings about unsafe pointer conversions in Go code.
static void run_check_with_id(check_run_function_t *check_run_ptr, const char *init_config, const char *instance_config, const char *enrichment, const callback_t *callback, const char *check_id, const char **error) {
	run_check_shared_library(check_run_ptr, init_config, instance_config, enrichment, callback, (void *)check_id, error);
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
	handle   unsafe.Pointer
	checkRun *C.check_run_function_t
	version  *C.version_function_t
}

// LibraryLoader is an interface for loading and using libraries
type LibraryLoader interface {
	Open(name string) (*Library, error)
	Close(lib *Library) error
	Run(lib *Library, checkID string, initConfig string, instanceConfig string, enrichment string) error
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

	return &Library{
		handle:   cLib.handle,
		checkRun: cLib.check_run,
		version:  cLib.version,
	}, nil
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

// Run calls the `check_run` symbol of the shared library to execute the check's implementation.
// The check ID is passed as the opaque ctx pointer so bridge callbacks can recover it and route
// submissions through pkg/collector/aggregator — the same path used by Python checks.
func (l *SharedLibraryLoader) Run(lib *Library, checkID string, initConfig string, instanceConfig string, enrichment string) error {
	if lib == nil {
		return errors.New("Pointer to 'Library' struct is NULL")
	}

	cInitConfig := C.CString(initConfig)
	defer C.free(unsafe.Pointer(cInitConfig))

	cInstanceConfig := C.CString(instanceConfig)
	defer C.free(unsafe.Pointer(cInstanceConfig))

	cEnrichment := C.CString(enrichment)
	defer C.free(unsafe.Pointer(cEnrichment))

	cCheckID := C.CString(checkID)
	defer C.free(unsafe.Pointer(cCheckID))

	// Build the callback struct with our Go bridge functions
	callback := buildCallback()

	var cErr *C.char

	C.run_check_with_id(lib.checkRun, cInitConfig, cInstanceConfig, cEnrichment, &callback, cCheckID, &cErr)
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
	return &SharedLibraryLoader{
		folderPath: folderPath,
		permission: permission,
	}, nil
}

// ---- Bridge callback functions ----
// These are Go functions exported to C that match the callback_t function
// pointer signatures. They recover the check ID from the opaque ctx pointer
// (set by run_check_with_id) and delegate to pkg/collector/aggregator — the
// same submission path used by Python checks.

// cStringArrayToSlice converts a null-terminated array of const char* to a Go string slice.
func cStringArrayToSlice(tags **C.char) []string {
	if tags == nil {
		return nil
	}
	var result []string
	for ptr := tags; *ptr != nil; ptr = (**C.char)(unsafe.Add(unsafe.Pointer(ptr), unsafe.Sizeof(*ptr))) {
		result = append(result, C.GoString(*ptr))
	}
	return result
}

//export BridgeSubmitMetric
func BridgeSubmitMetric(ctx unsafe.Pointer, metricType C.int, name *C.char, value C.double, tags **C.char, hostname *C.char, flushFirst C.int) {
	checkID := C.GoString((*C.char)(ctx))
	collectoraggregator.SubmitMetricForCheck(
		checkID,
		int(metricType),
		C.GoString(name),
		float64(value),
		cStringArrayToSlice(tags),
		C.GoString(hostname),
		flushFirst != 0,
	)
}

//export BridgeSubmitServiceCheck
func BridgeSubmitServiceCheck(ctx unsafe.Pointer, name *C.char, status C.int, tags **C.char, hostname *C.char, message *C.char) {
	checkID := C.GoString((*C.char)(ctx))
	collectoraggregator.SubmitServiceCheckForCheck(
		checkID,
		C.GoString(name),
		servicecheck.ServiceCheckStatus(status),
		cStringArrayToSlice(tags),
		C.GoString(hostname),
		C.GoString(message),
	)
}

func bridgeEventParseString(value *C.char, fieldName string) string {
	if value == nil {
		log.Tracef("Can't parse value for key '%s' in event submitted from slim check", fieldName)
		return ""
	}
	return C.GoString(value)
}

//export BridgeSubmitEvent
func BridgeSubmitEvent(ctx unsafe.Pointer, event *C.slim_event_t) {
	checkID := C.GoString((*C.char)(ctx))
	collectoraggregator.SubmitEventForCheck(
		checkID,
		metricsevent.Event{
			Title:          bridgeEventParseString(event.title, "title"),
			Text:           bridgeEventParseString(event.text, "text"),
			Priority:       metricsevent.Priority(bridgeEventParseString(event.priority, "priority")),
			Host:           bridgeEventParseString(event.host, "host"),
			Tags:           cStringArrayToSlice((**C.char)(unsafe.Pointer(event.tags))),
			AlertType:      metricsevent.AlertType(bridgeEventParseString(event.alert_type, "alert_type")),
			AggregationKey: bridgeEventParseString(event.aggregation_key, "aggregation_key"),
			SourceTypeName: bridgeEventParseString(event.source_type_name, "source_type_name"),
			Ts:             int64(event.ts),
		},
	)
}

//export BridgeSubmitHistogram
func BridgeSubmitHistogram(ctx unsafe.Pointer, name *C.char, value C.longlong, lowerBound C.float, upperBound C.float, monotonic C.int, hostname *C.char, tags **C.char, flushFirst C.int) {
	checkID := C.GoString((*C.char)(ctx))
	collectoraggregator.SubmitHistogramBucketForCheck(
		checkID,
		C.GoString(name),
		int64(value),
		float64(lowerBound),
		float64(upperBound),
		monotonic != 0,
		C.GoString(hostname),
		cStringArrayToSlice(tags),
		flushFirst != 0,
	)
}

//export BridgeSubmitEventPlatformEvent
func BridgeSubmitEventPlatformEvent(ctx unsafe.Pointer, rawEventPtr *C.char, rawEventSize C.int, eventType *C.char) {
	checkID := C.GoString((*C.char)(ctx))
	collectoraggregator.SubmitEventPlatformEventForCheck(
		checkID,
		C.GoBytes(unsafe.Pointer(rawEventPtr), rawEventSize),
		C.GoString(eventType),
	)
}

//export BridgeSubmitLog
func BridgeSubmitLog(ctx unsafe.Pointer, level C.int, message *C.char) {
	checkID := C.GoString((*C.char)(ctx))
	collectoraggregator.LogMessage(int(level), fmt.Sprintf("[check:%s] %s", checkID, C.GoString(message)))
}
