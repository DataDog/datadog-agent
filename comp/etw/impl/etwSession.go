// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package etwimpl

import (
	"errors"
	"fmt"
	"runtime/cgo"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/etw"
	"golang.org/x/sys/windows"
)

/*
#include "session.h"
*/
import "C"

type etwSession struct {
	Name          string
	hSession      C.TRACEHANDLE
	hTraceHandle  C.TRACEHANDLE
	wellKnown     bool
	propertiesBuf []byte
	providers     map[windows.GUID]etw.ProviderConfiguration
	utf16name     []uint16
	sessionConfig etw.SessionConfiguration
}

func (e *etwSession) ConfigureProvider(providerGUID windows.GUID, configurations ...etw.ProviderConfigurationFunc) {
	if e.wellKnown {
		return
	}
	cfg := etw.ProviderConfiguration{}
	for _, configuration := range configurations {
		configuration(&cfg)
	}
	e.providers[providerGUID] = cfg
}

func (e *etwSession) EnableProvider(providerGUID windows.GUID) error {

	if e.wellKnown {
		return fmt.Errorf("cannot enable provider on well-known session")
	}
	if _, ok := e.providers[providerGUID]; !ok {
		// ConfigureProvider was not called prior, set the default configuration
		e.ConfigureProvider(providerGUID, nil)
	}

	cfg := e.providers[providerGUID]
	var pids *C.ULONG
	var pidCount C.ULONG
	if len(cfg.PIDs) > 0 {
		pids = (*C.ULONG)(unsafe.SliceData(cfg.PIDs))
		pidCount = C.ULONG(len(cfg.PIDs))
	}

	ret := windows.Errno(C.DDEnableTrace(
		e.hSession,
		(*C.GUID)(unsafe.Pointer(&providerGUID)),
		C.EVENT_CONTROL_CODE_ENABLE_PROVIDER,
		C.UCHAR(cfg.TraceLevel),
		C.ULONGLONG(cfg.MatchAnyKeyword),
		C.ULONGLONG(cfg.MatchAllKeyword),
		0,
		// We can't pass to C-code Go pointers containing themselves
		// Go pointers, so we have to list all event filter descriptors here
		// and re-assemble them in C-land using our helper DDEnableTrace.
		pids,
		pidCount,
	))

	if ret != windows.ERROR_SUCCESS {
		return fmt.Errorf("failed to enable tracing for provider %v: %v", providerGUID, ret)
	}
	return nil
}

func (e *etwSession) DisableProvider(providerGUID windows.GUID) error {
	ret := windows.Errno(C.EnableTraceEx2(
		e.hSession,
		(*C.GUID)(unsafe.Pointer(&providerGUID)),
		C.EVENT_CONTROL_CODE_DISABLE_PROVIDER,
		0,
		0,
		0,
		0,
		nil))

	if ret == windows.ERROR_MORE_DATA ||
		ret == windows.ERROR_NOT_FOUND ||
		ret == windows.ERROR_SUCCESS {
		return nil
	}
	return ret
}

//export ddEtwCallbackC
func ddEtwCallbackC(eventRecord C.PEVENT_RECORD) {
	handle := cgo.Handle(eventRecord.UserContext)
	eventInfo := (*etw.DDEventRecord)(unsafe.Pointer(eventRecord))
	handle.Value().(etw.EventCallback)(eventInfo)
}

func (e *etwSession) StartTracing(callback etw.EventCallback) error {
	handle := cgo.NewHandle(callback)
	defer handle.Delete()
	traceHandle := C.DDStartTracing(
		(C.LPWSTR)(unsafe.Pointer(&e.utf16name[0])),
		(C.uintptr_t)(handle),
	)
	if traceHandle == C.INVALID_PROCESSTRACE_HANDLE {
		return fmt.Errorf("failed to start tracing: %v", windows.GetLastError())
	}

	e.hTraceHandle = traceHandle
	ret := windows.Errno(C.ProcessTrace(
		C.PTRACEHANDLE(&traceHandle),
		1,
		nil,
		nil,
	))
	if ret == windows.ERROR_SUCCESS || ret == windows.ERROR_CANCELLED {
		return nil
	}
	return ret
}

func (e *etwSession) StopTracing() error {
	var globalError error
	for guid := range e.providers {
		// nil errors are discarded
		globalError = errors.Join(globalError, e.DisableProvider(guid))
	}
	ptp := (C.PEVENT_TRACE_PROPERTIES)(unsafe.Pointer(&e.propertiesBuf[0]))
	ret := windows.Errno(C.ControlTraceW(
		e.hSession,
		nil,
		ptp,
		C.EVENT_TRACE_CONTROL_STOP))
	if !(ret == windows.ERROR_MORE_DATA ||
		ret == windows.ERROR_SUCCESS) {
		return errors.Join(ret, globalError)
	}
	return globalError
}

func (e *etwSession) GetSessionStatistics() (etw.SessionStatistics, error) {
	var stats etw.SessionStatistics
	// it is not clear if we can safely reuse the properties buffer here
	// so we allocate a new one
	_, ptp := initializeRealtimeSessionProperties(e)

	ret := windows.Errno(C.ControlTraceW(
		e.hSession,
		nil,
		ptp,
		C.EVENT_TRACE_CONTROL_QUERY))
	if ret != windows.ERROR_SUCCESS {
		return stats, ret
	}

	stats.NumberOfBuffers = uint32(ptp.NumberOfBuffers)
	stats.FreeBuffers = uint32(ptp.FreeBuffers)
	stats.EventsLost = uint32(ptp.EventsLost)
	stats.BuffersWritten = uint32(ptp.BuffersWritten)
	stats.LogBuffersLost = uint32(ptp.LogBuffersLost)
	stats.RealTimeBuffersLost = uint32(ptp.RealTimeBuffersLost)
	return stats, nil
}

// deleteEtwSession deletes an ETW session by name, typically after a crash since we don't have access to the session
// handle anymore.
func deleteEtwSession(name string) error {
	utf16SessionName, err := windows.UTF16FromString(name)
	if err != nil {
		return fmt.Errorf("incorrect session name; %w", err)
	}
	sessionNameLength := len(utf16SessionName) * int(unsafe.Sizeof(utf16SessionName[0]))

	const maxLengthLogfileName = 1024
	bufSize := int(unsafe.Sizeof(C.EVENT_TRACE_PROPERTIES{})) + sessionNameLength + maxLengthLogfileName
	propertiesBuf := make([]byte, bufSize)
	pProperties := (C.PEVENT_TRACE_PROPERTIES)(unsafe.Pointer(&propertiesBuf[0]))
	pProperties.Wnode.BufferSize = C.ulong(bufSize)

	ret := windows.Errno(C.ControlTraceW(
		0,
		(*C.ushort)(unsafe.Pointer(&utf16SessionName[0])),
		pProperties,
		C.EVENT_TRACE_CONTROL_STOP))

	if ret == windows.ERROR_MORE_DATA ||
		ret == windows.ERROR_SUCCESS {
		return nil
	}
	return ret
}

func createEtwSession(name string, f etw.SessionConfigurationFunc) (*etwSession, error) {
	_ = deleteEtwSession(name)

	utf16SessionName, err := windows.UTF16FromString(name)
	s := &etwSession{
		Name:      name,
		wellKnown: false,
		utf16name: utf16SessionName,
		providers: make(map[windows.GUID]etw.ProviderConfiguration),
	}

	if err != nil {
		return nil, fmt.Errorf("incorrect session name; %w", err)
	}

	// get any caller supplied configuration
	if f != nil {
		f(&s.sessionConfig)
		if s.sessionConfig.MaxBuffers != 0 && s.sessionConfig.MaxBuffers < s.sessionConfig.MinBuffers {
			return nil, fmt.Errorf("max buffers must be greater than or equal to min buffers")
		}
	}

	propertiesBuf, pProperties := initializeRealtimeSessionProperties(s)

	ret := windows.Errno(C.StartTraceW(
		&s.hSession,
		C.LPWSTR(unsafe.Pointer(&s.utf16name[0])),
		pProperties,
	))

	// Should never happen given we start by deleting any session with the same name
	if ret == windows.ERROR_ALREADY_EXISTS {
		return nil, fmt.Errorf("session %s already exists; %w", s.Name, err)
	}

	if ret == windows.ERROR_SUCCESS {
		s.propertiesBuf = propertiesBuf
		return s, nil
	}

	return nil, fmt.Errorf("StartTraceW failed; %w", err)
}

func createWellKnownEtwSession(name string, f etw.SessionConfigurationFunc) (*etwSession, error) {
	utf16SessionName, err := windows.UTF16FromString(name)
	if err != nil {
		return nil, fmt.Errorf("incorrect session name; %w", err)
	}

	s := &etwSession{
		Name:      name,
		utf16name: utf16SessionName,
		wellKnown: true,
	}

	// get any caller supplied configuration
	if f != nil {
		f(&s.sessionConfig)
		if s.sessionConfig.MaxBuffers != 0 && s.sessionConfig.MaxBuffers < s.sessionConfig.MinBuffers {
			return nil, fmt.Errorf("max buffers must be greater than or equal to min buffers")
		}
	}

	s.propertiesBuf, _ = initializeRealtimeSessionProperties(s)
	return s, nil
}

func initializeRealtimeSessionProperties(s *etwSession) ([]byte, C.PEVENT_TRACE_PROPERTIES) {
	sessionNameSize := (len(s.utf16name) * int(unsafe.Sizeof(s.utf16name[0])))
	bufSize := int(unsafe.Sizeof(C.EVENT_TRACE_PROPERTIES{})) + sessionNameSize
	propertiesBuf := make([]byte, bufSize)

	pProperties := (C.PEVENT_TRACE_PROPERTIES)(unsafe.Pointer(&propertiesBuf[0]))
	pProperties.Wnode.BufferSize = C.ulong(bufSize)
	pProperties.Wnode.ClientContext = 1
	pProperties.Wnode.Flags = C.WNODE_FLAG_TRACED_GUID

	pProperties.LogFileMode = C.EVENT_TRACE_REAL_TIME_MODE
	if s.sessionConfig.MaxBuffers > 0 {
		pProperties.MaximumBuffers = C.ulong(s.sessionConfig.MaxBuffers)
	}
	if s.sessionConfig.MinBuffers > 0 {
		pProperties.MinimumBuffers = C.ulong(s.sessionConfig.MinBuffers)
	}
	return propertiesBuf, pProperties
}
