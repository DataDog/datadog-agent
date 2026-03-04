// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package evtsubscribe

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"testing"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	publishermetadatacache "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/publishermetadatacache"

	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	evtbookmark "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
	eventlog_test "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"golang.org/x/sys/windows"
)

var debuglogFlag = flag.Bool("debuglog", false, "Enable seelog debug logging")

func optEnableDebugLogging() {
	// Enable logger
	if *debuglogFlag {
		pkglog.SetupLogger(pkglog.Default(), "debug")
	}
}

func TestInvalidChannel(t *testing.T) {
	optEnableDebugLogging()

	testerNames := eventlog_test.GetEnabledAPITesters()

	for _, tiName := range testerNames {
		t.Run(tiName+"API", func(t *testing.T) {
			ti := eventlog_test.GetAPITesterByName(tiName, t)
			sub := NewPullSubscription(
				"nonexistentchannel",
				"*",
				WithWindowsEventLogAPI(ti.API()))

			err := sub.Start()
			require.Error(t, err)
		})
	}
}

func createLog(t testing.TB, ti eventlog_test.APITester, channel string, source string) error {
	err := ti.InstallChannel(channel)
	if !assert.NoError(t, err) {
		return err
	}
	err = ti.API().EvtClearLog(channel)
	if !assert.NoError(t, err) {
		return err
	}
	err = ti.InstallSource(channel, source)
	if !assert.NoError(t, err) {
		return err
	}
	t.Cleanup(func() {
		ti.RemoveSource(channel, source)
		ti.RemoveChannel(channel)
	})
	return nil
}

func startSubscription(t testing.TB, ti eventlog_test.APITester, channel string, options ...PullSubscriptionOption) (PullSubscription, error) {
	opts := []PullSubscriptionOption{WithWindowsEventLogAPI(ti.API())}
	opts = append(opts, options...)

	// Create sub
	sub := NewPullSubscription(
		channel,
		"*",
		opts...)

	err := sub.Start()
	if !assert.NoError(t, err) {
		return nil, err
	}

	// run Stop() when the test finishes
	t.Cleanup(func() { sub.Stop() })
	return sub, nil
}

func getEventHandles(t testing.TB, ti eventlog_test.APITester, sub PullSubscription, numEvents uint) ([]*evtapi.EventRecord, error) {
	eventRecords, err := ReadNumEvents(t, ti, sub, numEvents)
	if err != nil {
		return nil, err
	}
	count := uint(len(eventRecords))
	if !assert.Equal(t, numEvents, count, fmt.Sprintf("Missing events, collected %d/%d events", count, numEvents)) {
		return eventRecords, errors.New("Missing events")
	}
	return eventRecords, nil
}

func assertNoMoreEvents(t testing.TB, sub PullSubscription) error {
	select {
	case <-sub.GetEvents():
		assert.Fail(t, "GetEvents should block when there are no more events!")
		return errors.New("GetEvents did not block")
	default:
		return nil
	}
}

// newMockSaverWithBookmark creates a mock bookmark saver that returns the given bookmark XML on Load()
func newMockSaverWithBookmark(bookmarkXML string) *evtbookmark.MockSaver {
	mockSaver := new(evtbookmark.MockSaver)
	mockSaver.On("Load").Return(bookmarkXML, nil).Once()
	return mockSaver
}

func BenchmarkTestGetEventHandles(b *testing.B) {
	optEnableDebugLogging()

	channel := "dd-test-channel-subscription"
	eventSource := "dd-test-source-subscription"
	numEvents := []uint{10, 100, 1000, 10000}

	testerNames := eventlog_test.GetEnabledAPITesters()

	for _, tiName := range testerNames {
		for _, v := range numEvents {
			b.Run(fmt.Sprintf("%vAPI/%d", tiName, v), func(b *testing.B) {
				ti := eventlog_test.GetAPITesterByName(tiName, b)
				createLog(b, ti, channel, eventSource)
				err := ti.GenerateEvents(eventSource, v)
				require.NoError(b, err)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					sub, err := startSubscription(b, ti, channel,
						WithStartAtOldestRecord(),
					)
					require.NoError(b, err)
					events, err := getEventHandles(b, ti, sub, v)
					require.NoError(b, err)
					err = assertNoMoreEvents(b, sub)
					require.NoError(b, err)
					for _, event := range events {
						evtapi.EvtCloseRecord(ti.API(), event.EventRecordHandle)
					}
					sub.Stop()
				}
				elapsed := b.Elapsed()
				totalEvents := float64(v) * float64(b.N)
				b.Logf("%.2f events/s (%.3fs)", totalEvents/elapsed.Seconds(), elapsed.Seconds())
			})
		}
	}
}

func formatEventMessage(api evtapi.API, cache *publishermetadatacache.PublisherMetadataCache, event *evtapi.EventRecord) (string, error) {
	// Create render context for the System values
	c, err := api.EvtCreateRenderContext(nil, evtapi.EvtRenderContextSystem)
	if err != nil {
		return "", fmt.Errorf("failed to create render context: %w", err)
	}
	defer evtapi.EvtCloseRenderContext(api, c)

	// Render the values
	vals, err := api.EvtRenderEventValues(c, event.EventRecordHandle)
	if err != nil {
		return "", fmt.Errorf("failed to render values: %w", err)
	}
	defer vals.Close()

	// Get the provider name
	provider, err := vals.String(evtapi.EvtSystemProviderName)
	if err != nil {
		return "", fmt.Errorf("failed to get provider name value: %w", err)
	}

	// Format Message
	message, err := cache.FormatMessage(provider, event.EventRecordHandle, evtapi.EvtFormatMessageEvent)
	if err != nil {
		return "", fmt.Errorf("failed to format message: %w", err)
	}

	return message, nil
}

func BenchmarkTestRenderEventXml(b *testing.B) {
	optEnableDebugLogging()

	channel := "dd-test-channel-subscription"
	eventSource := "dd-test-source-subscription"
	numEvents := []uint{10, 100, 1000, 10000}

	testerNames := eventlog_test.GetEnabledAPITesters()

	for _, tiName := range testerNames {
		for _, v := range numEvents {
			b.Run(fmt.Sprintf("%vAPI/%d", tiName, v), func(b *testing.B) {
				ti := eventlog_test.GetAPITesterByName(tiName, b)
				createLog(b, ti, channel, eventSource)
				err := ti.GenerateEvents(eventSource, v)
				require.NoError(b, err)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					sub, err := startSubscription(b, ti, channel,
						WithStartAtOldestRecord(),
					)
					require.NoError(b, err)
					events, err := getEventHandles(b, ti, sub, v)
					require.NoError(b, err)
					for _, event := range events {
						_, err := ti.API().EvtRenderEventXml(event.EventRecordHandle)
						require.NoError(b, err)
						evtapi.EvtCloseRecord(ti.API(), event.EventRecordHandle)
					}
					err = assertNoMoreEvents(b, sub)
					require.NoError(b, err)
					sub.Stop()
				}
				elapsed := b.Elapsed()
				totalEvents := float64(v) * float64(b.N)
				b.Logf("%.2f events/s (%.3fs)", totalEvents/elapsed.Seconds(), elapsed.Seconds())
			})
		}
	}
}

func BenchmarkTestFormatEventMessage(b *testing.B) {
	optEnableDebugLogging()

	channel := "dd-test-channel-subscription"
	eventSource := "dd-test-source-subscription"
	numEvents := []uint{10, 100, 1000, 10000}

	testerNames := eventlog_test.GetEnabledAPITesters()

	for _, tiName := range testerNames {
		for _, v := range numEvents {
			b.Run(fmt.Sprintf("%vAPI/%d", tiName, v), func(b *testing.B) {
				if tiName == "Fake" {
					b.Skip("Fake API does not implement EvtRenderValues")
				}
				ti := eventlog_test.GetAPITesterByName(tiName, b)
				createLog(b, ti, channel, eventSource)
				err := ti.GenerateEvents(eventSource, v)
				require.NoError(b, err)
				cache := publishermetadatacache.New(ti.API())
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					sub, err := startSubscription(b, ti, channel,
						WithStartAtOldestRecord(),
					)
					require.NoError(b, err)
					events, err := getEventHandles(b, ti, sub, v)
					require.NoError(b, err)
					for _, event := range events {
						_, err := formatEventMessage(ti.API(), cache, event)
						require.NoError(b, err)
						evtapi.EvtCloseRecord(ti.API(), event.EventRecordHandle)
					}
					err = assertNoMoreEvents(b, sub)
					require.NoError(b, err)
					sub.Stop()
				}
				elapsed := b.Elapsed()
				totalEvents := float64(v) * float64(b.N)
				b.Logf("%.2f events/s (%.3fs)", totalEvents/elapsed.Seconds(), elapsed.Seconds())
			})
		}
	}
}

type GetEventsTestSuite struct {
	suite.Suite

	channelPath string
	eventSource string
	testAPI     string
	numEvents   uint

	ti eventlog_test.APITester
}

func (s *GetEventsTestSuite) SetupSuite() {
	//fmt.Println("SetupSuite")

	optEnableDebugLogging()

	s.ti = eventlog_test.GetAPITesterByName(s.testAPI, s.T())
	err := s.ti.InstallChannel(s.channelPath)
	require.NoError(s.T(), err)
	err = s.ti.InstallSource(s.channelPath, s.eventSource)
	require.NoError(s.T(), err)
}

func (s *GetEventsTestSuite) TearDownSuite() {
	// fmt.Println("TearDownSuite")
	s.ti.RemoveSource(s.channelPath, s.eventSource)
	s.ti.RemoveChannel(s.channelPath)
}

func (s *GetEventsTestSuite) SetupTest() {
	// Ensure the log is empty
	// fmt.Println("SetupTest")
	err := s.ti.API().EvtClearLog(s.channelPath)
	require.NoError(s.T(), err)

}

func (s *GetEventsTestSuite) TearDownTest() {
	// fmt.Println("TearDownTest")
	err := s.ti.API().EvtClearLog(s.channelPath)
	require.NoError(s.T(), err)

}

// Tests that the subscription can read old events (EvtSubscribeStartAtOldestRecord)
func (s *GetEventsTestSuite) TestReadOldEvents() {
	// Put events in the log
	err := s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Create sub
	sub, err := startSubscription(s.T(), s.ti, s.channelPath,
		WithStartAtOldestRecord(),
	)
	require.NoError(s.T(), err)

	// Put events in the log
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Get Events
	_, err = getEventHandles(s.T(), s.ti, sub, 2*s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)
}

// Tests that the subscription is notified of and can read new events
func (s *GetEventsTestSuite) TestReadNewEvents() {
	// Create sub
	sub, err := startSubscription(s.T(), s.ti, s.channelPath)
	require.NoError(s.T(), err)

	// Put events in the log
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Get Events
	_, err = getEventHandles(s.T(), s.ti, sub, s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)
}

// Tests that the subscription can skip over old events (EvtSubscribeToFutureEvents)
func (s *GetEventsTestSuite) TestReadOnlyNewEvents() {
	// Put events in the log
	err := s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Create sub
	sub, err := startSubscription(s.T(), s.ti, s.channelPath)
	require.NoError(s.T(), err)

	// Put events in the log
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Get Events
	_, err = getEventHandles(s.T(), s.ti, sub, s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)
}

// Tests that Stop() can be called when there are events available to be collected
func (s *GetEventsTestSuite) TestStopWhileWaitingWithEventsAvailable() {
	// reduce batch count below events we will generate
	batchCount := s.numEvents / 2

	// Create subscription
	sub, err := startSubscription(s.T(), s.ti, s.channelPath,
		WithEventBatchCount(batchCount))
	require.NoError(s.T(), err)

	// Put events in the log
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	readyToStop := make(chan struct{})
	stopped := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Read not all of the events
		_, err := ReadNumEvents(s.T(), s.ti, sub, batchCount)
		close(readyToStop)
		if !assert.NoError(s.T(), err) {
			return
		}
		// Purposefully don't read the rest of the events. This leaves the signal event set.
		// Wait for Stop() to finish
		<-stopped
		_, ok := <-sub.GetEvents()
		assert.False(s.T(), ok, "GetEvents channel should be closed after Stop()")
	}()

	<-readyToStop
	sub.Stop()
	close(stopped)
	<-done
}

// Tests that Stop() can be called when we read all events but haven't received ERROR_NO_MORE_ITEMS
func (s *GetEventsTestSuite) TestStopWhileWaitingWithNoMoreItemseNotFinalized() {
	// Create subscription
	sub, err := startSubscription(s.T(), s.ti, s.channelPath)
	require.NoError(s.T(), err)

	// Put events in the log
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	readyToStop := make(chan struct{})
	stopped := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Read all events
		_, err := getEventHandles(s.T(), s.ti, sub, s.numEvents)
		close(readyToStop)
		if !assert.NoError(s.T(), err) {
			return
		}
		// Purposefully don't call EvtNext the final time when it would normally return ERROR_NO_MORE_ITEMS.
		// This leaves the signal event set.
		// Wait for Stop() to finish
		<-stopped
		_, ok := <-sub.GetEvents()
		assert.False(s.T(), ok, "Getevents channel should be closed after Stop()")
	}()

	<-readyToStop
	sub.Stop()
	close(stopped)
	<-done
}

// Tests that Stop() can be called when the subscription is in a ERROR_NO_MORE_ITEMS state
func (s *GetEventsTestSuite) TestStopWhileWaitingNoMoreEvents() {
	// Create subscription
	sub, err := startSubscription(s.T(), s.ti, s.channelPath)
	require.NoError(s.T(), err)

	// Put events in the log
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	readyToStop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		// Read all events
		_, err := getEventHandles(s.T(), s.ti, sub, s.numEvents)
		if err != nil {
			close(readyToStop)
			close(done)
			return
		}
		err = assertNoMoreEvents(s.T(), sub)
		if err != nil {
			close(readyToStop)
			close(done)
			return
		}
		close(readyToStop)
		// block on events available notification
		_, ok := <-sub.GetEvents()
		assert.False(s.T(), ok, "GetEvents channel should be closed after Stop()")
		close(done)
	}()

	<-readyToStop
	sub.Stop()
	<-done
}

// Tests that GetEvents does not deadlock when getEventsLoop unexpectedly exits
func (s *GetEventsTestSuite) TestHandleEarlyGetEventsLoopExit() {
	// Create sub
	sub, err := startSubscription(s.T(), s.ti, s.channelPath)
	require.NoError(s.T(), err)
	defer sub.Stop()

	// Need base type for this test
	baseSub := sub.(*pullSubscription)

	// set stop event to trigger getEventsLoop to exit
	windows.SetEvent(windows.Handle(baseSub.stopEventHandle))
	require.NoError(s.T(), err)

	// wait for the loop to exit
	baseSub.getEventsLoopWaiter.Wait()

	// ensure the events channel is closed
	_, ok := <-sub.GetEvents()
	require.False(s.T(), ok, "GetEvents should close when the loop exits")
	require.Error(s.T(), sub.Error(), "GetEvents should set error when the loop exits")

	// handle error by stopping subscription, then restart it
	sub.Stop()
	sub.Start()

	// Put events in the log
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Read the events
	_, err = getEventHandles(s.T(), s.ti, sub, s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)

	// success if we did not deadlock
}

// Tests that the subscription can start from a provided bookmark
func (s *GetEventsTestSuite) TestStartAfterBookmark() {
	//
	// Add some events to the log and create a bookmark
	//

	// Put events in the log
	err := s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Create sub
	sub, err := startSubscription(s.T(), s.ti, s.channelPath,
		WithStartAtOldestRecord(),
	)
	require.NoError(s.T(), err)

	// Read the events
	events, err := getEventHandles(s.T(), s.ti, sub, s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)

	// Get bookmark XML from last event
	bookmarkXML, err := bookmarkXMLFromEvent(s.ti.API(), events[len(events)-1])
	require.NoError(s.T(), err)

	// Close out this subscription
	sub.Stop()

	//
	// Add more events and verify the log contains twice as many events
	//

	// Add some more events
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Create sub
	sub, err = startSubscription(s.T(), s.ti, s.channelPath,
		WithStartAtOldestRecord(),
	)
	require.NoError(s.T(), err)

	// Read the events
	_, err = getEventHandles(s.T(), s.ti, sub, 2*s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)

	// Close out this subscription
	sub.Stop()

	//
	// Start subscription part way through log with bookmark
	//

	// Create a new subscription starting from the bookmark
	mockSaver := newMockSaverWithBookmark(bookmarkXML)
	sub, err = startSubscription(s.T(), s.ti, s.channelPath,
		WithBookmarkSaver(mockSaver),
		WithStartMode("now"),
	)
	require.NoError(s.T(), err)
	mockSaver.AssertExpectations(s.T())

	// Get Events
	_, err = getEventHandles(s.T(), s.ti, sub, s.numEvents)
	require.NoError(s.T(), err)
	// Since we started halfway through there should be no more events
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)
}

// Tests that the subscription starts when a bookmark is not found and the EvtSubscribeStrict flag is NOT provided
func (s *GetEventsTestSuite) TestStartAfterBookmarkNotFoundWithoutStrictFlag() {
	//
	// Add some events to the log and create a bookmark
	//

	// Put events in the log
	err := s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Create sub
	sub, err := startSubscription(s.T(), s.ti, s.channelPath,
		WithStartAtOldestRecord(),
	)
	require.NoError(s.T(), err)

	// Read the events
	events, err := getEventHandles(s.T(), s.ti, sub, s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)

	// Get bookmark XML from last event
	bookmarkXML, err := bookmarkXMLFromEvent(s.ti.API(), events[len(events)-1])
	require.NoError(s.T(), err)

	// Close out this subscription
	sub.Stop()

	// Clear the log so the bookmark is missing
	err = s.ti.API().EvtClearLog(s.channelPath)
	require.NoError(s.T(), err)

	//
	// Add more events and verify the log contains only that many events
	//

	// Add some more events
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Create sub
	sub, err = startSubscription(s.T(), s.ti, s.channelPath,
		WithStartAtOldestRecord(),
	)
	require.NoError(s.T(), err)

	// Read the events
	_, err = getEventHandles(s.T(), s.ti, sub, s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)

	// Close out this subscription
	sub.Stop()

	//
	// Bookmark is not found so subscription should start from beginning
	//

	// Create a new subscription starting from the bookmark
	mockSaver := newMockSaverWithBookmark(bookmarkXML)
	sub, err = startSubscription(s.T(), s.ti, s.channelPath,
		WithBookmarkSaver(mockSaver),
		WithStartMode("now"),
	)
	// strict flag not set so there should be no error
	require.NoError(s.T(), err)
	mockSaver.AssertExpectations(s.T())

	// Get Events
	_, err = getEventHandles(s.T(), s.ti, sub, s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)
}

// Tests that the subscription returns an error when a bookmark is not found and the EvtSubscribeStrict flag is provided
func (s *GetEventsTestSuite) TestStartAfterBookmarkNotFoundWithStrictFlag() {
	//
	// Add some events to the log and create a bookmark
	//

	// Put events in the log
	err := s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Create sub
	sub, err := startSubscription(s.T(), s.ti, s.channelPath,
		WithStartAtOldestRecord(),
	)
	require.NoError(s.T(), err)

	// Read the events
	events, err := getEventHandles(s.T(), s.ti, sub, s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)

	// Get bookmark XML from last event
	bookmarkXML, err := bookmarkXMLFromEvent(s.ti.API(), events[len(events)-1])
	require.NoError(s.T(), err)

	// Close out this subscription
	sub.Stop()

	// Clear the log so the bookmark is missing
	err = s.ti.API().EvtClearLog(s.channelPath)
	require.NoError(s.T(), err)

	//
	// Add more events and verify the log contains only that many events
	//

	// Add some more events
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Create sub
	sub, err = startSubscription(s.T(), s.ti, s.channelPath,
		WithStartAtOldestRecord(),
	)
	require.NoError(s.T(), err)

	// Read the events
	_, err = getEventHandles(s.T(), s.ti, sub, s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)

	// Close out this subscription
	sub.Stop()

	//
	// With bookmark not found and strict flag set subscription should fail
	//

	mockSaver := newMockSaverWithBookmark(bookmarkXML)
	sub = NewPullSubscription(
		s.channelPath,
		"*",
		WithWindowsEventLogAPI(s.ti.API()),
		WithBookmarkSaver(mockSaver),
		WithStartMode("now"),
		WithSubscribeFlags(evtapi.EvtSubscribeStrict))
	err = sub.Start()
	require.Error(s.T(), err, "Subscription should return error when bookmark is not found and the Strict flag is set")
}

// Tests that the subscription can call GetEvents() when there are no events
func (s *GetEventsTestSuite) TestReadWhenNoEvents() {
	// Create sub
	sub, err := startSubscription(s.T(), s.ti, s.channelPath,
		WithStartAtOldestRecord())
	require.NoError(s.T(), err)

	// Channel should block when there are no events
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)
}

// Tests that the subscription can call GetEvents() when there are no more events
func (s *GetEventsTestSuite) TestReadWhenNoMoreEvents() {
	// Put events in the log
	err := s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Create sub
	sub, err := startSubscription(s.T(), s.ti, s.channelPath,
		WithStartAtOldestRecord())
	require.NoError(s.T(), err)

	// Put events in the log
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Read all the events
	eventRecords, err := getEventHandles(s.T(), s.ti, sub, 2*s.numEvents)
	require.NoError(s.T(), err)
	count := uint(len(eventRecords))
	require.Equal(s.T(), 2*s.numEvents, count, fmt.Sprintf("Missing events, collected %d/%d events", count, 2*s.numEvents))
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)

	// Get no more items again
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)
}

// Test that a persisted bookmark is loaded and used when available
func (s *GetEventsTestSuite) TestInitializeBookmark_LoadPersistedBookmark() {
	// Generate initial events
	err := s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Create a subscription to establish a bookmark
	sub1, err := startSubscription(s.T(), s.ti, s.channelPath, WithStartAtOldestRecord())
	require.NoError(s.T(), err)

	// Read all events and update bookmark
	events, err := getEventHandles(s.T(), s.ti, sub1, s.numEvents)
	require.NoError(s.T(), err)
	bookmarkXML, err := bookmarkXMLFromEvent(s.ti.API(), events[len(events)-1])
	require.NoError(s.T(), err)

	sub1.Stop()

	// Generate more events while subscription is stopped
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Create new subscription with the mock saver providing the bookmark XML
	mockSaver := newMockSaverWithBookmark(bookmarkXML)
	sub2, err := startSubscription(s.T(), s.ti, s.channelPath,
		WithBookmarkSaver(mockSaver),
		WithStartMode("now"))
	require.NoError(s.T(), err)

	// Verify Load was called
	mockSaver.AssertExpectations(s.T())

	// Should only read the new events after the bookmark
	_, err = getEventHandles(s.T(), s.ti, sub2, s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub2)
	require.NoError(s.T(), err)
}

// Test that "oldest" mode doesn't create initial bookmark
func (s *GetEventsTestSuite) TestInitializeBookmark_StartModeOldest() {
	mockSaver := new(evtbookmark.MockSaver)
	mockSaver.On("Load").Return("", nil).Once()

	// Create subscription with "oldest" mode and empty log
	sub, err := startSubscription(s.T(), s.ti, s.channelPath,
		WithBookmarkSaver(mockSaver),
		WithStartMode("oldest"))
	require.NoError(s.T(), err)

	// Verify Load was called but Save was not called for 'oldest' mode with empty log
	mockSaver.AssertExpectations(s.T())
	mockSaver.AssertNotCalled(s.T(), "Save", mock.Anything)

	// Generate events
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Should read all events
	_, err = getEventHandles(s.T(), s.ti, sub, s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)
}

// Test "now" mode creates bookmark from latest event when events exist
func (s *GetEventsTestSuite) TestInitializeBookmark_StartModeNowWithEvents() {
	// Generate initial events
	err := s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	mockSaver := new(evtbookmark.MockSaver)
	mockSaver.On("Load").Return("", nil).Once()
	// Expect Save to be called with any bookmark XML
	var savedBookmark string
	mockSaver.On("Save", mock.MatchedBy(func(xml string) bool {
		savedBookmark = xml
		return len(xml) > 0
	})).Return(nil).Once()

	// Create subscription with "now" mode
	sub, err := startSubscription(s.T(), s.ti, s.channelPath,
		WithBookmarkSaver(mockSaver),
		WithStartMode("now"))
	require.NoError(s.T(), err)

	// Verify bookmark was created and persisted
	mockSaver.AssertExpectations(s.T())
	require.NotEmpty(s.T(), savedBookmark, "Bookmark should be saved")

	// Should not read old events
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)

	sub.Stop()

	// Generate more events while stopped
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Restart with the saved bookmark XML
	mockSaver2 := newMockSaverWithBookmark(savedBookmark)
	sub2, err := startSubscription(s.T(), s.ti, s.channelPath,
		WithBookmarkSaver(mockSaver2),
		WithStartMode("now"))
	require.NoError(s.T(), err)
	defer sub2.Stop()

	// Verify it loaded the persisted bookmark
	mockSaver2.AssertExpectations(s.T())

	// Should only read new events
	_, err = getEventHandles(s.T(), s.ti, sub2, s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub2)
	require.NoError(s.T(), err)
}

// Test "now" mode behavior with empty log (no matching events)
func (s *GetEventsTestSuite) TestInitializeBookmark_StartModeNowEmptyLog() {
	mockSaver := new(evtbookmark.MockSaver)
	mockSaver.On("Load").Return("", nil).Once()
	// Expect Save to be called with empty bookmark XML
	var savedBookmark string
	mockSaver.On("Save", mock.MatchedBy(func(xml string) bool {
		savedBookmark = xml
		return xml == "<BookmarkList>\r\n</BookmarkList>"
	})).Return(nil).Once()

	// Start with empty log
	sub, err := startSubscription(s.T(), s.ti, s.channelPath,
		WithBookmarkSaver(mockSaver),
		WithStartMode("now"))
	require.NoError(s.T(), err)
	defer sub.Stop()

	// Verify empty bookmark is saved (per ErrNoMatchingEvents handling)
	mockSaver.AssertExpectations(s.T())

	// stop the subscription
	sub.Stop()

	// Generate events while subscription is stopped
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Restart with the saved bookmark XML
	mockSaver2 := newMockSaverWithBookmark(savedBookmark)
	sub2, err := startSubscription(s.T(), s.ti, s.channelPath,
		WithBookmarkSaver(mockSaver2),
		WithStartMode("now"))
	require.NoError(s.T(), err)
	defer sub2.Stop()

	// Verify it loaded the persisted bookmark
	mockSaver2.AssertExpectations(s.T())

	// Should read the events
	_, err = getEventHandles(s.T(), s.ti, sub2, s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub2)
	require.NoError(s.T(), err)
}

// Test fallback behavior when bookmark loading fails
func (s *GetEventsTestSuite) TestInitializeBookmark_LoadFailureFallbackToNow() {
	// Generate events before subscription starts
	err := s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	mockSaver := new(evtbookmark.MockSaver)
	// Simulate load error
	mockSaver.On("Load").Return("", errors.New("simulated load error")).Once()
	// After load fails, should fall back to creating bookmark from latest event
	mockSaver.On("Save", mock.MatchedBy(func(xml string) bool {
		return strings.Contains(xml, "RecordId=")
	})).Return(nil).Once()

	// Create subscription with "now" mode
	sub, err := startSubscription(s.T(), s.ti, s.channelPath,
		WithBookmarkSaver(mockSaver),
		WithStartMode("now"))
	require.NoError(s.T(), err)
	defer sub.Stop()

	// Verify expectations
	mockSaver.AssertExpectations(s.T())

	// Should not read old events (started from latest bookmark)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)
}

// Test that subscription works correctly when no saver is provided
func (s *GetEventsTestSuite) TestInitializeBookmark_NoSaverProvided() {
	// Generate events
	err := s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Create subscription without saver, using default behavior (future events)
	sub, err := startSubscription(s.T(), s.ti, s.channelPath)
	require.NoError(s.T(), err)
	defer sub.Stop()

	// Should not read old events (default is EvtSubscribeToFutureEvents)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)

	// Generate new events
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Should collect new events
	_, err = getEventHandles(s.T(), s.ti, sub, s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)
}

// Test that stopping and restarting the same subscription retains bookmark via saver
func (s *GetEventsTestSuite) TestStartStopRestart_RetainsBookmarkWithSaver() {
	// Generate initial events
	err := s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Create a mock saver to track bookmark updates
	mockSaver := new(evtbookmark.MockSaver)
	mockSaver.On("Load").Return("", nil).Once() // First start, no saved bookmark

	// Create subscription with bookmarksaver using "oldest" mode
	sub := NewPullSubscription(
		s.channelPath,
		"*",
		WithWindowsEventLogAPI(s.ti.API()),
		WithBookmarkSaver(mockSaver),
		WithStartMode("oldest"))

	// Start subscription
	err = sub.Start()
	require.NoError(s.T(), err)
	defer sub.Stop()

	// Verify initial Load was called
	mockSaver.AssertExpectations(s.T())

	// Read all the initial events and save bookmark
	events, err := getEventHandles(s.T(), s.ti, sub, s.numEvents)
	require.NoError(s.T(), err)

	// Create bookmark from last read event
	savedBookmarkXML, err := bookmarkXMLFromEvent(s.ti.API(), events[len(events)-1])
	require.NoError(s.T(), err)

	// Stop subscription
	sub.Stop()

	// Generate more events while stopped
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Restart the same subscription with saved bookmark
	mockSaver.On("Load").Return(savedBookmarkXML, nil).Once()
	err = sub.Start()
	require.NoError(s.T(), err)
	mockSaver.AssertExpectations(s.T())

	// Should only read the new events generated while stopped
	_, err = getEventHandles(s.T(), s.ti, sub, s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)
}

// Test multiple start/stop cycles
func (s *GetEventsTestSuite) TestStartStopRestart_MultipleCycles() {
	var savedBookmark string
	mockSaver := new(evtbookmark.MockSaver)
	mockSaver.On("Load").Return("", nil).Once() // Initial start
	mockSaver.On("Save", mock.MatchedBy(func(xml string) bool {
		savedBookmark = xml
		return len(xml) > 0
	})).Return(nil) // Can be called multiple times

	// Create subscription
	sub := NewPullSubscription(
		s.channelPath,
		"*",
		WithWindowsEventLogAPI(s.ti.API()),
		WithBookmarkSaver(mockSaver),
		WithStartMode("now"))

	// Cycle through multiple start/stop iterations
	for i := 0; i < 3; i++ {
		if i > 0 {
			// Load the saved bookmark on subsequent starts
			mockSaver.On("Load").Return(savedBookmark, nil).Once()
		}

		err := sub.Start()
		require.NoError(s.T(), err, "Cycle %d: Start failed", i)

		// Generate events
		err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
		require.NoError(s.T(), err, "Cycle %d: GenerateEvents failed", i)

		// Read events
		_, err = getEventHandles(s.T(), s.ti, sub, s.numEvents)
		require.NoError(s.T(), err, "Cycle %d: getEventHandles failed", i)

		// Stop
		sub.Stop()
	}

	mockSaver.AssertExpectations(s.T())
}

// Test start/stop without bookmarksaver
func (s *GetEventsTestSuite) TestStartStopRestart_NoSaver() {
	// Generate initial events
	err := s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Create subscription without saver, using oldest mode
	sub := NewPullSubscription(
		s.channelPath,
		"*",
		WithWindowsEventLogAPI(s.ti.API()),
		WithStartMode("oldest"))

	// Start subscription
	err = sub.Start()
	require.NoError(s.T(), err)
	defer sub.Stop()

	// Read all events
	_, err = getEventHandles(s.T(), s.ti, sub, s.numEvents)
	require.NoError(s.T(), err)

	// Stop subscription
	sub.Stop()

	// Generate more events while stopped
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Restart the same subscription
	err = sub.Start()
	require.NoError(s.T(), err)

	// Without saver, should restart from oldest and read ALL events (both batches)
	_, err = getEventHandles(s.T(), s.ti, sub, 2*s.numEvents)
	require.NoError(s.T(), err)
	err = assertNoMoreEvents(s.T(), sub)
	require.NoError(s.T(), err)
}

func TestLaunchGetEventsTestSuite(t *testing.T) {
	testerNames := eventlog_test.GetEnabledAPITesters()

	for _, tiName := range testerNames {
		t.Run(tiName+"API", func(t *testing.T) {
			var s GetEventsTestSuite
			s.channelPath = "dd-test-channel-subscription"
			s.eventSource = "dd-test-source-subscription"
			s.testAPI = tiName
			s.numEvents = 10
			suite.Run(t, &s)
		})
	}
}
