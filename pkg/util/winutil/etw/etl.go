// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package etw

/*
#include "etw.h"
*/
import "C"
import (
	"fmt"
	"runtime/cgo"
	"unsafe"

	"golang.org/x/sys/windows"
)

// EventRecordFilter is an optional filter called for each raw event before parsing.
// Return true to process the event (call EventCallback), false to skip.
// Use for fast filtering by ProviderID and EventID to avoid parsing unwanted events.
type EventRecordFilter func(providerID windows.GUID, eventID uint16) bool

// EventCallback is called for each event that passes the filter (or all events if no filter).
type EventCallback func(event *Event)

// ProcessOptions configures ETL file processing.
type ProcessOptions struct {
	Filter EventRecordFilter
}

// ProcessOption configures ProcessETLFile.
type ProcessOption func(*ProcessOptions)

// WithEventRecordFilter sets a filter to skip events before parsing.
func WithEventRecordFilter(f EventRecordFilter) ProcessOption {
	return func(o *ProcessOptions) {
		o.Filter = f
	}
}

// ProcessETLFile reads an ETL file and invokes the callback for each event.
// Blocks until the file is fully processed or an error occurs.
func ProcessETLFile(etlPath string, callback EventCallback, opts ...ProcessOption) error {
	options := &ProcessOptions{}
	for _, opt := range opts {
		opt(options)
	}

	utf16Path, err := windows.UTF16PtrFromString(etlPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	ctx := &processContext{
		callback: callback,
		filter:   options.Filter,
	}
	handle := cgo.NewHandle(ctx)
	defer handle.Delete()

	var openErr C.ULONG
	traceHandle := C.DDOpenTraceFromFile((*C.WCHAR)(unsafe.Pointer(utf16Path)), C.uintptr_t(handle), &openErr)
	if traceHandle == C.INVALID_PROCESSTRACE_HANDLE {
		return fmt.Errorf("OpenTraceW failed: %w", windows.Errno(openErr))
	}
	defer C.CloseTrace(traceHandle)

	ret := C.DDProcessETLFile(traceHandle)
	switch status := windows.Errno(ret); status {
	case windows.ERROR_SUCCESS, windows.ERROR_CANCELLED:
		return nil
	default:
		return fmt.Errorf("ProcessTrace failed: %w", status)
	}
}

type processContext struct {
	callback EventCallback
	filter   EventRecordFilter
}

//export ddEtwEventCallback
func ddEtwEventCallback(eventRecord C.PEVENT_RECORD) {
	handle := cgo.Handle(C.DDGetEventContext(eventRecord))
	ctx, ok := handle.Value().(*processContext)
	if !ok {
		return
	}

	providerID := guidFromEventRecord(eventRecord)
	eventID := uint16(eventRecord.EventHeader.EventDescriptor.Id)

	if ctx.filter != nil && !ctx.filter(providerID, eventID) {
		return
	}

	event := &Event{
		ProviderID:  providerID,
		EventID:     eventID,
		Timestamp:   fileTimeToGo(C.LONGLONG(*(*int64)(unsafe.Pointer(&eventRecord.EventHeader.TimeStamp)))),
		eventRecord: eventRecord,
	}
	ctx.callback(event)
}

func guidFromEventRecord(r C.PEVENT_RECORD) windows.GUID {
	g := r.EventHeader.ProviderId
	return guidFromC(g)
}
