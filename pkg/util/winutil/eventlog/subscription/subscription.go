// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package evtsubscribe provides helpers for reading Windows Event Logs with a Pull Subscription
package evtsubscribe

import (
	"errors"
	"fmt"
	"sync"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/session"
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
	session             evtsession.Session

	// Bookmark initialization
	bookmarkSaver evtbookmark.Saver
	startMode     string // "oldest" or "now"
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

// WithBookmarkSaver provides an interface for the subscription to load and save persisted bookmarks.
//
// On Start(), the subscription will attempt to load a persisted bookmark.
//   - If successful, the subscription starts from that position.
//   - If loading fails or returns an empty string, the subscription uses the startMode to determine where to start reading events.
//     When a bookmark is created by FromLatestEvent (start mode "now"), it is immediately persisted.
//
// The user should update the bookmark to an event record returned from GetEvents() when it makes sense for the user.
// https://learn.microsoft.com/en-us/windows/win32/wes/bookmarking-events
func WithBookmarkSaver(saver evtbookmark.Saver) PullSubscriptionOption {
	return func(q *pullSubscription) {
		q.bookmarkSaver = saver
	}
}

// WithStartMode sets the start mode ("oldest" or "now") used when no bookmark exists.
// This option is only used when no bookmark is loaded via WithBookmarkSaver.
//   - "oldest": start from oldest event in log (EvtSubscribeStartAtOldestRecord)
//   - "now": use FromLatestEvent to create bookmark from latest matching event
//
// If startMode is "now" and no matching events are found, the subscription will start
// with EvtSubscribeToFutureEvents to capture any new events that match the query.
// The default mode if not specified is "now" (EvtSubscribeToFutureEvents).
func WithStartMode(mode string) PullSubscriptionOption {
	return func(q *pullSubscription) {
		q.startMode = mode
		// Set the appropriate origin flag based on start mode
		if mode == "oldest" {
			q.subscribeOriginFlag = evtapi.EvtSubscribeStartAtOldestRecord
		} else {
			// Default to "now" mode (future events)
			q.subscribeOriginFlag = evtapi.EvtSubscribeToFutureEvents
		}
	}
}

func (q *pullSubscription) Error() error {
	return q.err
}

func (q *pullSubscription) Running() bool {
	return q.started
}

func (q *pullSubscription) Start() error {

	if q.started {
		return errors.New("Query subscription is already started")
	}

	// Initialize bookmark (may load from saver or create new)
	bookmark, err := q.initializeBookmark()
	if err != nil {
		return err
	}

	var bookmarkHandle evtapi.EventBookmarkHandle
	if bookmark != nil {
		// Close bookmark when we're done with it
		defer bookmark.Close()
		bookmarkHandle = bookmark.Handle()
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

func (q *pullSubscription) initializeBookmark() (evtbookmark.Bookmark, error) {
	if q.bookmarkSaver == nil {
		// no bookmark saver provided, so we can't load a bookmark
		return nil, nil
	}

	// Try to load persisted bookmark
	bookmarkXML, err := q.bookmarkSaver.Load()
	if err == nil && bookmarkXML != "" {
		// Load bookmark from XML
		bookmark, err := evtbookmark.New(
			evtbookmark.WithWindowsEventLogAPI(q.eventLogAPI),
			evtbookmark.FromXML(bookmarkXML))
		if err == nil {
			pkglog.Debug("Loaded persisted bookmark from saver")
			q.subscribeOriginFlag = evtapi.EvtSubscribeStartAfterBookmark
			return bookmark, nil
		}
		pkglog.Warnf("Failed to load bookmark from XML: %v", err)
	}

	// If no bookmark and startMode is "oldest", we don't need to create a bookmark
	// we will always start from the oldest event in the log, and will create a bookmark
	// once we read an event.
	if q.startMode == "oldest" {
		return nil, nil
	}

	// If no bookmark and startMode is "now", create from latest event and save it
	if q.startMode == "now" {
		pkglog.Debugf("No bookmark found, creating from latest event for channel '%s'", q.channelPath)
		return q.initializeBookmarkFromLatestEvent()
	}

	return nil, nil
}

func (q *pullSubscription) initializeBookmarkFromLatestEvent() (evtbookmark.Bookmark, error) {
	if q.bookmarkSaver == nil {
		// This function doesn't make sense if we're not going to save the bookmark
		return nil, errors.New("bookmark saver not provided")
	}

	bookmark, err := evtbookmark.FromLatestEvent(q.eventLogAPI, q.channelPath, q.query)
	if err != nil {
		if errors.Is(err, evtbookmark.ErrNoMatchingEvents) {
			// No events found - log and continue with EvtSubscribeToFutureEvents
			pkglog.Debugf("No matching events found for channel '%s', starting subscription from future events", q.channelPath)
			// Persist an empty bookmark, see documentation for persistEmptyBookmark for more details.
			if err := persistEmptyBookmark(q.eventLogAPI, q.bookmarkSaver); err != nil {
				pkglog.Warnf("Failed to persist empty bookmark: %v", err)
			}
			// Return nil bookmark - we'll start with EvtSubscribeToFutureEvents
			return nil, nil
		}
		return nil, fmt.Errorf("failed to create bookmark from latest event: %w", err)
	}

	// We have a bookmark from the latest event
	pkglog.Debug("Created bookmark from latest event")
	q.subscribeOriginFlag = evtapi.EvtSubscribeStartAfterBookmark

	// Immediately persist the bookmark
	bookmarkXML, err := bookmark.Render()
	if err == nil {
		if err := q.bookmarkSaver.Save(bookmarkXML); err != nil {
			pkglog.Warnf("Failed to persist bookmark: %v", err)
		}
	}

	return bookmark, nil
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
						q.err = errors.New("received stop signal")
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
			q.err = errors.New("received stop signal")
			pkglog.Info(q.err)
			return
		}

		// some other unexpected return value
		q.logAndSetError(fmt.Errorf("WaitForMultipleObjects unexpected return value: %d (%#x)",
			dwWait,
			dwWait))
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

// persistEmptyBookmark saves an empty bookmark to the bookmark saver.
//
// An empty bookmark will cause the subscription to start from the beginning of the log. This behavior seems undocumented.
// This is useful for handling ErrNoMatchingEvents from FromLatestEvent. We just queried the log and we know that there are no matching events,
// so next Agent start can read from the beginning of the log without worrying about duplicating or sending old events.
// This behavior is tested by TestInitializeBookmark_StartModeNowEmptyLog
// If we do not save the empty bookmark now, then we could miss events that occur when the agent is not running. Though this would
// only be an issue the first time. Once there are matching events then FromLatestEvent will see the latset and create a bookmark.
// Note: empty bookmark refers to an empty BookmarkList XML field, not an empty string.
func persistEmptyBookmark(api evtapi.API, bookmarkSaver evtbookmark.Saver) error {
	emptyBookmark, err := evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(api))
	if err != nil {
		return fmt.Errorf("failed to create empty bookmark: %w", err)
	}
	defer emptyBookmark.Close()

	bookmarkXML, err := emptyBookmark.Render()
	if err != nil {
		return fmt.Errorf("failed to render empty bookmark: %w", err)
	}

	if err := bookmarkSaver.Save(bookmarkXML); err != nil {
		return fmt.Errorf("failed to persist empty bookmark: %w", err)
	}

	return nil
}
