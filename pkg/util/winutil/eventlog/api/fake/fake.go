// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package fakeevtapi

import (
	"fmt"
	"sync"

	"go.uber.org/atomic"
	"golang.org/x/sys/windows"

	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

// backing datastructures for the Fake API

// API is a fake implementation of the Windows Event Log API intended to be used in tests.
// It does not make any Windows Event Log API calls.
// Event rendering is not implemented.
type API struct {
	eventLogs map[string]*eventLog

	nextHandle       *atomic.Uint64
	subscriptions    map[evtapi.EventResultSetHandle]*subscription
	eventHandles     map[evtapi.EventRecordHandle]*eventRecord
	sourceHandles    map[evtapi.EventSourceHandle]string
	bookmarkHandles  map[evtapi.EventBookmarkHandle]*bookmark
	publisherHandles map[evtapi.EventPublisherMetadataHandle]*publisherMetadata
}

type eventLog struct {
	name   string
	events []*eventRecord
	mu     sync.Mutex

	nextRecordID *atomic.Uint64

	sources map[string]*eventSource

	// For notifying of new events
	subscriptions map[evtapi.EventResultSetHandle]*subscription
}

type eventSource struct {
	name    string
	logName string
}

type subscription struct {
	channel string
	query   string
	handle  evtapi.EventResultSetHandle
	// owned by caller, not closed by this lib
	signalEventHandle evtapi.WaitEventHandle

	nextEvent uint
	isReverse bool // Track if this is a reverse direction query
}

type eventRecord struct {
	handle evtapi.EventRecordHandle

	// Must be exported so template can render them
	Type     uint
	Category uint
	EventID  uint
	UserSID  *windows.SID
	Strings  []string
	RawData  []uint8
	EventLog string
	RecordID uint
}

type bookmark struct {
	handle        evtapi.EventBookmarkHandle
	eventRecordID uint
}

type publisherMetadata struct {
	handle      evtapi.EventPublisherMetadataHandle
	publisherID string
	// Fake publisher metadata properties
	// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_publisher_metadata_property_id
	properties map[uint]string
	// For testing - allows simulating invalid handles
	isValid bool
}

// New returns a new Windows Event Log API fake
func New() *API {
	var api API

	api.nextHandle = atomic.NewUint64(0)

	api.subscriptions = make(map[evtapi.EventResultSetHandle]*subscription)
	api.eventHandles = make(map[evtapi.EventRecordHandle]*eventRecord)
	api.sourceHandles = make(map[evtapi.EventSourceHandle]string)
	api.bookmarkHandles = make(map[evtapi.EventBookmarkHandle]*bookmark)
	api.publisherHandles = make(map[evtapi.EventPublisherMetadataHandle]*publisherMetadata)

	api.eventLogs = make(map[string]*eventLog)

	return &api
}

func newEventLog(name string) *eventLog {
	var e eventLog
	e.name = name
	e.nextRecordID = atomic.NewUint64(0)
	e.subscriptions = make(map[evtapi.EventResultSetHandle]*subscription)
	e.sources = make(map[string]*eventSource)
	return &e
}

func newSubscription(channel string, query string) *subscription {
	var s subscription
	s.channel = channel
	s.query = query
	return &s
}

func newEventRecord(Type uint, category uint, eventID uint, userSID *windows.SID, eventLog string, strings []string, data []uint8) *eventRecord {
	var e eventRecord
	e.Type = Type
	e.Category = category
	e.EventID = eventID
	if userSID != nil {
		e.UserSID, _ = userSID.Copy()
	}
	e.Strings = strings
	e.RawData = data
	e.EventLog = eventLog
	return &e
}

func (api *API) addSubscription(sub *subscription) {
	h := api.nextHandle.Inc()
	sub.handle = evtapi.EventResultSetHandle(h)
	api.subscriptions[sub.handle] = sub
}

func (api *API) addEventRecord(event *eventRecord) {
	h := api.nextHandle.Inc()
	event.handle = evtapi.EventRecordHandle(h)
	api.eventHandles[event.handle] = event
}

func (api *API) addBookmark(bookmark *bookmark) {
	h := api.nextHandle.Inc()
	bookmark.handle = evtapi.EventBookmarkHandle(h)
	api.bookmarkHandles[bookmark.handle] = bookmark
}

func (api *API) addPublisherMetadata(publisher *publisherMetadata) {
	h := api.nextHandle.Inc()
	publisher.handle = evtapi.EventPublisherMetadataHandle(h)
	api.publisherHandles[publisher.handle] = publisher
}

func (api *API) getSubscriptionByHandle(subHandle evtapi.EventResultSetHandle) (*subscription, error) {
	v, ok := api.subscriptions[subHandle]
	if !ok {
		return nil, fmt.Errorf("Subscription not found: %#x", subHandle)
	}
	return v, nil
}

func (api *API) getEventRecordByHandle(eventHandle evtapi.EventRecordHandle) (*eventRecord, error) {
	v, ok := api.eventHandles[eventHandle]
	if !ok {
		return nil, fmt.Errorf("Event not found: %#x", eventHandle)
	}
	return v, nil
}

func (api *API) getBookmarkByHandle(bookmarkHandle evtapi.EventBookmarkHandle) (*bookmark, error) {
	v, ok := api.bookmarkHandles[bookmarkHandle]
	if !ok {
		return nil, fmt.Errorf("Bookmark not found: %#x", bookmarkHandle)
	}
	return v, nil
}

func (api *API) getPublisherMetadataByHandle(publisherHandle evtapi.EventPublisherMetadataHandle) (*publisherMetadata, error) {
	v, ok := api.publisherHandles[publisherHandle]
	if !ok {
		return nil, fmt.Errorf("Publisher metadata not found: %#x", publisherHandle)
	}
	return v, nil
}

// InvalidatePublisherHandle marks a publisher handle as invalid for testing
func (api *API) InvalidatePublisherHandle(handle evtapi.EventPublisherMetadataHandle) error {
	publisher, err := api.getPublisherMetadataByHandle(handle)
	if err != nil {
		return err
	}
	publisher.isValid = false
	return nil
}

func (api *API) getEventLog(name string) (*eventLog, error) {
	v, ok := api.eventLogs[name]
	if !ok {
		return nil, fmt.Errorf("The Log name \"%v\" does not exist", name)
	}
	return v, nil
}

func (api *API) getEventSourceByHandle(sourceHandle evtapi.EventSourceHandle) (*eventLog, error) {
	// lookup name using handle
	v, ok := api.sourceHandles[sourceHandle]
	if !ok {
		return nil, fmt.Errorf("Invalid source handle: %#x", sourceHandle)
	}

	return api.getEventLog(v)
}

func (api *API) addEventLog(eventLog *eventLog) {
	api.eventLogs[eventLog.name] = eventLog
}

func (e *eventLog) addEventRecord(event *eventRecord) {
	event.RecordID = uint(e.nextRecordID.Inc())
	e.events = append(e.events, event)
}

func (e *eventLog) reportEvent(
	_ *API,
	Type uint,
	Category uint,
	EventID uint,
	UserSID *windows.SID,
	Strings []string,
	RawData []uint8) *eventRecord {

	event := newEventRecord(
		Type,
		Category,
		EventID,
		UserSID,
		e.name,
		Strings,
		RawData)
	e.addEventRecord(event)

	// notify subscriptions
	for _, sub := range e.subscriptions {
		windows.SetEvent(windows.Handle(sub.signalEventHandle))
	}
	return event
}
