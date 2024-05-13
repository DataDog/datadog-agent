// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package eventlog

import (
	"fmt"
	"testing"
	"time"

	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	evtsubscribe "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/subscription"
	eventlog_test "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// Example usage of the eventlog utility library to get event records from the Windows Event Log
// while using a channel to be notified when new events are available.
func testSubscriptionExample(t testing.TB, ti eventlog_test.APITester, stop chan struct{}, done chan struct{}, channelPath string, numEvents uint) { //nolint:revive // TODO fix revive unused-parameter
	defer close(done)

	// Choose the Windows Event Log API implementation
	// Windows API
	//   "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
	//   api = winevtapi.New()
	// Fake API
	//   "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/fake"
	//   api = fakeevtapi.New()
	// For this test the API implementation is selected by the test runner
	api := ti.API()

	// Create the subscription
	sub := evtsubscribe.NewPullSubscription(
		channelPath,
		"*",
		evtsubscribe.WithStartAtOldestRecord(),
		evtsubscribe.WithWindowsEventLogAPI(api))

	// Start the subscription
	err := sub.Start()
	if !assert.NoError(t, err) {
		return
	}
	// Cleanup the subscription when done
	defer sub.Stop()

	// Get events until stop is set
outerLoop:
	for {
		select {
		case <-stop:
			break outerLoop
		case events, ok := <-sub.GetEvents():
			if !ok {
				// The channel is closed, this indicates an error or that sub.Stop() was called
				// Use sub.Error() to get the error, if any.
				err = sub.Error()
				if err != nil {
					// If there is an error, you must stop the subscription. It is possible to resume
					// the subscription by calling sub.Start() again.
					sub.Stop()
				}
				break outerLoop
			}
			// handle the events
			for _, eventRecord := range events {
				// do something with the event
				// ...
				err = printEventXML(api, eventRecord)
				assert.NoError(t, err)
				err = printEventValues(api, eventRecord)
				assert.NoError(t, err)
				// close the event when done
				evtapi.EvtCloseRecord(api, eventRecord.EventRecordHandle)
			}
		}
	}
}

// Example usage of the eventlog utility library to render the entire event as XML
func printEventXML(api evtapi.API, event *evtapi.EventRecord) error {
	xml, err := api.EvtRenderEventXml(event.EventRecordHandle)
	if err != nil {
		return fmt.Errorf("failed to render event XML: %w", err)
	}

	fmt.Printf("%s\n", windows.UTF16ToString(xml))
	return nil
}

// Example usage of the eventlog utility library to render specific values from the event
func printEventValues(api evtapi.API, event *evtapi.EventRecord) error {
	// Create render context for the System values
	// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_system_property_id
	c, err := api.EvtCreateRenderContext(nil, evtapi.EvtRenderContextSystem)
	if err != nil {
		return fmt.Errorf("failed to create render context: %w", err)
	}
	defer evtapi.EvtCloseRenderContext(api, c)

	// Render the values
	vals, err := api.EvtRenderEventValues(c, event.EventRecordHandle)
	if err != nil {
		return fmt.Errorf("failed to render values: %w", err)
	}
	defer vals.Close()

	// EventID
	eventid, err := vals.UInt(evtapi.EvtSystemEventID)
	if err != nil {
		return fmt.Errorf("failed to get eventid value: %w", err)
	}
	fmt.Printf("eventid: %d\n", eventid)

	// Provider
	provider, err := vals.String(evtapi.EvtSystemProviderName)
	if err != nil {
		return fmt.Errorf("failed to get provider name value: %w", err)
	}
	fmt.Printf("provider name: %s\n", provider)

	// Computer
	computer, err := vals.String(evtapi.EvtSystemComputer)
	if err != nil {
		return fmt.Errorf("failed to get computer name value: %w", err)
	}
	fmt.Printf("computer name: %s\n", computer)

	// Time Created
	ts, err := vals.Time(evtapi.EvtSystemTimeCreated)
	if err != nil {
		return fmt.Errorf("failed to get time created value: %w", err)
	}
	fmt.Printf("time created: %d\n", ts)

	// Level
	level, err := vals.UInt(evtapi.EvtSystemLevel)
	if err != nil {
		return fmt.Errorf("failed to get level value: %w", err)
	}
	fmt.Printf("level: %d\n", level)

	// Format Message
	pm, err := api.EvtOpenPublisherMetadata(provider, "")
	if err != nil {
		return fmt.Errorf("failed to open provider metadata: %w", err)
	}
	defer evtapi.EvtClosePublisherMetadata(api, pm)

	message, err := api.EvtFormatMessage(pm, event.EventRecordHandle, 0, nil, evtapi.EvtFormatMessageEvent)
	if err != nil {
		return fmt.Errorf("failed to format event message: %w", err)
	}
	fmt.Printf("message: %s\n", message)

	return nil
}

// test helper function that sets up an event log for the test
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

// tests our example implementation in testSubscriptionExample can read events
func TestSubscriptionExample(t *testing.T) {
	testInterfaceNames := eventlog_test.GetEnabledAPITesters()

	channelPath := "dd-test-channel-example"
	eventSource := "dd-test-source-example"
	numEvents := uint(10)
	for _, tiName := range testInterfaceNames {
		t.Run(fmt.Sprintf("%sAPI", tiName), func(t *testing.T) {
			if tiName == "Fake" {
				t.Skip("Fake API does not implement EvtRenderValues")
			}
			ti := eventlog_test.GetAPITesterByName(tiName, t)
			// Create some test events
			createLog(t, ti, channelPath, eventSource)
			err := ti.GenerateEvents(eventSource, numEvents)
			require.NoError(t, err)
			// Create stop channel to use as example of an external signal to shutdown
			stop := make(chan struct{})
			done := make(chan struct{})

			// Start our example implementation
			go testSubscriptionExample(t, ti, stop, done, channelPath, numEvents)

			// Create some test events while that's running
			for i := 0; i < 3; i++ {
				err := ti.GenerateEvents(eventSource, numEvents)
				require.NoError(t, err)
				// simulate some delay in event generation
				time.Sleep(100 * time.Millisecond)
			}
			// Stop the event collector
			close(stop)
			<-done
		})
	}
}
