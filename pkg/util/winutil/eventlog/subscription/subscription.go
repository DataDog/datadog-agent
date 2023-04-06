// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows
// +build windows

package evtsubscribe

import (
	"fmt"
	"sync"

	// "github.com/DataDog/datadog-agent/comp/core/log"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
	"golang.org/x/sys/windows"
)

const (
	// How many events to fetch per EvtNext call
	DEFAULT_EVENT_BATCH_COUNT = 512
)

type PullSubscription interface {
	// Start the event subscription
	Start() error

	// Stop the event subscription and free resources.
	// Stop will automatically close any outstanding event record handles associated with this subscription.
	// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtclose
	Stop()

	// EventsAvailable returns a readonly channel that will contain a value to signal the user to call GetEvents().
	// Create the subscription with the WithNotifyEventsAvailable option to enable this channel.
	//
	// Receiving a value from this channel does not guarantee that GetEvents() will return events.
	EventsAvailable() <-chan struct{}

	// GetEvents returns the next available events in the subscription.
	// If no events are available returns nil,nil
	// You must close every event record handle returned from this function.
	GetEvents() ([]*evtapi.EventRecord, error)
}

type pullSubscription struct {
	// Configuration
	channelPath      string
	query            string
	eventBatchCount  uint
	useNotifyChannel bool

	// Notify user that event records are available
	notifyEventsAvailable chan struct{}

	// datadog components
	//log log.Component

	// Windows API
	eventLogAPI        evtapi.API
	subscriptionHandle evtapi.EventResultSetHandle
	waitEventHandle    evtapi.WaitEventHandle
	stopEventHandle    evtapi.WaitEventHandle
	evtNextStorage     []evtapi.EventRecordHandle

	// Query loop management
	started                       bool
	notifyEventsAvailableWaiter   sync.WaitGroup
	notifyEventsAvailableLoopDone chan struct{}
	notifyStop                    chan struct{}

	// notifyNoMoreItems synchronizes notifyEventsAvailableLoop and GetEvents when
	// EvtNext returns ERROR_NO_MORE_ITEMS.
	// GetEvents writes to this channel to tell notifyEventsAvailableLoop to skip writing
	// to the NotifyEventsAvailable channel and return to the WaitForMultipleObjects call.
	// Without this synchronization notifyEventsAvailableLoop would block writing to the
	// NotifyEventsAvailable channel until the user read from the channel again, at which
	// point the user would be erroneously notified that events are available.
	notifyNoMoreItems         chan struct{}
	notifyNoMoreItemsComplete chan struct{}

	// EvtSubscribe args
	subscribeOriginFlag uint
	subscribeFlags      uint
	bookmark            evtbookmark.Bookmark
}

type PullSubscriptionOption func(*pullSubscription)

func newSubscriptionWaitEvent() (evtapi.WaitEventHandle, error) {
	// https://learn.microsoft.com/en-us/windows/win32/api/synchapi/nf-synchapi-createeventa
	// Manual reset, must be initially set, Windows will not set it for old events
	hEvent, err := windows.CreateEvent(nil, 1, 1, nil)
	return evtapi.WaitEventHandle(hEvent), err
}

func newStopWaitEvent() (evtapi.WaitEventHandle, error) {
	// https://learn.microsoft.com/en-us/windows/win32/api/synchapi/nf-synchapi-createeventa
	// Manual reset, initially unset
	hEvent, err := windows.CreateEvent(nil, 1, 0, nil)
	return evtapi.WaitEventHandle(hEvent), err
}

// func NewPullSubscription(log log.Component) *pullSubscription {
func NewPullSubscription(channelPath, query string, options ...PullSubscriptionOption) *pullSubscription {
	var q pullSubscription
	q.subscriptionHandle = evtapi.EventResultSetHandle(0)
	q.waitEventHandle = evtapi.WaitEventHandle(0)

	q.eventBatchCount = DEFAULT_EVENT_BATCH_COUNT

	q.channelPath = channelPath
	q.query = query
	q.subscribeOriginFlag = evtapi.EvtSubscribeToFutureEvents
	// q.log = log

	for _, o := range options {
		o(&q)
	}

	return &q
}

// Windows limits this to 1024
// https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-even6/65f22d62-5f0f-4306-85c4-50fb9e77075b
func WithEventBatchCount(count uint) PullSubscriptionOption {
	return func(q *pullSubscription) {
		q.eventBatchCount = count
	}
}

func WithWindowsEventLogAPI(api evtapi.API) PullSubscriptionOption {
	return func(q *pullSubscription) {
		q.eventLogAPI = api
	}
}

func WithStartAfterBookmark(bookmark evtbookmark.Bookmark) PullSubscriptionOption {
	return func(q *pullSubscription) {
		q.bookmark = bookmark
		q.subscribeOriginFlag = evtapi.EvtSubscribeStartAfterBookmark
	}
}

func WithStartAtOldestRecord() PullSubscriptionOption {
	return func(q *pullSubscription) {
		q.subscribeOriginFlag = evtapi.EvtSubscribeStartAtOldestRecord
	}
}

func WithSubscribeFlags(flags uint) PullSubscriptionOption {
	return func(q *pullSubscription) {
		q.subscribeFlags = flags
	}
}

func WithNotifyEventsAvailable() PullSubscriptionOption {
	return func(q *pullSubscription) {
		q.useNotifyChannel = true
	}
}

func (q *pullSubscription) EventsAvailable() <-chan struct{} {
	return q.notifyEventsAvailable
}

func (q *pullSubscription) Start() error {

	if q.started {
		return fmt.Errorf("Query subscription is already started")
	}

	var bookmarkHandle evtapi.EventBookmarkHandle
	if q.bookmark != nil {
		bookmarkHandle = q.bookmark.Handle()
	}

	// create subscription
	hSubWait, err := newSubscriptionWaitEvent()
	if err != nil {
		return err
	}
	hSub, err := q.eventLogAPI.EvtSubscribe(
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

	q.waitEventHandle = hSubWait
	q.subscriptionHandle = hSub

	if q.useNotifyChannel {
		hStopWait, err := newStopWaitEvent()
		if err != nil {
			evtapi.EvtCloseResultSet(q.eventLogAPI, q.subscriptionHandle)
			safeCloseNullHandle(windows.Handle(hSubWait))
			return err
		}
		q.stopEventHandle = hStopWait

		// Query loop management
		q.notifyStop = make(chan struct{})
		q.notifyNoMoreItems = make(chan struct{})
		q.notifyNoMoreItemsComplete = make(chan struct{})
		q.notifyEventsAvailable = make(chan struct{})
		q.notifyEventsAvailableLoopDone = make(chan struct{})

		// start goroutine to query events for channel
		q.notifyEventsAvailableWaiter.Add(1)
		go q.notifyEventsAvailableLoop()
	}

	q.started = true

	return nil
}

func (q *pullSubscription) Stop() {
	if !q.started {
		return
	}

	if q.useNotifyChannel {
		// Signal notifyEventsAvailableLoop to stop
		windows.SetEvent(windows.Handle(q.stopEventHandle))
		close(q.notifyStop)
		// Wait for notifyEventsAvailableLoop to stop
		q.notifyEventsAvailableWaiter.Wait()

		close(q.notifyNoMoreItems)
		close(q.notifyNoMoreItemsComplete)
		safeCloseNullHandle(windows.Handle(q.stopEventHandle))
	}

	// Cleanup Windows API
	evtapi.EvtCloseResultSet(q.eventLogAPI, q.subscriptionHandle)
	safeCloseNullHandle(windows.Handle(q.waitEventHandle))

	q.started = false
}

// notifyEventsAvailableLoop writes to the notifyEventsAvailable channel
// when the Windows Event Log API Subscription sets the waitEventHandle.
// On return, closes the notifyEventsAvailable channel to notify the user
// of an error or a Stop().
func (q *pullSubscription) notifyEventsAvailableLoop() {
	// q.Stop() waits on this goroutine to finish, notify it that we are done
	defer q.notifyEventsAvailableWaiter.Done()
	// close the notify channel so the user knows this loop is dead
	defer close(q.notifyEventsAvailable)
	// close so internal functions know this loop is dead
	defer close(q.notifyEventsAvailableLoopDone)

	waiters := []windows.Handle{windows.Handle(q.waitEventHandle), windows.Handle(q.stopEventHandle)}

	for {
		dwWait, err := windows.WaitForMultipleObjects(waiters, false, windows.INFINITE)
		if err != nil {
			// WAIT_FAILED
			pkglog.Errorf("WaitForMultipleObjects failed: %v", err)
			return
		}
		if dwWait == windows.WAIT_OBJECT_0 {
			// Event records are available, notify the user
			pkglog.Debugf("Events are available")
			select {
			case <-q.notifyStop:
				return
			case q.notifyEventsAvailable <- struct{}{}:
				break
			case <-q.notifyNoMoreItems:
				// EvtNext called, there are no more items to read, this case
				// allows us to cancel sending notifyEventsAvailable to the user.
				// Now we must wait for the event to be reset to ensure WaitForMultipleObjects will
				// block until Windows sets the event again.
				// We cannot just call ResetEvent here instead because that creates a race
				// with the SetEvent call in GetEvents() that could create a deadlock.
				select {
				case <-q.notifyStop:
					return
				case <-q.notifyNoMoreItemsComplete:
					break
				}
				break
			}
		} else if dwWait == (windows.WAIT_OBJECT_0 + 1) {
			// Stop event is set
			return
		} else if dwWait == uint32(windows.WAIT_TIMEOUT) {
			// timeout
			// this shouldn't happen
			pkglog.Errorf("WaitForMultipleObjects timed out")
			return
		} else {
			// some other error occurred
			gle := windows.GetLastError()
			pkglog.Errorf("WaitForMultipleObjects unknown error: wait(%d,%#x) gle(%d,%#x)",
				dwWait,
				dwWait,
				gle,
				gle)
			return
		}
	}
}

// synchronizeNoMoreItems is used to synchronize notifyEventsAvailableLoop when
// EvtNext returns ERROR_NO_MORE_ITEMS.
// Note that the Microsoft's Pull Subscription model is inherently racey. It is possible
// for EvtNext to return ERROR_NO_MORE_ITEMS and then for Windows to call SetEvent(waitHandle)
// before our code reaches the ResetEvent(waitHandle). If this happens we will not see those
// events until newer events are created and Windows once again calls SetEvent(waitHandle).
func (q *pullSubscription) synchronizeNoMoreItems() error {
	if !q.useNotifyChannel {
		_ = windows.ResetEvent(windows.Handle(q.waitEventHandle))
		return nil
	}

	// If notifyEventsAvailableLoop is blocking on WaitForMultipleObjects
	// wake it up so we can sync on notifyNoMoreItems
	// If notifyEventsAvailableLoop is blocking on notifyNoMoreItems channel then this is a no-op
	windows.SetEvent(windows.Handle(q.waitEventHandle))
	// If notifyEventsAvailableLoop is blocking on sending notifyEventsAvailable
	// then wake/cancel it so it does not erroneously send notifyEventsAvailable.
	select {
	case <-q.notifyStop:
		return fmt.Errorf("stop signal")
	case <-q.notifyEventsAvailableLoopDone:
		return fmt.Errorf("notify loop is not running")
	case q.notifyNoMoreItems <- struct{}{}:
		break
	}
	// Reset the events ready event so notifyEventsAvailableLoop will wait again in WaitForMultipleObjects,
	// then write to notifyNoMoreItemsComplete to tell the loop that the event has been reset and it
	// can safely continue.
	_ = windows.ResetEvent(windows.Handle(q.waitEventHandle))
	select {
	case <-q.notifyStop:
		return fmt.Errorf("stop signal")
	case <-q.notifyEventsAvailableLoopDone:
		return fmt.Errorf("notify loop is not running")
	case q.notifyNoMoreItemsComplete <- struct{}{}:
		break
	}
	return nil
}

// GetEvents returns the next available events in the subscription.
func (q *pullSubscription) GetEvents() ([]*evtapi.EventRecord, error) {
	// Do a non-blocking check to see if the "events available" event handle is set.
	// We should only call EvtNext when this event is set.
	dwWait, err := windows.WaitForSingleObject(windows.Handle(q.waitEventHandle), 0)
	if err != nil {
		return nil, fmt.Errorf("WaitForSingleObject failed: %v", err)
	}
	if dwWait != windows.WAIT_OBJECT_0 {
		// event is not set, there are no events available
		return nil, nil
	}

	// We supply INFINITE for the Timeout parameter but if we are at the end of the log file/there are no more events EvtNext will return,
	// it will not block forever.
	// https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-even6/cd4c258c-5a2c-4ba8-bce3-37eefaa416e7
	eventRecordHandles, err := q.eventLogAPI.EvtNext(q.subscriptionHandle, q.evtNextStorage, uint(len(q.evtNextStorage)), windows.INFINITE)
	if err == nil {
		pkglog.Debugf("EvtNext returned %v handles", len(eventRecordHandles))
		// got events
		eventRecords := q.parseEventRecordHandles(eventRecordHandles)
		return eventRecords, nil
	} else if err == windows.ERROR_TIMEOUT {
		// no more events
		// TODO: Should we reset the handle? MS example says no
		pkglog.Errorf("evtnext timeout")
		return nil, fmt.Errorf("timeout")
	} else if err == windows.ERROR_NO_MORE_ITEMS {
		// EvtNext returns ERROR_NO_MORE_ITEMS when there are no more items available.
		pkglog.Debugf("EvtNext returned no more items")
		err := q.synchronizeNoMoreItems()
		if err != nil {
			return nil, err
		}
		// not an error, there are just no more items
		return nil, nil
	}

	pkglog.Errorf("EvtNext failed: %v", err)
	return nil, err
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
