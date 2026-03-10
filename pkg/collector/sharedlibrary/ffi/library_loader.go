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
	"sync"
	"unsafe"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	metricsevent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#include <stdlib.h>
#include <stdint.h>

#cgo CFLAGS: -I "${SRCDIR}/../../../../rtloader/include"
#include "ffi.h"

// run_check_with_ctx wraps run_check_shared_library, converting a uintptr_t
// context ID to void* so Go code doesn't need unsafe.Pointer(uintptr) casts
// that trigger go vet warnings.
static void run_check_with_ctx(check_run_function_t *check_run_ptr, const char *init_config, const char *instance_config, const char *enrichment, const callback_t *callback, uintptr_t ctx_id, const char **error) {
	run_check_shared_library(check_run_ptr, init_config, instance_config, enrichment, callback, (void *)ctx_id, error);
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
	Run(lib *Library, checkID string, initConfig string, instanceConfig string, enrichment string, senderManager sender.SenderManager) error
	Version(lib *Library) (string, error)
	ComputeLibraryPath(name string) string
}

// SharedLibraryLoader loads and uses shared libraries
type SharedLibraryLoader struct {
	folderPath string
}

// Open looks for a shared library with the corresponding name and check if it has the required symbols
func (l *SharedLibraryLoader) Open(path string) (*Library, error) {
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
// It creates a bridge context containing the senderManager and checkID, populates a callback_t
// struct with Go bridge functions, and passes both through to the Rust check.
func (l *SharedLibraryLoader) Run(lib *Library, checkID string, initConfig string, instanceConfig string, enrichment string, senderManager sender.SenderManager) error {
	if lib == nil {
		return errors.New("Pointer to 'Library' struct is NULL")
	}

	cInitConfig := C.CString(initConfig)
	defer C.free(unsafe.Pointer(cInitConfig))

	cInstanceConfig := C.CString(instanceConfig)
	defer C.free(unsafe.Pointer(cInstanceConfig))

	cEnrichment := C.CString(enrichment)
	defer C.free(unsafe.Pointer(cEnrichment))

	// Register a bridge context so callbacks can route to the correct sender
	bc := &BridgeContext{
		senderManager: senderManager,
		checkID:       checkid.ID(checkID),
	}
	ctxID := registerBridgeContext(bc)
	defer unregisterBridgeContext(ctxID)

	// Build the callback struct with our Go bridge functions
	callback := buildCallback()

	var cErr *C.char

	C.run_check_with_ctx(lib.checkRun, cInitConfig, cInstanceConfig, cEnrichment, &callback, C.uintptr_t(ctxID), &cErr)
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
func NewSharedLibraryLoader(folderPath string) *SharedLibraryLoader {
	return &SharedLibraryLoader{
		folderPath: folderPath,
	}
}

// ---- Bridge context management ----
// A BridgeContext maps an opaque void* pointer back to the Go-side sender
// for a specific check instance. This replaces the old model where check_id
// was passed as a string through the C ABI.

// BridgeContext holds the sender manager and check ID for callback routing
type BridgeContext struct {
	senderManager sender.SenderManager
	checkID       checkid.ID
}

var (
	bridgeMu       sync.Mutex
	bridgeContexts = map[uintptr]*BridgeContext{}
	bridgeNextID   uintptr
)

func registerBridgeContext(bc *BridgeContext) uintptr {
	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	bridgeNextID++
	bridgeContexts[bridgeNextID] = bc
	return bridgeNextID
}

func unregisterBridgeContext(id uintptr) {
	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	delete(bridgeContexts, id)
}

func lookupBridgeContext(ctx unsafe.Pointer) *BridgeContext {
	id := uintptr(ctx)
	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	return bridgeContexts[id]
}

// ---- Bridge callback functions ----
// These are Go functions exported to C that match the callback_t function
// pointer signatures. They extract the BridgeContext from the ctx pointer
// and route submissions to the appropriate sender.

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
	bc := lookupBridgeContext(ctx)
	if bc == nil {
		log.Error("BridgeSubmitMetric: invalid bridge context")
		return
	}

	s, err := bc.senderManager.GetSender(bc.checkID)
	if err != nil || s == nil {
		log.Errorf("BridgeSubmitMetric: error getting sender: %v", err)
		return
	}

	_name := C.GoString(name)
	_value := float64(value)
	_hostname := C.GoString(hostname)
	_tags := cStringArrayToSlice(tags)
	_flushFirst := (flushFirst != 0)

	switch metricType {
	case 0: // Gauge
		s.Gauge(_name, _value, _hostname, _tags)
	case 1: // Rate
		s.Rate(_name, _value, _hostname, _tags)
	case 2: // Count
		s.Count(_name, _value, _hostname, _tags)
	case 3: // MonotonicCount
		s.MonotonicCountWithFlushFirstValue(_name, _value, _hostname, _tags, _flushFirst)
	case 4: // Counter
		s.Counter(_name, _value, _hostname, _tags)
	case 5: // Histogram
		s.Histogram(_name, _value, _hostname, _tags)
	case 6: // Historate
		s.Historate(_name, _value, _hostname, _tags)
	}
}

//export BridgeSubmitServiceCheck
func BridgeSubmitServiceCheck(ctx unsafe.Pointer, name *C.char, status C.int, tags **C.char, hostname *C.char, message *C.char) {
	bc := lookupBridgeContext(ctx)
	if bc == nil {
		log.Error("BridgeSubmitServiceCheck: invalid bridge context")
		return
	}

	s, err := bc.senderManager.GetSender(bc.checkID)
	if err != nil || s == nil {
		log.Errorf("BridgeSubmitServiceCheck: error getting sender: %v", err)
		return
	}

	_name := C.GoString(name)
	_status := servicecheck.ServiceCheckStatus(status)
	_tags := cStringArrayToSlice(tags)
	_hostname := C.GoString(hostname)
	_message := C.GoString(message)

	s.ServiceCheck(_name, _status, _hostname, _tags, _message)
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
	bc := lookupBridgeContext(ctx)
	if bc == nil {
		log.Error("BridgeSubmitEvent: invalid bridge context")
		return
	}

	s, err := bc.senderManager.GetSender(bc.checkID)
	if err != nil || s == nil {
		log.Errorf("BridgeSubmitEvent: error getting sender: %v", err)
		return
	}

	_event := metricsevent.Event{
		Title:          bridgeEventParseString(event.title, "title"),
		Text:           bridgeEventParseString(event.text, "text"),
		Priority:       metricsevent.Priority(bridgeEventParseString(event.priority, "priority")),
		Host:           bridgeEventParseString(event.host, "host"),
		Tags:           cStringArrayToSlice((**C.char)(unsafe.Pointer(event.tags))),
		AlertType:      metricsevent.AlertType(bridgeEventParseString(event.alert_type, "alert_type")),
		AggregationKey: bridgeEventParseString(event.aggregation_key, "aggregation_key"),
		SourceTypeName: bridgeEventParseString(event.source_type_name, "source_type_name"),
		Ts:             int64(event.ts),
	}

	s.Event(_event)
}

//export BridgeSubmitHistogram
func BridgeSubmitHistogram(ctx unsafe.Pointer, name *C.char, value C.longlong, lowerBound C.float, upperBound C.float, monotonic C.int, hostname *C.char, tags **C.char, flushFirst C.int) {
	bc := lookupBridgeContext(ctx)
	if bc == nil {
		log.Error("BridgeSubmitHistogram: invalid bridge context")
		return
	}

	s, err := bc.senderManager.GetSender(bc.checkID)
	if err != nil || s == nil {
		log.Errorf("BridgeSubmitHistogram: error getting sender: %v", err)
		return
	}

	_name := C.GoString(name)
	_value := int64(value)
	_lowerBound := float64(lowerBound)
	_upperBound := float64(upperBound)
	_monotonic := (monotonic != 0)
	_hostname := C.GoString(hostname)
	_tags := cStringArrayToSlice(tags)
	_flushFirst := (flushFirst != 0)

	s.HistogramBucket(_name, _value, _lowerBound, _upperBound, _monotonic, _hostname, _tags, _flushFirst)
}

//export BridgeSubmitEventPlatformEvent
func BridgeSubmitEventPlatformEvent(ctx unsafe.Pointer, rawEventPtr *C.char, rawEventSize C.int, eventType *C.char) {
	bc := lookupBridgeContext(ctx)
	if bc == nil {
		log.Error("BridgeSubmitEventPlatformEvent: invalid bridge context")
		return
	}

	s, err := bc.senderManager.GetSender(bc.checkID)
	if err != nil || s == nil {
		log.Errorf("BridgeSubmitEventPlatformEvent: error getting sender: %v", err)
		return
	}

	s.EventPlatformEvent(C.GoBytes(unsafe.Pointer(rawEventPtr), rawEventSize), C.GoString(eventType))
}

//export BridgeSubmitLog
func BridgeSubmitLog(ctx unsafe.Pointer, level C.int, message *C.char) {
	bc := lookupBridgeContext(ctx)
	if bc == nil {
		log.Error("BridgeSubmitLog: invalid bridge context")
		return
	}

	_message := C.GoString(message)

	// Route log messages to the appropriate log level
	switch level {
	case 7: // Trace
		log.Tracef("[check:%s] %s", bc.checkID, _message)
	case 10: // Debug
		log.Debugf("[check:%s] %s", bc.checkID, _message)
	case 20: // Info
		log.Infof("[check:%s] %s", bc.checkID, _message)
	case 30: // Warning
		log.Warnf("[check:%s] %s", bc.checkID, _message)
	case 40: // Error
		log.Errorf("[check:%s] %s", bc.checkID, _message)
	case 50: // Critical
		log.Criticalf("[check:%s] %s", bc.checkID, _message)
	default:
		log.Infof("[check:%s] %s", bc.checkID, _message)
	}
}
