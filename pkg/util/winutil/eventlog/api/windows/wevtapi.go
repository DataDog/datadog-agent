// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package winevtapi implements the evtapi.API interface with the Windows Event Log API
package winevtapi

import (
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"

	"golang.org/x/sys/windows"
)

var (
	// New Event Log API
	// https://learn.microsoft.com/en-us/windows/win32/wes/using-windows-event-log
	wevtapi                  = windows.NewLazySystemDLL("wevtapi.dll")
	evtSubscribe             = wevtapi.NewProc("EvtSubscribe")
	evtClose                 = wevtapi.NewProc("EvtClose")
	evtNext                  = wevtapi.NewProc("EvtNext")
	evtCreateBookmark        = wevtapi.NewProc("EvtCreateBookmark")
	evtUpdateBookmark        = wevtapi.NewProc("EvtUpdateBookmark")
	evtCreateRenderContext   = wevtapi.NewProc("EvtCreateRenderContext")
	evtRender                = wevtapi.NewProc("EvtRender")
	evtClearLog              = wevtapi.NewProc("EvtClearLog")
	evtOpenPublisherMetadata = wevtapi.NewProc("EvtOpenPublisherMetadata")
	evtFormatMessage         = wevtapi.NewProc("EvtFormatMessage")
	evtOpenSession           = wevtapi.NewProc("EvtOpenSession")

	// Legacy Event Logging API
	// https://learn.microsoft.com/en-us/windows/win32/eventlog/using-event-logging
	advapi32              = windows.NewLazySystemDLL("advapi32.dll")
	registerEventSource   = advapi32.NewProc("RegisterEventSourceW")
	deregisterEventSource = advapi32.NewProc("DeregisterEventSource")
	reportEvent           = advapi32.NewProc("ReportEventW")
)

// API implements Golang wrappers for Windows Event Log API methods
// https://learn.microsoft.com/en-us/windows/win32/wes/windows-event-log-functions
type API struct{}

// New returns a new Windows Event Log API
func New() *API {
	var api API
	return &api
}

// EvtSubscribe wrapper.
// Must pass the returned handle to EvtClose when finished using the handle.
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtsubscribe
func (api *API) EvtSubscribe(
	Session evtapi.EventSessionHandle,
	SignalEvent evtapi.WaitEventHandle,
	ChannelPath string,
	Query string,
	Bookmark evtapi.EventBookmarkHandle,
	Flags uint) (evtapi.EventResultSetHandle, error) {

	// Convert Go string to Windows API string
	channelPath, err := winutil.UTF16PtrOrNilFromString(ChannelPath)
	if err != nil {
		return evtapi.EventResultSetHandle(0), err
	}
	query, err := winutil.UTF16PtrOrNilFromString(Query)
	if err != nil {
		return evtapi.EventResultSetHandle(0), err
	}

	// Call API
	r1, _, lastErr := evtSubscribe.Call(
		uintptr(Session),
		uintptr(SignalEvent),
		uintptr(unsafe.Pointer(channelPath)),
		uintptr(unsafe.Pointer(query)),
		uintptr(Bookmark),
		uintptr(0), // No context in pull mode
		uintptr(0), // No callback in pull mode
		uintptr(Flags))
	// EvtSubscribe returns NULL on error
	if r1 == 0 {
		return evtapi.EventResultSetHandle(0), lastErr
	}

	return evtapi.EventResultSetHandle(r1), nil
}

// EvtNext wrapper.
// Must pass on every handle returned to EvtClose when finished using the handle.
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtnext
func (api *API) EvtNext(
	Session evtapi.EventResultSetHandle,
	EventsArray []evtapi.EventRecordHandle,
	EventsSize uint,
	Timeout uint) ([]evtapi.EventRecordHandle, error) {

	var Returned uint32

	if len(EventsArray) == 0 {
		return nil, fmt.Errorf("input EventsArray is empty")
	}

	// Fill array
	r1, _, lastErr := evtNext.Call(
		uintptr(Session),
		uintptr(EventsSize),
		uintptr(unsafe.Pointer(unsafe.SliceData(EventsArray))),
		uintptr(Timeout),
		uintptr(0), // reserved must be 0
		uintptr(unsafe.Pointer(&Returned)))
	// EvtNext returns C BOOL FALSE (0) on "error"
	// "error" can mean error, ERROR_TIMEOUT, or ERROR_NO_MORE_ITEMS
	if r1 == 0 {
		return nil, lastErr
	}

	// Trim slice over returned # elements
	return EventsArray[:Returned], nil
}

// EvtClose wrapper
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtclose
func (api *API) EvtClose(h windows.Handle) {
	if h != windows.Handle(0) {
		_, _, _ = evtClose.Call(uintptr(h))
	}
}

// EvtCreateBookmark wrapper
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtcreatebookmark
func (api *API) EvtCreateBookmark(BookmarkXML string) (evtapi.EventBookmarkHandle, error) {
	var bookmarkXML *uint16

	bookmarkXML, err := winutil.UTF16PtrOrNilFromString(BookmarkXML)
	if err != nil {
		return evtapi.EventBookmarkHandle(0), err
	}

	r1, _, lastErr := evtCreateBookmark.Call(uintptr(unsafe.Pointer(bookmarkXML)))
	// EvtCreateBookmark returns NULL on error
	if r1 == 0 {
		return evtapi.EventBookmarkHandle(0), lastErr
	}

	return evtapi.EventBookmarkHandle(r1), nil
}

// EvtUpdateBookmark wrapper
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtupdatebookmark
func (api *API) EvtUpdateBookmark(Bookmark evtapi.EventBookmarkHandle, Event evtapi.EventRecordHandle) error {
	r1, _, lastErr := evtUpdateBookmark.Call(uintptr(Bookmark), uintptr(Event))
	// EvtUpdateBookmark returns C FALSE (0) on error
	if r1 == 0 {
		return lastErr
	}

	return nil
}

// EvtCreateRenderContext wrapper
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtcreaterendercontext
func (api *API) EvtCreateRenderContext(ValuePaths []string, Flags uint) (evtapi.EventRenderContextHandle, error) {
	var err error

	var valuePathsPtr unsafe.Pointer

	if len(ValuePaths) > 0 {
		valuePaths := make([]*uint16, len(ValuePaths))
		for i, v := range ValuePaths {
			valuePaths[i], err = windows.UTF16PtrFromString(v)
			if err != nil {
				return evtapi.EventRenderContextHandle(0), err
			}
		}
		valuePathsPtr = unsafe.Pointer(unsafe.SliceData(valuePaths))
	} else {
		valuePathsPtr = nil
	}

	r1, _, lastErr := evtCreateRenderContext.Call(
		uintptr(len(ValuePaths)),
		uintptr(valuePathsPtr),
		uintptr(Flags))
	// EvtCreateRenderContext returns NULL on error
	if r1 == 0 {
		return evtapi.EventRenderContextHandle(0), lastErr
	}

	return evtapi.EventRenderContextHandle(r1), nil
}

// EvtRenderText supports the EvtRenderEventXml and EvtRenderBookmark Flags
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtrender
func evtRenderText(
	Fragment windows.Handle,
	Flags uint) ([]uint16, error) {

	if Flags != evtapi.EvtRenderEventXml && Flags != evtapi.EvtRenderBookmark {
		return nil, fmt.Errorf("Invalid Flags")
	}

	// Get required buffer size
	var BufferUsed uint32
	var PropertyCount uint32
	r1, _, lastErr := evtRender.Call(
		uintptr(0),
		uintptr(Fragment),
		uintptr(Flags),
		uintptr(0),
		uintptr(0),
		uintptr(unsafe.Pointer(&BufferUsed)),
		uintptr(unsafe.Pointer(&PropertyCount)))
	// EvtRenders returns C FALSE (0) on error
	if r1 == 0 {
		if lastErr != windows.ERROR_INSUFFICIENT_BUFFER {
			return nil, lastErr
		}
	} else {
		return nil, nil
	}

	if BufferUsed == 0 {
		return nil, nil
	}

	// Allocate buffer space (BufferUsed is size in bytes)
	Buffer := make([]uint16, BufferUsed/2)

	// Fill buffer
	r1, _, lastErr = evtRender.Call(
		uintptr(0),
		uintptr(Fragment),
		uintptr(Flags),
		uintptr(BufferUsed),
		uintptr(unsafe.Pointer(unsafe.SliceData(Buffer))),
		uintptr(unsafe.Pointer(&BufferUsed)),
		uintptr(unsafe.Pointer(&PropertyCount)))
	// EvtRenders returns C FALSE (0) on error
	if r1 == 0 {
		return nil, lastErr
	}

	// Trim buffer to size (BufferUsed is size in bytes)
	return Buffer[:BufferUsed/2], nil
}

// EvtRenderEventXml wraps EvtRender with EvtRenderEventXml
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtrender
//
//revive:disable-next-line:var-naming Name is intended to match the Windows API name
func (api *API) EvtRenderEventXml(Fragment evtapi.EventRecordHandle) ([]uint16, error) {
	return evtRenderText(windows.Handle(Fragment), evtapi.EvtRenderEventXml)
}

// EvtRenderBookmark wraps EvtRender with EvtRenderBookmark
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtrender
func (api *API) EvtRenderBookmark(Fragment evtapi.EventBookmarkHandle) ([]uint16, error) {
	return evtRenderText(windows.Handle(Fragment), evtapi.EvtRenderBookmark)
}

// RegisterEventSource wrapper
// https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-registereventsourcew
func (api *API) RegisterEventSource(SourceName string) (evtapi.EventSourceHandle, error) {
	sourceName, err := winutil.UTF16PtrOrNilFromString(SourceName)
	if err != nil {
		return evtapi.EventSourceHandle(0), err
	}

	r1, _, lastErr := registerEventSource.Call(
		uintptr(0), // local computer only
		uintptr(unsafe.Pointer(sourceName)))
	// RegisterEventSource returns NULL on error
	if r1 == 0 {
		return evtapi.EventSourceHandle(0), lastErr
	}

	return evtapi.EventSourceHandle(r1), nil
}

// DeregisterEventSource wrapper
// https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-deregistereventsource
func (api *API) DeregisterEventSource(EventLog evtapi.EventSourceHandle) error {
	r1, _, lastErr := deregisterEventSource.Call(uintptr(EventLog))
	// DeregisterEventSource returns C FALSE (0) on error
	if r1 == 0 {
		return lastErr
	}

	return nil
}

// ReportEvent wrapper
// https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-reporteventw
func (api *API) ReportEvent(
	EventLog evtapi.EventSourceHandle,
	Type uint,
	Category uint,
	EventID uint,
	UserSID *windows.SID,
	Strings []string,
	RawData []uint8) error {

	var err error
	strings := make([]*uint16, len(Strings))

	for i, s := range Strings {
		strings[i], err = windows.UTF16PtrFromString(s)
		if err != nil {
			return err
		}
	}

	var stringsPtr **uint16
	if len(strings) == 0 {
		stringsPtr = nil
	} else {
		stringsPtr = unsafe.SliceData(strings)
	}

	var rawData *uint8
	if len(RawData) == 0 {
		rawData = nil
	} else {
		rawData = unsafe.SliceData(RawData)
	}

	r1, _, lastErr := reportEvent.Call(
		uintptr(EventLog),
		uintptr(Type),
		uintptr(Category),
		uintptr(EventID),
		// This is how Golang zsyscall_windows.go passes *windows.SID to syscalls
		uintptr(unsafe.Pointer(UserSID)),
		uintptr(len(strings)),
		uintptr(len(RawData)),
		uintptr(unsafe.Pointer(stringsPtr)),
		uintptr(unsafe.Pointer(rawData)))
	// ReportEvent returns C FALSE (0) on error
	if r1 == 0 {
		return lastErr
	}

	return nil
}

// EvtClearLog wrapper
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtclearlog
func (api *API) EvtClearLog(ChannelPath string) error {
	channelPath, err := winutil.UTF16PtrOrNilFromString(ChannelPath)
	if err != nil {
		return err
	}

	r1, _, lastErr := evtClearLog.Call(
		uintptr(0), // local computer only
		uintptr(unsafe.Pointer(channelPath)),
		uintptr(0), // TargetFilePath not supported
		uintptr(0)) // reserved must be 0
	// EvtClearLog returns C FALSE (0) on error
	if r1 == 0 {
		return lastErr
	}

	return nil
}

// EvtOpenPublisherMetadata wrapper
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtopenpublishermetadata
func (api *API) EvtOpenPublisherMetadata(
	PublisherID string,
	LogFilePath string) (evtapi.EventPublisherMetadataHandle, error) {

	publisherID, err := windows.UTF16PtrFromString(PublisherID)
	if err != nil {
		return evtapi.EventPublisherMetadataHandle(0), err
	}

	logFilePath, err := winutil.UTF16PtrOrNilFromString(LogFilePath)
	if err != nil {
		return evtapi.EventPublisherMetadataHandle(0), err
	}

	r1, _, lastErr := evtOpenPublisherMetadata.Call(
		uintptr(0), // local computer only
		uintptr(unsafe.Pointer(publisherID)),
		uintptr(unsafe.Pointer(logFilePath)),
		uintptr(0), // use current locale
		uintptr(0)) // reserved must be 0
	// EvtOpenPublisherMetadata returns NULL on error
	if r1 == 0 {
		return evtapi.EventPublisherMetadataHandle(0), lastErr
	}

	return evtapi.EventPublisherMetadataHandle(r1), nil
}

// EvtFormatMessage wrapper
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtformatmessage
func (api *API) EvtFormatMessage(
	PublisherMetadata evtapi.EventPublisherMetadataHandle,
	Event evtapi.EventRecordHandle,
	MessageID uint,
	Values evtapi.EvtVariantValues, //nolint:revive // TODO fix revive unused-parameter
	Flags uint) (string, error) {

	// `EvtFormatMessage` has a bug in its size calculations that can cause a crash.
	//
	// `BufferUsed` output includes the null term character.
	// `BufferSize` is compared to the same value written to `BufferUsed`, so
	// that implies BufferSize should also include the null term character.
	// However, `EvtFormatMessage` (internal symbol EventRendering::MessageRender)
	// instead calls `memcpy(Buffer, data, (BufferUsed*2)+2)`, which copies a second
	// null term char into the buffer
	// * even if `Buffer` is NULL
	// * even if the +2 causes the memcpy to exceed the input `BufferSize`
	// * even if the `BufferSize` and source size are 0.
	// The source data buffer appears to have three null-term chars, so the source buffer
	// shouldn't be overrun.
	// This is particularly problematic because the Microsoft example code and the common
	// GetSize/Alloc/GetData pattern means our first call passes Buffer=NULL and BufferSize=0.
	// To fix this we must
	// * Provide a `DummyBuf` to the first `EvtFormatMessage` call so we can safely
	//   receive the extra null term char when the output value has size 0.
	// * Allocate space for BufferUsed+1, but don't include our extra space in
	//   the BufferSize argument to the second `EvtFormatMessage` call.
	// DummyBuf must be at least 2 bytes wide.
	var DummyBuf uint64
	var BufferUsed uint32

	r1, _, lastErr := evtFormatMessage.Call(
		uintptr(PublisherMetadata),
		uintptr(Event),
		uintptr(MessageID),
		uintptr(0),
		uintptr(0),
		uintptr(Flags),
		uintptr(0), // Intentionally 0 and not sizeof(DummyBuf)
		uintptr(unsafe.Pointer(&DummyBuf)),
		uintptr(unsafe.Pointer(&BufferUsed)))
	// EvtFormatMessage returns C FALSE (0) on error
	if r1 == 0 {
		if lastErr != windows.ERROR_INSUFFICIENT_BUFFER {
			return "", lastErr
		}
	} else {
		return "", nil
	}

	if BufferUsed == 0 {
		return "", nil
	}

	// Allocate buffer space (BufferUsed is size in characters)
	// Allocate an extra character due to EvtFormatMessage bug
	Buffer := make([]uint16, BufferUsed+1)

	r1, _, lastErr = evtFormatMessage.Call(
		uintptr(PublisherMetadata),
		uintptr(Event),
		uintptr(MessageID),
		uintptr(0),
		uintptr(0),
		uintptr(Flags),
		uintptr(BufferUsed), // ensure this value is 1 less than we allocate
		uintptr(unsafe.Pointer(unsafe.SliceData(Buffer))),
		uintptr(unsafe.Pointer(&BufferUsed)))
	// EvtFormatMessage returns C FALSE (0) on error
	if r1 == 0 {
		return "", lastErr
	}

	// Trim Buffer to output size (BufferUsed is size in characters)
	return windows.UTF16ToString(Buffer[:BufferUsed]), nil
}

// EvtOpenSession wrapper
//
// NOTE:
// The connection is not made and the creds are not validated at the time of this call.
// Those operations occur when the session is first used (e.g. EvtSubscribe)
//
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtopensession
func (api *API) EvtOpenSession(
	Server string,
	User string,
	Domain string,
	Password string,
	Flags uint,
) (evtapi.EventSessionHandle, error) {
	server, err := winutil.UTF16PtrOrNilFromString(Server)
	if err != nil {
		return evtapi.EventSessionHandle(0), err
	}

	user, err := winutil.UTF16PtrOrNilFromString(User)
	if err != nil {
		return evtapi.EventSessionHandle(0), err
	}

	domain, err := winutil.UTF16PtrOrNilFromString(Domain)
	if err != nil {
		return evtapi.EventSessionHandle(0), err
	}

	password, err := winutil.UTF16PtrOrNilFromString(Password)
	if err != nil {
		return evtapi.EventSessionHandle(0), err
	}

	login := &EVT_RPC_LOGIN{
		Server:   server,
		User:     user,
		Domain:   domain,
		Password: password,
		Flags:    Flags,
	}

	r1, _, lastErr := evtOpenSession.Call(
		uintptr(EvtRpcLogin),
		uintptr(unsafe.Pointer(login)),
		uintptr(0), // Timeout, reserved, must be zero
		uintptr(0), // Flags, reserved, must be zero
	)
	// EvtOpenSession returns NULL on error
	if r1 == 0 {
		return evtapi.EventSessionHandle(0), lastErr
	}

	return evtapi.EventSessionHandle(r1), nil
}
