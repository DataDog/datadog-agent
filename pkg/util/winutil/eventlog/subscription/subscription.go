// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package evtsubscribe provides helpers for reading Windows Event Logs with a Pull Subscription
package evtsubscribe

import (
	"fmt"
	"sync"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows"
)

const (
	// DefaultEventBatchCount is the default number of events to fetch per EvtNext call
	DefaultEventBatchCount = 10
)

// PullSubscription defines the interface for reading Windows Event Logs with a Pull Subscription
// https://learn.microsoft.com/en-us/windows/win32/wes/subscribing-to-events#pull-subscriptions
type PullSubscription interface {
	// Start the event subscription
	Start() error

	// Stop the event subscription and free resources.
	// The subscription can be started again after it is stopped.
	//
	// Stop will automatically close any outstanding event record handles associated with this subscription,
	// so you must not continue using any EventRecord returned by GetEvents.
	// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtclose
	Stop()

	// Return true is the subscription is active (started), false otherwise (stopped)
	Running() bool

	// GetEvents returns a channel that provides the next available events in the subscription.
	// The channel is closed on error and Error() returns the error.
	// If an error occurs the subscription must be stopped to free resources.
	// You must close every event record handle returned from this function.
	// You must not use any EventRecords after the subscription is stopped. Windows automatically closes
	// all of the event record handles when the subscription handle is closed.
	// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtclose
	GetEvents() <-chan []*evtapi.EventRecord

	// Error returns the last error returned from the subscription, for example from EvtNext
	Error() error

	// Set the subscription to "StartAfterBookmark"
	SetBookmark(bookmark evtbookmark.Bookmark)
}

type pullSubscription struct {
	// Configuration
	channelPath     string
	query           string
	eventBatchCount uint

	// Notify user that event records are available
	eventsChannel chan []*evtapi.EventRecord
	err           error

	// Windows API
	eventLogAPI                   evtapi.API
	subscriptionHandle            evtapi.EventResultSetHandle
	subscriptionSignalEventHandle evtapi.WaitEventHandle
	stopEventHandle               evtapi.WaitEventHandle
	evtNextStorage                []evtapi.EventRecordHandle

	// Query loop management
	started             bool
	getEventsLoopWaiter sync.WaitGroup
	notifyStop          chan struct{}

	// EvtSubscribe args
	subscribeOriginFlag uint
	subscribeFlags      uint
	bookmark            evtbookmark.Bookmark
	session             evtsession.Session
}

// PullSubscriptionOption type for option pattern for NewPullSubscription constructor
type PullSubscriptionOption func(*pullSubscription)

func newSubscriptionSignalEvent() (evtapi.WaitEventHandle, error) {
	// https://learn.microsoft.com/en-us/windows/win32/api/synchapi/nf-synchapi-createeventa
	// Auto reset, must be initially set, Windows will not set it for old events
	//
	// The MSDN example uses a manual reset event, but the API docs don't specify that a manual
	// reset event is required. The manual reset event is inherently racey, consider the following
	// 1. We are waiting in WaitForMultipleObjects for the event handle to be set
	// 2. (in background) Events arrive and the handle is set
	// 3. WaitForMultipleObjects returns
	// 4. EvtNext is called and returns the events and ERROR_NO_MORE_ITEMS
	// 5. (in background) Events arrive and the handle is set
	// 6. We handle the ERROR_NO_MORE_ITEMS result and call ResetEvent to unset the event handle
	// Now WaitForMultipleObjects will block and we won't see the second set of events until a third
	// set arrives.
	// Instead, to avoid this race we use an auto reset event, which is unset when WaitForMultipleObjects
	// returns. When EvtNext returns ERROR_NO_MORE_ITEMS the event is already unset and we don't need to do anything.
	hEvent, err := windows.CreateEvent(nil, 0, 1, nil)
	return evtapi.WaitEventHandle(hEvent), err
}

func newStopWaitEvent() (evtapi.WaitEventHandle, error) {
	// https://learn.microsoft.com/en-us/windows/win32/api/synchapi/nf-synchapi-createeventa
	// Manual reset, initially unset
	hEvent, err := windows.CreateEvent(nil, 1, 0, nil)
	return evtapi.WaitEventHandle(hEvent), err
}

// NewPullSubscription constructs a new PullSubscription.
// Call Stop() when done to release resources.
func NewPullSubscription(channelPath, query string, options ...PullSubscriptionOption) PullSubscription {
	var q pullSubscription
	q.subscriptionHandle = evtapi.EventResultSetHandle(0)
	q.subscriptionSignalEventHandle = evtapi.WaitEventHandle(0)

	q.eventBatchCount = DefaultEventBatchCount

	q.channelPath = channelPath
	q.query = query
	q.subscribeOriginFlag = evtapi.EvtSubscribeToFutureEvents

	for _, o := range options {
		o(&q)
	}

	return &q
}

// WithEventBatchCount sets the maximum number of event records returned per EvtNext call.
//
// Keep this value low, EvtNext will fail if the sum of the size of the events it is
// returning exceeds a buffer size that is internal to subscription. Note that this
// maximum is unrelated provided to EvtNext, except in that a lower event batch
// means the per-event size must be larger to cause the error.
//
// There is a very small difference in performance between requesting 10 events per call
// and 1000 events per call. The bottlneck by far is EvtFormatMessage. See subscription benchmark
// tests for results.
//
// Windows limits this to 1024.
// https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-even6/65f22d62-5f0f-4306-85c4-50fb9e77075b
func WithEventBatchCount(count uint) PullSubscriptionOption {
	return func(q *pullSubscription) {
		q.eventBatchCount = count
	}
}

// WithWindowsEventLogAPI sets the API implementation used by the subscription
func WithWindowsEventLogAPI(api evtapi.API) PullSubscriptionOption {
	return func(q *pullSubscription) {
		q.eventLogAPI = api
	}
}

// WithStartAfterBookmark sets the bookmark for the subscription.
// The subscription will start reading the event log from the record identified by the bookmark.
// The subscription will not automatically update the bookmark. The user should update the
// bookmark to an event record returned from GetEvents() when it makes sense for the user.
// https://learn.microsoft.com/en-us/windows/win32/wes/bookmarking-events
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_subscribe_flags
func WithStartAfterBookmark(bookmark evtbookmark.Bookmark) PullSubscriptionOption {
	return func(q *pullSubscription) {
		q.SetBookmark(bookmark)
	}
}

// WithStartAtOldestRecord will start the subscription from the oldest record in the event log.
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_subscribe_flags
func WithStartAtOldestRecord() PullSubscriptionOption {
	return func(q *pullSubscription) {
		q.subscribeOriginFlag = evtapi.EvtSubscribeStartAtOldestRecord
	}
}

// WithSubscribeFlags can be used to manually set EVT_SUBSCRIBE_FLAGS
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_subscribe_flags
func WithSubscribeFlags(flags uint) PullSubscriptionOption {
	return func(q *pullSubscription) {
		q.subscribeFlags = flags
	}
}

// WithSession sets the session option for the subscription to enable collecting
// event logs from remote hosts.
// https://learn.microsoft.com/en-us/windows/win32/wes/accessing-remote-computers
func WithSession(session evtsession.Session) PullSubscriptionOption {
	return func(q *pullSubscription) {
		q.session = session
	}
}

func (q *pullSubscription) Error() error {
	return q.err
}

func (q *pullSubscription) Running() bool {
	return q.started
}

func (q *pullSubscription) SetBookmark(bookmark evtbookmark.Bookmark) {
	q.bookmark = bookmark
	q.subscribeOriginFlag = evtapi.EvtSubscribeStartAfterBookmark
}

func (q *pullSubscription) Start() error {

	if q.started {
		return fmt.Errorf("Query subscription is already started")
	}

	var bookmarkHandle evtapi.EventBookmarkHandle
	if q.bookmark != nil {
		bookmarkHandle = q.bookmark.Handle()
	}

	// Get session handle, if one was provided
	sessionHandle := evtapi.EventSessionHandle(0)
	if q.session != nil {
		sessionHandle = q.session.Handle()
	}

	// create subscription
	hSubWait, err := newSubscriptionSignalEvent()
	if err != nil {
		return err
	}
	hSub, err := q.eventLogAPI.EvtSubscribe(
		sessionHandle,
		hSubWait,
		q.channelPath,
		q.query,
		bookmarkHandle,
		q.subscribeOriginFlag|q.subscribeFlags)
	if err != nil {
		safeCloseNullHandle(windows.Handle(hSubWait))
		return err
	}

	// alloc reusable storage for EvtNext output
	q.evtNextStorage = make([]evtapi.EventRecordHandle, q.eventBatchCount)

	q.subscriptionSignalEventHandle = hSubWait
	q.subscriptionHandle = hSub

	hStopWait, err := newStopWaitEvent()
	if err != nil {
		evtapi.EvtCloseResultSet(q.eventLogAPI, q.subscriptionHandle)
		safeCloseNullHandle(windows.Handle(hSubWait))
		return err
	}
	q.stopEventHandle = hStopWait

	q.eventsChannel = make(chan []*evtapi.EventRecord)

	q.notifyStop = make(chan struct{})

	// after sub is ready, reset subscription error state
	// doing this later lets callers reference the error
	// until the sub actually starts again.
	q.err = nil

	// start goroutine to query events for channel
	q.getEventsLoopWaiter.Add(1)
	go q.getEventsLoop()

	q.started = true

	return nil
}

func (q *pullSubscription) Stop() {
	if !q.started {
		return
	}

	// Signal getEventsLoop to stop
	windows.SetEvent(windows.Handle(q.stopEventHandle))
	close(q.notifyStop)

	// Wait for getEventsLoop to stop
	q.getEventsLoopWaiter.Wait()

	safeCloseNullHandle(windows.Handle(q.stopEventHandle))

	// Cleanup Windows API
	evtapi.EvtCloseResultSet(q.eventLogAPI, q.subscriptionHandle)
	safeCloseNullHandle(windows.Handle(q.subscriptionSignalEventHandle))

	q.started = false
}

func (q *pullSubscription) logAndSetError(err error) {
	pkglog.Error(err)
	q.err = err
}

// getEventsLoop waits for events to be available and writes them to eventsChannel.
// On return, closes the eventsChannel channel to notify the user of an error or a Stop().
func (q *pullSubscription) getEventsLoop() {
	// q.Stop() waits on this goroutine to finish, notify it that we are done
	defer q.getEventsLoopWaiter.Done()
	defer close(q.eventsChannel)

	waiters := []windows.Handle{windows.Handle(q.subscriptionSignalEventHandle), windows.Handle(q.stopEventHandle)}

waitLoop:
	for {
		dwWait, err := windows.WaitForMultipleObjects(waiters, false, windows.INFINITE)
		if err != nil {
			// WAIT_FAILED
			q.logAndSetError(fmt.Errorf("WaitForMultipleObjects failed: %w", err))
			return
		}

		if dwWait == windows.WAIT_OBJECT_0 {
			// We were signalled by the subscription, check for events
			pkglog.Trace("Checking for events")
			// loop calling EvtNext until it returns ERROR_NO_MORE_ITEMS, or an error, or stop event is set
			for {
				// We supply INFINITE for the Timeout parameter but if we are at the end of the log file/there are no more events EvtNext will return,
				// it will not block forever.
				// https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-even6/cd4c258c-5a2c-4ba8-bce3-37eefaa416e7
				eventRecordHandles, err := q.eventLogAPI.EvtNext(q.subscriptionHandle, q.evtNextStorage, uint(len(q.evtNextStorage)), windows.INFINITE)
				if err == nil {
					pkglog.Tracef("EvtNext returned %v handles", len(eventRecordHandles))
					select {
					case q.eventsChannel <- q.parseEventRecordHandles(eventRecordHandles):
					case <-q.notifyStop:
						q.err = fmt.Errorf("received stop signal")
						pkglog.Info(q.err)
						return
					}
					continue
				}

				if err == windows.ERROR_NO_MORE_ITEMS || err == windows.ERROR_INVALID_OPERATION {
					// EvtNext returns ERROR_NO_MORE_ITEMS when there are no more items available.
					// EvtNext returns ERROR_INVALID_OPERATION when it is called when the notify event is not set.
					pkglog.Tracef("EvtNext returned no more items")
					// We do not call ResetEvent here beause we use an auto-reset event instead, so the event is
					// already unset when WaitForMultipleObjects returns.
					continue waitLoop
				}

				q.logAndSetError(fmt.Errorf("EvtNext failed: %w", err))
				return
			}
		} else if dwWait == (windows.WAIT_OBJECT_0 + 1) {
			// Stop event is set
			q.err = fmt.Errorf("received stop signal")
			pkglog.Info(q.err)
			return
		}

		// some other error occurred
		gle := windows.GetLastError()
		q.logAndSetError(fmt.Errorf("WaitForMultipleObjects unknown error: wait(%d,%#x) gle(%d,%#x)",
			dwWait,
			dwWait,
			gle,
			gle))
		return
	}
}

func (q *pullSubscription) GetEvents() <-chan []*evtapi.EventRecord {
	return q.eventsChannel
}

func (q *pullSubscription) parseEventRecordHandles(eventRecordHandles []evtapi.EventRecordHandle) []*evtapi.EventRecord {
	var err error

	eventRecords := make([]*evtapi.EventRecord, len(eventRecordHandles))

	for i, eventRecordHandle := range eventRecordHandles {
		eventRecords[i], err = q.parseEventRecordHandle(eventRecordHandle)
		if err != nil {
			pkglog.Errorf("Failed to process event (%#x): %v", eventRecordHandle, err)
		}
	}

	return eventRecords
}

func (q *pullSubscription) parseEventRecordHandle(eventRecordHandle evtapi.EventRecordHandle) (*evtapi.EventRecord, error) {
	var e evtapi.EventRecord
	e.EventRecordHandle = eventRecordHandle
	return &e, nil
}

func safeCloseNullHandle(h windows.Handle) {
	if h != windows.Handle(0) {
		windows.CloseHandle(h)
	}
}
