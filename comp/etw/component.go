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

// DDEtwBufferContext defines the go definition of the C structure ETW_BUFFER_CONTEXT
type DDEtwBufferContext struct {
	/*
			the actual struct is defined as
			 union {
		    struct {
		      UCHAR ProcessorNumber;
		      UCHAR Alignment;
		    } DUMMYSTRUCTNAME;
		    USHORT ProcessorIndex;
		  } DUMMYUNIONNAME;
		  USHORT LoggerId;
		  but go doesn't really do unions.  So for now, just put in ProcessorIndex
	*/
	ProcessorIndex uint16
	LoggerID       uint16
}

// DDEventHeaderExtendedDataItem defines the layout of the extended data for an event that Event Tracing for Windows (ETW) delivers.
// see https://learn.microsoft.com/en-us/windows/win32/api/evntcons/ns-evntcons-event_header_extended_data_item
type DDEventHeaderExtendedDataItem struct {
	Reserved1 uint16
	ExtType   uint16
	Reserved2 uint16 // is actually 1 bit of linkage and 15 bits of reserved
	DataSize  uint16
	DataPtr   *uint8
}

// DDEventRecord defines the layout of an event that Event Tracing for Windows (ETW) delivers.
// see https://learn.microsoft.com/en-us/windows/win32/api/evntcons/ns-evntcons-event_record
// 104
type DDEventRecord struct {
	EventHeader       DDEventHeader
	BufferContext     DDEtwBufferContext
	ExtendedDataCount uint16
	UserDataLength    uint16
	ExtendedData      *DDEventHeaderExtendedDataItem // this is actually a pointer to the extended data.
	UserData          *uint8
	UserContext       *uint8
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

//revive:disable:exported
const (
	EVENT_HEADER_EXT_TYPE_RELATED_ACTIVITYID = 0x0001
	EVENT_HEADER_EXT_TYPE_SID                = 0x0002
	EVENT_HEADER_EXT_TYPE_TS_ID              = 0x0003
	EVENT_HEADER_EXT_TYPE_INSTANCE_INFO      = 0x0004
	EVENT_HEADER_EXT_TYPE_STACK_TRACE32      = 0x0005
	EVENT_HEADER_EXT_TYPE_STACK_TRACE64      = 0x0006
	EVENT_HEADER_EXT_TYPE_PEBS_INDEX         = 0x0007
	EVENT_HEADER_EXT_TYPE_PMC_COUNTERS       = 0x0008
	EVENT_HEADER_EXT_TYPE_PSM_KEY            = 0x0009
	EVENT_HEADER_EXT_TYPE_EVENT_KEY          = 0x000A
	EVENT_HEADER_EXT_TYPE_EVENT_SCHEMA_TL    = 0x000B
	EVENT_HEADER_EXT_TYPE_PROV_TRAITS        = 0x000C
	EVENT_HEADER_EXT_TYPE_PROCESS_START_KEY  = 0x000D
	EVENT_HEADER_EXT_TYPE_CONTROL_GUID       = 0x000E
	EVENT_HEADER_EXT_TYPE_QPC_DELTA          = 0x000F
	EVENT_HEADER_EXT_TYPE_CONTAINER_ID       = 0x0010
	EVENT_HEADER_EXT_TYPE_MAX                = 0x0011
)

//revive:enable:exported
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

	// The EnabledIDs and DisabledIDs fields are mutually exclusive; the API allows you to send one or the other but not both.
	//
	// Setting the list of EnabledIDs will enable only listed events.
	// Setting the list of DisabledIDs will disable only listed events, and allow all others

	// EventIDs for enableTrace = TRUE
	EnabledIDs []uint16

	// eventIds for enabletrace = FALSE
	DisabledIDs []uint16
}

// ProviderConfigurationFunc is a function used to configure a provider
type ProviderConfigurationFunc func(cfg *ProviderConfiguration)

// SessionConfiguration is a structure containing all the configuration options for an ETW session
type SessionConfiguration struct {
	//MinBuffers is the minimum number of trace buffers for ETW to allocate for a session.  The default is 0
	MinBuffers uint32
	//MaxBuffers is the maximum number of buffers for ETW to allocate.  The default is 0
	MaxBuffers uint32
}

// SessionStatistics contains statistics about the session
type SessionStatistics struct {
	// NumberOfBuffers is the number of buffers allocated for the session
	NumberOfBuffers uint32
	// FreeBuffers is the number of buffers that are free
	FreeBuffers uint32
	// EventsLost is the number of events not recorded
	EventsLost uint32
	// BuffersWritten is the number of buffers written
	BuffersWritten uint32
	// LogBuffersLost is the number of log buffers lost
	LogBuffersLost uint32
	// RealTimeBuffersLost is the number of real-time buffers lost
	RealTimeBuffersLost uint32
}

// SessionConfigurationFunc is a function used to configure a session
type SessionConfigurationFunc func(cfg *SessionConfiguration)

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

	// GetSessionStatistics returns statistics about the session
	GetSessionStatistics() (SessionStatistics, error)
}

// Component offers a way to create ETW tracing sessions with a given name.
type Component interface {
	NewSession(sessionName string, f SessionConfigurationFunc) (Session, error)
	NewWellKnownSession(sessionName string, f SessionConfigurationFunc) (Session, error)
}

// UserData offers a wrapper around the UserData field of an ETW event.
type UserData interface {
	// ParseUnicodeString reads from the userdata field, and returns the string
	// the next offset in the buffer of the next field, whether the value was found,
	// and if a terminating null was found, the index of that
	ParseUnicodeString(offset int) (string, int, bool, int)

	// GetUint64 reads a uint64 from the userdata field
	GetUint64(offset int) uint64

	// GetUint32 reads a uint32 from the userdata field
	GetUint32(offset int) uint32

	// GetUint16 reads a uint16 from the userdata field
	GetUint16(offset int) uint16

	// Bytes returns the selected slice of bytes
	Bytes(offset int, length int) []byte
	// Length returns the length of the userdata field
	Length() int
}
