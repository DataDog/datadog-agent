// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows
// +build windows

package evtapi

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// EVT_SUBSCRIBE_FLAGS
	// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_subscribe_flags
	EvtSubscribeToFutureEvents      = 1
	EvtSubscribeStartAtOldestRecord = 2
	EvtSubscribeStartAfterBookmark  = 3
	EvtSubscribeOriginMask          = 3
	EvtSubscribeTolerateQueryErrors = 0x1000
	EvtSubscribeStrict              = 0x10000
)

const (
	// EVT_RENDER_CONTEXT_FLAGS
	// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_render_context_flags
	EvtRenderContextValues = iota
	EvtRenderContextSystem
	EvtRenderContextUser
)

const (
	// EVT_RENDER_FLAGS
	// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_render_flags
	EvtRenderEventValues = iota
	EvtRenderEventXml
	EvtRenderBookmark
)

const (
	// EVT_VARIANT_TYPE
	// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_variant_type
	EvtVarTypeNull       = 0
	EvtVarTypeString     = 1
	EvtVarTypeAnsiString = 2
	EvtVarTypeSByte      = 3
	EvtVarTypeByte       = 4
	EvtVarTypeInt16      = 5
	EvtVarTypeUInt16     = 6
	EvtVarTypeInt32      = 7
	EvtVarTypeUInt32     = 8
	EvtVarTypeInt64      = 9
	EvtVarTypeUInt64     = 10
	EvtVarTypeSingle     = 11
	EvtVarTypeDouble     = 12
	EvtVarTypeBoolean    = 13
	EvtVarTypeBinary     = 14
	EvtVarTypeGuid       = 15
	EvtVarTypeSizeT      = 16
	EvtVarTypeFileTime   = 17
	EvtVarTypeSysTime    = 18
	EvtVarTypeSid        = 19
	EvtVarTypeHexInt32   = 20
	EvtVarTypeHexInt64   = 21
	EvtVarTypeEvtHandle  = 32
	EvtVarTypeEvtXml     = 35
)

const (
	// EVT_SYSTEM_PROPERTY_ID
	// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_system_property_id
	EvtSystemProviderName = iota
	EvtSystemProviderGuid
	EvtSystemEventID
	EvtSystemQualifiers
	EvtSystemLevel
	EvtSystemTask
	EvtSystemOpcode
	EvtSystemKeywords
	EvtSystemTimeCreated
	EvtSystemEventRecordId
	EvtSystemActivityID
	EvtSystemRelatedActivityID
	EvtSystemProcessID
	EvtSystemThreadID
	EvtSystemChannel
	EvtSystemComputer
	EvtSystemUserID
	EvtSystemVersion
	EvtSystemPropertyIdEND
)

const (
	// EVT_FORMAT_MESSAGE_FLAGS
	// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_format_message_flags
	EvtFormatMessageEvent = iota + 1
	EvtFormatMessageLevel
	EvtFormatMessageTask
	EvtFormatMessageOpcode
	EvtFormatMessageKeyword
	EvtFormatMessageChannel
	EvtFormatMessageProvider
	EvtFormatMessageId
	EvtFormatMessageXml
)

// Returned from EvtQuery and EvtSubscribe
type EventResultSetHandle windows.Handle

// Returned from EvtNext
type EventRecordHandle windows.Handle

// Returned from EvtCreateBookmark
type EventBookmarkHandle windows.Handle

// Returned from EvtCreateRenderContext
type EventRenderContextHandle windows.Handle

// Returned from EvtOpenPublisherMetadata
type EventPublisherMetadataHandle windows.Handle

// Returned from RegisterEventSource
type EventSourceHandle windows.Handle

// Returned from CreateEvent
type WaitEventHandle windows.Handle

type API interface {
	// Windows Event Log API methods
	EvtSubscribe(
		SignalEvent WaitEventHandle,
		ChannelPath string,
		Query string,
		Bookmark EventBookmarkHandle,
		Flags uint) (EventResultSetHandle, error)

	EvtNext(
		Session EventResultSetHandle,
		EventsArray []EventRecordHandle,
		EventsSize uint,
		Timeout uint) ([]EventRecordHandle, error)

	EvtClose(h windows.Handle)

	EvtRenderEventXml(Fragment EventRecordHandle) ([]uint16, error)

	EvtRenderBookmark(Fragment EventBookmarkHandle) ([]uint16, error)

	EvtCreateRenderContext(ValuePaths []string, Flags uint) (EventRenderContextHandle, error)

	// Note: Must call .Close() on the return value when done using it
	EvtRenderEventValues(Context EventRenderContextHandle, Fragment EventRecordHandle) (EvtVariantValues, error)

	EvtCreateBookmark(BookmarkXml string) (EventBookmarkHandle, error)

	EvtUpdateBookmark(Bookmark EventBookmarkHandle, Event EventRecordHandle) error

	EvtOpenPublisherMetadata(
		PublisherId string,
		LogFilePath string) (EventPublisherMetadataHandle, error)

	EvtFormatMessage(
		PublisherMetadata EventPublisherMetadataHandle,
		Event EventRecordHandle,
		MessageId uint,
		Values EvtVariantValues,
		Flags uint) (string, error)

	// Windows Event Logging methods
	RegisterEventSource(SourceName string) (EventSourceHandle, error)

	DeregisterEventSource(EventLog EventSourceHandle) error

	EvtClearLog(ChannelPath string) error

	ReportEvent(
		EventLog EventSourceHandle,
		Type uint,
		Category uint,
		EventID uint,
		Strings []string,
		RawData []uint8) error
}

type EventRecord struct {
	EventRecordHandle EventRecordHandle
}

// EvtVariantValues is returned from EvtRenderEventValues
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ns-winevt-evt_variant
type EvtVariantValues interface {
	// Each type method accepts an index argument that determines which element in the
	// array to return.
	String(uint) (string, error)

	UInt(uint) (uint64, error)

	// Returns unix timestamp in seconds
	Time(uint) (int64, error)

	// Returns the EVT_VARIANT_TYPE of the element at index
	Type(uint) (uint, error)

	// Buffer to raw EVT_VARIANT buffer
	Buffer() unsafe.Pointer

	// The number of values
	Count() uint

	// Free resources
	Close()
}

// Helpful wrappers for custom types
func EvtCloseResultSet(api API, h EventResultSetHandle) {
	api.EvtClose(windows.Handle(h))
}

func EvtCloseBookmark(api API, h EventBookmarkHandle) {
	api.EvtClose(windows.Handle(h))
}

func EvtCloseRecord(api API, h EventRecordHandle) {
	api.EvtClose(windows.Handle(h))
}

func EvtCloseRenderContext(api API, h EventRenderContextHandle) {
	api.EvtClose(windows.Handle(h))
}

func EvtClosePublisherMetadata(api API, h EventPublisherMetadataHandle) {
	api.EvtClose(windows.Handle(h))
}
