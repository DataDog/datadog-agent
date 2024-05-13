// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package fakeevtapi is a fake implementation of the Windows Event Log API intended to be used in tests.
// It does not make any Windows Event Log API calls.
// Event rendering is not implemented.
package fakeevtapi

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"text/template"

	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"

	"golang.org/x/sys/windows"
)

// Fake Windows APIs that implement evtapi.API

// EvtSubscribe fake
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtsubscribe
func (api *API) EvtSubscribe(
	Session evtapi.EventSessionHandle,
	SignalEvent evtapi.WaitEventHandle,
	ChannelPath string,
	Query string,
	Bookmark evtapi.EventBookmarkHandle,
	Flags uint) (evtapi.EventResultSetHandle, error) {

	if Query != "" && Query != "*" {
		return evtapi.EventResultSetHandle(0), fmt.Errorf("Fake API does not support query syntax")
	}

	if Session != evtapi.EventSessionHandle(0) {
		return evtapi.EventResultSetHandle(0), fmt.Errorf("Fake API does not support remote sessions")
	}

	// ensure channel exists
	evtlog, err := api.getEventLog(ChannelPath)
	if err != nil {
		return evtapi.EventResultSetHandle(0), err
	}
	evtlog.mu.Lock()
	defer evtlog.mu.Unlock()

	// get bookmark
	var bookmark *bookmark
	if Bookmark != evtapi.EventBookmarkHandle(0) {
		bookmark, err = api.getBookmarkByHandle(Bookmark)
		if err != nil {
			return evtapi.EventResultSetHandle(0), err
		}
	}

	// create sub
	sub := newSubscription(ChannelPath, Query)
	sub.signalEventHandle = SignalEvent

	// go to bookmark
	if bookmark != nil && (Flags&evtapi.EvtSubscribeOriginMask == evtapi.EvtSubscribeStartAfterBookmark) {
		// binary search for event with matching RecordID, or insert location
		i := sort.Search(len(evtlog.events), func(i int) bool {
			e := evtlog.events[i]
			return e.RecordID >= bookmark.eventRecordID
		})
		if i < len(evtlog.events) && evtlog.events[i].RecordID == bookmark.eventRecordID {
			// start AFTER bookmark
			sub.nextEvent = uint(i + 1)
		} else {
			// bookmarked event is no longer in the log
			if Flags&evtapi.EvtSubscribeStrict == evtapi.EvtSubscribeStrict {
				return evtapi.EventResultSetHandle(0), fmt.Errorf("bookmark not found and Strict flag set")
			}
			// MSDN says
			// If you do not include the EvtSubscribeStrict flag and the bookmarked event does not exist,
			// the subscription begins with the event that is after the event that is closest to the bookmarked event.
			// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_subscribe_flags
			// However, empirically, that is off by one and after clearing the event log it starts at the beginning,
			// or "at the event that is closest to the bookmarked event".
			sub.nextEvent = uint(i)
			// TODO: add a test for if the event log is uninstalled and re-installed, does the RecordID start over?
			//       if so then it's possible for the RecordID to be less than our bookmark.
		}
	}

	// origin flags
	if Flags&evtapi.EvtSubscribeOriginMask == evtapi.EvtSubscribeToFutureEvents {
		sub.nextEvent = uint(len(evtlog.events))
	}
	if Flags&evtapi.EvtSubscribeOriginMask == evtapi.EvtSubscribeStartAtOldestRecord {
		sub.nextEvent = 0
	}

	api.addSubscription(sub)
	evtlog.subscriptions[sub.handle] = sub
	return sub.handle, nil
}

// EvtNext fake
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtnext
func (api *API) EvtNext(
	Session evtapi.EventResultSetHandle,
	EventsArray []evtapi.EventRecordHandle, //nolint:revive // TODO fix revive unused-parameter
	EventsSize uint,
	Timeout uint) ([]evtapi.EventRecordHandle, error) { //nolint:revive // TODO fix revive unused-parameter

	// get subscription
	sub, err := api.getSubscriptionByHandle(Session)
	if err != nil {
		return nil, err
	}

	// get event log
	eventLog, err := api.getEventLog(sub.channel)
	if err != nil {
		return nil, err
	}
	eventLog.mu.Lock()
	defer eventLog.mu.Unlock()

	// is event log empty
	if len(eventLog.events) == 0 {
		return nil, windows.ERROR_NO_MORE_ITEMS
	}
	// if we are at end
	if sub.nextEvent >= uint(len(eventLog.events)) {
		return nil, windows.ERROR_NO_MORE_ITEMS
	}

	// get next events from log
	end := sub.nextEvent + EventsSize
	if end > uint(len(eventLog.events)) {
		end = uint(len(eventLog.events))
	}
	events := eventLog.events[sub.nextEvent:end]
	eventHandles := make([]evtapi.EventRecordHandle, len(events))
	for i, e := range events {
		api.addEventRecord(e)
		eventHandles[i] = e.handle
	}
	sub.nextEvent = end

	return eventHandles, nil
}

// EvtClose fake
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtclose
func (api *API) EvtClose(h windows.Handle) {
	// is handle an event?
	event, err := api.getEventRecordByHandle(evtapi.EventRecordHandle(h))
	if err == nil {
		delete(api.eventHandles, event.handle)
		return
	}
	// TODO
	// Is handle a subscription?
	sub, err := api.getSubscriptionByHandle(evtapi.EventResultSetHandle(h))
	if err == nil {
		eventLog, err := api.getEventLog(sub.channel)
		if err != nil {
			return
		}
		delete(eventLog.subscriptions, sub.handle)
		delete(api.subscriptions, sub.handle)
		return
	}
}

// EvtRenderEventXml is a fake of EvtRender with EvtRenderEventXml
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtrender
//
//revive:disable-next-line:var-naming Name is intended to match the Windows API name
func (api *API) EvtRenderEventXml(Fragment evtapi.EventRecordHandle) ([]uint16, error) {
	// get event object
	event, err := api.getEventRecordByHandle(Fragment)
	if err != nil {
		return nil, err
	}

	// Format event
	tstr := `<Event xmlns="http://scemas.microsoft.com/win/2004/08/events/event">
  <System>
	<EventID>{{ .EventID }}</EventID>
	<Channel>{{ .EventLog }}</Channel>
	<EventRecordID>{{ .RecordID }}</EventRecordID>
  </System>
  <EventData>
    {{- range $v := .Strings}}
    <Data>{{ $v }}</Data>
    {{- end}}
    {{- if .RawData}}
    <Binary>{{ hexstring .RawData }}</Binary>
    {{- end}}
  </EventData>
</Event>
`

	funcMap := template.FuncMap{
		"hexstring": hex.EncodeToString,
	}

	t, err := template.New("eventRenderXML").Funcs(funcMap).Parse(tstr)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(nil)

	err = t.Execute(buf, event)
	if err != nil {
		return nil, err
	}

	// convert from utf-8 to utf-16
	res, err := windows.UTF16FromString(buf.String())
	if err != nil {
		return nil, err
	}

	return res, nil
}

// EvtRenderBookmark is a fake of EvtRender with EvtRenderEventBookmark
// not implemented.
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtrender
func (api *API) EvtRenderBookmark(Fragment evtapi.EventBookmarkHandle) ([]uint16, error) { //nolint:revive // TODO fix revive unused-parameter
	return nil, fmt.Errorf("not implemented")
}

// RegisterEventSource fake
// https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-registereventsourcew
func (api *API) RegisterEventSource(SourceName string) (evtapi.EventSourceHandle, error) {
	// find the log the source is registered to
	for _, log := range api.eventLogs {
		_, ok := log.sources[SourceName]
		if ok {
			// Create a handle
			h := evtapi.EventSourceHandle(api.nextHandle.Inc())
			api.sourceHandles[h] = log.name
			return h, nil
		}
	}
	return evtapi.EventSourceHandle(0), fmt.Errorf("Event source %s not found", SourceName)
}

// DeregisterEventSource fake
// https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-deregistereventsource
func (api *API) DeregisterEventSource(sourceHandle evtapi.EventSourceHandle) error {
	_, err := api.getEventSourceByHandle(sourceHandle)
	if err != nil {
		return err
	}
	delete(api.sourceHandles, sourceHandle)
	return nil
}

// ReportEvent fake
// https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-reporteventw
func (api *API) ReportEvent(
	EventLog evtapi.EventSourceHandle,
	Type uint,
	Category uint,
	EventID uint,
	UserSID *windows.SID,
	Strings []string,
	RawData []uint8) error {

	// get event log
	eventLog, err := api.getEventSourceByHandle(EventLog)
	if err != nil {
		return err
	}

	_ = eventLog.reportEvent(
		api,
		Type,
		Category,
		EventID,
		UserSID,
		Strings,
		RawData)

	return nil
}

// EvtClearLog fake
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtclearlog
func (api *API) EvtClearLog(ChannelPath string) error {
	// Ensure eventlog exists
	eventLog, err := api.getEventLog(ChannelPath)
	if err != nil {
		return err
	}

	// clear the log
	eventLog.mu.Lock()
	defer eventLog.mu.Unlock()
	eventLog.events = nil
	// clearing the log does NOT reset the record IDs
	return nil
}

// EvtCreateBookmark fake
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtcreatebookmark
func (api *API) EvtCreateBookmark(BookmarkXML string) (evtapi.EventBookmarkHandle, error) { //nolint:revive // TODO fix revive unused-parameter
	var b bookmark

	// TODO: parse Xml to get record ID

	api.addBookmark(&b)

	return evtapi.EventBookmarkHandle(b.handle), nil
}

// EvtUpdateBookmark fake
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtupdatebookmark
func (api *API) EvtUpdateBookmark(Bookmark evtapi.EventBookmarkHandle, Event evtapi.EventRecordHandle) error {
	// Get bookmark
	bookmark, err := api.getBookmarkByHandle(Bookmark)
	if err != nil {
		return err
	}

	// get event
	event, err := api.getEventRecordByHandle(Event)
	if err != nil {
		return err
	}

	// Update bookmark to point to event
	bookmark.eventRecordID = event.RecordID

	return nil
}

// EvtCreateRenderContext fake
// not implemented.
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtcreaterendercontext
func (api *API) EvtCreateRenderContext(ValuePaths []string, Flags uint) (evtapi.EventRenderContextHandle, error) { //nolint:revive // TODO fix revive unused-parameter
	return evtapi.EventRenderContextHandle(0), fmt.Errorf("not implemented")
}

// EvtRenderEventValues is a fake of EvtRender with EvtRenderEventValues
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtrender
func (api *API) EvtRenderEventValues(Context evtapi.EventRenderContextHandle, Fragment evtapi.EventRecordHandle) (evtapi.EvtVariantValues, error) { //nolint:revive // TODO fix revive unused-parameter
	return nil, fmt.Errorf("not implemented")
}

// EvtOpenPublisherMetadata fake
// not implemented.
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtopenpublishermetadata
func (api *API) EvtOpenPublisherMetadata(
	PublisherID string, //nolint:revive // TODO fix revive unused-parameter
	LogFilePath string) (evtapi.EventPublisherMetadataHandle, error) {
	return evtapi.EventPublisherMetadataHandle(0), fmt.Errorf("not implemented")
}

// EvtFormatMessage fake
// not implemented.
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtformatmessage
func (api *API) EvtFormatMessage(
	PublisherMetadata evtapi.EventPublisherMetadataHandle, //nolint:revive // TODO fix revive unused-parameter
	Event evtapi.EventRecordHandle, //nolint:revive // TODO fix revive unused-parameter
	MessageID uint, //nolint:revive // TODO fix revive unused-parameter
	Values evtapi.EvtVariantValues, //nolint:revive // TODO fix revive unused-parameter
	Flags uint) (string, error) { //nolint:revive // TODO fix revive unused-parameter
	return "", fmt.Errorf("not implemented")
}

// EvtOpenSession fake
// not implemented.
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtformatmessage
func (api *API) EvtOpenSession(
	Server string, //nolint:revive // TODO fix revive unused-parameter
	User string, //nolint:revive // TODO fix revive unused-parameter
	Domain string, //nolint:revive // TODO fix revive unused-parameter
	Password string, //nolint:revive // TODO fix revive unused-parameter
	Flags uint, //nolint:revive // TODO fix revive unused-parameter
) (evtapi.EventSessionHandle, error) {
	return evtapi.EventSessionHandle(0), fmt.Errorf("not implemented")
}
