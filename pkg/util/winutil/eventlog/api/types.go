// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows

// Package evtapi defines the interface and common types for interacting with the Windows Event Log API from Golang
package evtapi

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

//revive:disable:var-naming These names are intended to match the Windows API names

// EVT_SUBSCRIBE_FLAGS
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_subscribe_flags
const (
	EvtSubscribeToFutureEvents      = 1
	EvtSubscribeStartAtOldestRecord = 2
	EvtSubscribeStartAfterBookmark  = 3
	EvtSubscribeOriginMask          = 3
	EvtSubscribeTolerateQueryErrors = 0x1000
	EvtSubscribeStrict              = 0x10000
)

// EVT_RENDER_CONTEXT_FLAGS
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_render_context_flags
const (
	EvtRenderContextValues = iota
	EvtRenderContextSystem
	EvtRenderContextUser
)

// EVT_RENDER_FLAGS
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_render_flags
const (
	EvtRenderEventValues = iota
	EvtRenderEventXml
	EvtRenderBookmark
)

// EVT_VARIANT_TYPE
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_variant_type
const (
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

// EVT_SYSTEM_PROPERTY_ID
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_system_property_id
const (
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

// EVT_FORMAT_MESSAGE_FLAGS
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_format_message_flags
const (
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

// EVT_RPC_LOGIN_FLAGS
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_rpc_login_flags
const (
	EvtRpcLoginAuthDefault = iota
	EvtRpcLoginAuthNegotiate
	EvtRpcLoginAuthKerberos
	EvtRpcLoginAuthNTLM
)

//revive:enable:var-naming

// EventSessionHandle is a typed windows.Handle returned from EvtOpenSession
type EventSessionHandle windows.Handle

// EventResultSetHandle is a typed windows.Handle returned from EvtQuery and EvtSubscribe
type EventResultSetHandle windows.Handle

// EventRecordHandle is a typed windows.Handle returned from EvtNext
type EventRecordHandle windows.Handle

// EventBookmarkHandle is a typed windows.Handle returned from EvtCreateBookmark
type EventBookmarkHandle windows.Handle

// EventRenderContextHandle is a typed windows.Handle returned from EvtCreateRenderContext
type EventRenderContextHandle windows.Handle

// EventPublisherMetadataHandle is a typed windows.Handle returned from EvtOpenPublisherMetadata
type EventPublisherMetadataHandle windows.Handle

// EventSourceHandle is a typed windows.Handle returned from RegisterEventSource
type EventSourceHandle windows.Handle

// WaitEventHandle is a typed windows.Handle returned from CreateEvent
type WaitEventHandle windows.Handle

// API is an interface for Windows Event Log API methods
// https://learn.microsoft.com/en-us/windows/win32/wes/windows-event-log-functions
type API interface {
	EvtSubscribe(
		Session EventSessionHandle,
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

	EvtCreateBookmark(BookmarkXML string) (EventBookmarkHandle, error)

	EvtUpdateBookmark(Bookmark EventBookmarkHandle, Event EventRecordHandle) error

	EvtOpenPublisherMetadata(
		PublisherID string,
		LogFilePath string) (EventPublisherMetadataHandle, error)

	EvtFormatMessage(
		PublisherMetadata EventPublisherMetadataHandle,
		Event EventRecordHandle,
		MessageID uint,
		Values EvtVariantValues,
		Flags uint) (string, error)

	EvtOpenSession(
		Server string,
		User string,
		Domain string,
		Password string,
		Flags uint,
	) (EventSessionHandle, error)

	// Windows Event Logging methods
	RegisterEventSource(SourceName string) (EventSourceHandle, error)

	DeregisterEventSource(EventLog EventSourceHandle) error

	EvtClearLog(ChannelPath string) error

	ReportEvent(
		EventLog EventSourceHandle,
		Type uint,
		Category uint,
		EventID uint,
		UserSID *windows.SID,
		Strings []string,
		RawData []uint8) error
}

// EventRecord is a light wrapper around EventRecordHandle for now.
// In the future it may contain other fields to assist in event rendering.
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

	// Returns a SID
	SID(uint) (*windows.SID, error)

	// Returns the EVT_VARIANT_TYPE of the element at index
	Type(uint) (uint, error)

	// Buffer to raw EVT_VARIANT buffer
	Buffer() unsafe.Pointer

	// The number of values
	Count() uint

	// Free resources
	Close()
}

//
// Helpful wrappers for custom types
//

// EvtCloseSession closes EventSessionHandle
func EvtCloseSession(api API, h EventSessionHandle) {
	api.EvtClose(windows.Handle(h))
}

// EvtCloseResultSet closes EventResultSetHandle
func EvtCloseResultSet(api API, h EventResultSetHandle) {
	api.EvtClose(windows.Handle(h))
}

// EvtCloseBookmark closes EventBookmarkHandle
func EvtCloseBookmark(api API, h EventBookmarkHandle) {
	api.EvtClose(windows.Handle(h))
}

// EvtCloseRecord closes EventRecordHandle
func EvtCloseRecord(api API, h EventRecordHandle) {
	api.EvtClose(windows.Handle(h))
}

// EvtCloseRenderContext closes EventRenderContextHandle
func EvtCloseRenderContext(api API, h EventRenderContextHandle) {
	api.EvtClose(windows.Handle(h))
}

// EvtClosePublisherMetadata closes EventPublisherMetadataHandle
func EvtClosePublisherMetadata(api API, h EventPublisherMetadataHandle) {
	api.EvtClose(windows.Handle(h))
}
