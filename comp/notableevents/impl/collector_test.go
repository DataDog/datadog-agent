// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package notableeventsimpl

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventlog_test "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/test"
)

func TestCollector_CollectEvents(t *testing.T) {
	ctx := t.Context()

	// only run test with Windows API tester, needs event rendering
	enabledAPIs := eventlog_test.GetEnabledAPITesters()
	if !slices.Contains(enabledAPIs, "Windows") {
		t.Skip("Windows API tester not enabled")
	}
	ti := eventlog_test.GetAPITesterByName("Windows", t)

	// Create test channel
	channelPath := "dd-test-notable-events"
	eventSource := "dd-test-source"

	// Set up test log
	err := ti.InstallChannel(channelPath)
	require.NoError(t, err)
	defer ti.RemoveChannel(channelPath)

	err = ti.API().EvtClearLog(channelPath)
	require.NoError(t, err)

	err = ti.InstallSource(channelPath, eventSource)
	require.NoError(t, err)
	defer ti.RemoveSource(channelPath, eventSource)

	// Create output channel
	outChan := make(chan eventPayload)

	// Create collector using constructor
	collector := newCollector(outChan)

	// Customize for testing with test API and test channel
	collector.api = ti.API()
	collector.channelPath = channelPath
	collector.query = "*" // Collect all events from the test channel

	// Start collector
	err = collector.start()
	require.NoError(t, err)
	defer collector.stop()

	// Wait for subscription to be running
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		if collector.sub.Running() {
			break
		}
		select {
		case <-ticker.C:
			// Continue waiting
		case <-ctx.Done():
			t.Fatal("Context cancelled while waiting for subscription to start")
		}
	}

	// Generate test events using helper
	err = ti.GenerateEvents(eventSource, 3)
	require.NoError(t, err)

	// Wait for events to be collected and sent to output channel
	var receivedEvents []eventPayload

	for i := 0; i < 3; i++ {
		select {
		case payload := <-outChan:
			receivedEvents = append(receivedEvents, payload)
		case <-ctx.Done():
			t.Fatalf("Context cancelled while waiting for event %d. Received %d events so far", i+1, len(receivedEvents))
		}
	}

	// Verify we received all expected events
	require.Len(t, receivedEvents, 3, "Should have received 3 events")

	// Verify event payloads have correct channel
	for i, event := range receivedEvents {
		assert.Equal(t, channelPath, event.Channel, "Event %d should have correct channel", i)
		assert.NotZero(t, event.EventID, "Event %d should have non-zero Event ID", i)
	}

	// Verify no more events are in the channel
	select {
	case unexpected := <-outChan:
		t.Fatalf("Received unexpected event: %+v", unexpected)
	case <-time.After(100 * time.Millisecond):
		// Expected - no more events
	}
}
