// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package etwimpl

import "C"
import (
	"fmt"
	"github.com/DataDog/datadog-agent/comp/etw"
	"golang.org/x/sys/windows"
	"unsafe"
)

/*
#include "session.h"
*/
import "C"

type etwSession struct {
	Name          string
	hSession      C.TRACEHANDLE
	propertiesBuf []byte
	providers     map[windows.GUID]etw.ProviderConfiguration
}

func (e *etwSession) ConfigureProvider(providerGUID windows.GUID, configurations ...etw.ProviderConfigurationFunc) error {
	cfg := etw.ProviderConfiguration{}
	for _, configuration := range configurations {
		configuration(&cfg)
	}
	e.providers[providerGUID] = cfg
	return nil
}

func (e *etwSession) StartTracing(providerGUID windows.GUID, callback etw.EventCallback) error {
	return nil
}

func (e *etwSession) StopTracing() error {
	return nil
}

func CreateEtwSession(name string) (etw.Session, error) {
	s := &etwSession{
		Name:      name,
		providers: make(map[windows.GUID]etw.ProviderConfiguration),
	}

	utf16SessionName, err := windows.UTF16FromString(name)
	if err != nil {
		return nil, fmt.Errorf("incorrect session name; %w", err)
	}
	sessionNameSize := len(utf16SessionName) * int(unsafe.Sizeof(utf16SessionName))
	bufSize := int(unsafe.Sizeof(C.EVENT_TRACE_PROPERTIES{})) + sessionNameSize
	propertiesBuf := make([]byte, bufSize)

	pProperties := (C.PEVENT_TRACE_PROPERTIES)(unsafe.Pointer(&propertiesBuf[0]))
	pProperties.Wnode.BufferSize = C.ulong(bufSize)
	pProperties.Wnode.ClientContext = 1
	pProperties.Wnode.Flags = C.WNODE_FLAG_TRACED_GUID

	pProperties.LogFileMode = C.EVENT_TRACE_REAL_TIME_MODE

	ret := C.StartTraceW(
		&s.hSession,
		C.LPWSTR(unsafe.Pointer(&utf16SessionName[0])),
		pProperties,
	)
	switch err := windows.Errno(ret); err {
	case windows.ERROR_ALREADY_EXISTS:
		return nil, fmt.Errorf("session %s already exists; %w", s.Name, err)
	case windows.ERROR_SUCCESS:
		s.propertiesBuf = propertiesBuf
		return s, nil
	default:
		return nil, fmt.Errorf("StartTraceW failed; %w", err)
	}
}
