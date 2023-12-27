// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
// Package etw provides ETW tracing facilities to other components

//go:build windows

// Package etw provides an ETW tracing interface
package etw

import (
	"golang.org/x/sys/windows"
)

// team: windows-agent

// DDGUID represents a GUID
// 16 bytes
type DDGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]uint8
}

// DDEventDescriptor contains information (metadata) about an ETW event.
// see https://learn.microsoft.com/en-us/windows/win32/api/evntprov/ns-evntprov-event_descriptor
// 16 bytes
type DDEventDescriptor struct {
	ID      uint16
	Version uint8
	Channel uint8
	Level   uint8
	Opcode  uint8
	Task    uint16
	Keyword uint64
}

// DDEventHeader defines information about an ETW event.
// see https://learn.microsoft.com/en-us/windows/win32/api/evntcons/ns-evntcons-event_header
// 80 bytes
type DDEventHeader struct {
	Size            uint16
	HeaderType      uint16
	Flags           uint16
	EventProperty   uint16
	ThreadID        uint32
	ProcessID       uint32
	TimeStamp       uint64
	ProviderID      DDGUID
	EventDescriptor DDEventDescriptor
	Pad             [8]uint8
	ActivityID      DDGUID
}

// DDEventRecord defines the layout of an event that Event Tracing for Windows (ETW) delivers.
// see https://learn.microsoft.com/en-us/windows/win32/api/evntcons/ns-evntcons-event_record
// 104
type DDEventRecord struct {
	EventHeader    DDEventHeader
	Pad1           [6]uint8 // sizeof(ETW_BUFFER_CONTEXT) + sizeof(USHORT) = 6
	UserDataLength uint16
	Pad2           [8]uint8 // sizeof(PEVENT_HEADER_EXTENDED_DATA_ITEM) = 8
	UserData       *uint8
	// UserDataContext is left out
}

// EventCallback is a function that will be called when an ETW event is received
type EventCallback func(e *DDEventRecord)

// TraceLevel A value that indicates the maximum level of events that you want the provider to write.
// The provider typically writes an event if the event's level is less than or equal to this value,
// in addition to meeting the MatchAnyKeyword and MatchAllKeyword criteria.
// See https://learn.microsoft.com/en-us/windows/win32/api/evntrace/nf-evntrace-enabletraceex2
type TraceLevel uint8

//revive:disable:var-naming Keep the Microsoft naming-style
const (
	TRACE_LEVEL_CRITICAL    = TraceLevel(1)
	TRACE_LEVEL_ERROR       = TraceLevel(2)
	TRACE_LEVEL_WARNING     = TraceLevel(3)
	TRACE_LEVEL_INFORMATION = TraceLevel(4)
	TRACE_LEVEL_VERBOSE     = TraceLevel(5)
)

//revive:enable:var-naming

// ProviderConfiguration is a structure containing all the configuration options for an ETW provider
type ProviderConfiguration struct {
	// TraceLevel is a value that indicates the maximum level of events that you want the provider to write.
	// The provider typically writes an event if the event's level is less than or equal to this value.
	// See https://learn.microsoft.com/en-us/windows/win32/api/evntrace/nf-evntrace-enabletraceex2
	TraceLevel TraceLevel

	// MatchAnyKeyword is a 64-bit bitmask of keywords that determine the categories of events that you want the
	// provider to write.
	// See https://learn.microsoft.com/en-us/windows/win32/api/evntrace/nf-evntrace-enabletraceex2
	MatchAnyKeyword uint64

	// MatchAllKeyword is a 64-bit bitmask of keywords that restricts the events that you want the provider to write.
	// See https://learn.microsoft.com/en-us/windows/win32/api/evntrace/nf-evntrace-enabletraceex2
	MatchAllKeyword uint64

	// PIDs allow filtering by PIDs if non-empty
	PIDs []uint32
}

// ProviderConfigurationFunc is a function used to configure a provider
type ProviderConfigurationFunc func(cfg *ProviderConfiguration)

// Session represents an ETW session. A session can have multiple tracing providers enabled.
type Session interface {
	// ConfigureProvider configures a particular ETW provider identified by its GUID for this session.
	// After calling this function, call EnableProvider to apply the configuration.
	ConfigureProvider(providerGUID windows.GUID, configurations ...ProviderConfigurationFunc)

	// EnableProvider enables the given provider. If ConfigureProvider was not called prior to calling this
	// function, then a default provider configuration is applied.
	EnableProvider(providerGUID windows.GUID) error

	// DisableProvider disables the given provider.
	DisableProvider(providerGUID windows.GUID) error

	// StartTracing starts tracing with the given callback.
	// This function blocks until StopTracing is called.
	StartTracing(callback EventCallback) error

	// StopTracing stops all tracing activities.
	// It's not possible to use the session anymore after a call to StopTracing.
	StopTracing() error
}

// Component offers a way to create ETW tracing sessions with a given name.
type Component interface {
	NewSession(sessionName string) (Session, error)
}
