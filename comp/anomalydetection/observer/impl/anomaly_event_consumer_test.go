// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/checksfit"
	"github.com/DataDog/datadog-agent/comp/anomalydetection/checksfit/fitcore"
	anomalydetectionconfig "github.com/DataDog/datadog-agent/comp/anomalydetection/config"
)

// startEventPublisher opens the broadcast ring the consumer subscribes to. The
// Agent never publishes events in production; tests need a publisher because it
// owns the endpoint.
func startEventPublisher(t *testing.T, endpoint fitcore.SetupEndpoint) *fitcore.BroadcastPublisher {
	publisher, err := fitcore.OpenBroadcastPublisher(fitcore.NewBroadcastConfig(endpoint), checksfit.AnomalyEventsDescriptor)
	require.NoError(t, err)
	t.Cleanup(func() { _ = publisher.Close() })
	return publisher
}

func publishEvent(t *testing.T, publisher *fitcore.BroadcastPublisher, event checksfit.AnomalyEvent) {
	payload, err := event.Encode()
	require.NoError(t, err)
	outcome := publisher.SendBatch([]fitcore.Record{{Kind: checksfit.TypeAnomalyEvent, Payload: payload}})
	require.Equal(t, 1, outcome.Accepted, "the publisher waits for a subscriber before accepting")
}

func newEventConsumer(t *testing.T, endpointValue string) *anomalyEventConsumer {
	consumer, err := newAnomalyEventConsumer(anomalydetectionconfig.AnomalyEventsConfig{
		Enabled:       true,
		Endpoint:      endpointValue,
		RetryInterval: 200 * time.Millisecond,
	})
	require.NoError(t, err)
	require.NotNil(t, consumer)
	t.Cleanup(func() { require.NoError(t, consumer.close()) })
	return consumer
}

func TestAnomalyEventConsumerReceivesPublishedEvents(t *testing.T) {
	if !checksfit.Supported {
		t.Skip("FIT is unavailable on this platform")
	}
	endpointValue, endpoint := newFITEndpoint(t)
	consumer := newEventConsumer(t, endpointValue)

	// The consumer starts before its publisher exists and retries, so this
	// publisher binds the endpoint the consumer is polling.
	publisher := startEventPublisher(t, endpoint)

	events := []checksfit.AnomalyEvent{
		{Title: "AAD anomaly: system.load.1", Description: "severity medium->high with 3 contributing series", Timestamp: 1_791_536_046},
		{Title: "AAD debug trigger: debug.trigger-anomaly.check", Description: "value=1 host=h tags=[] channel=0", Timestamp: 1_791_536_047},
	}
	// A send with no subscriber yet waits for one, which is the transport's
	// policy, so publishing must not block this goroutine.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, event := range events {
			payload, err := event.Encode()
			if err != nil {
				return
			}
			publisher.SendBatch([]fitcore.Record{{Kind: checksfit.TypeAnomalyEvent, Payload: payload}})
		}
	}()

	require.Eventually(t, func() bool { return consumer.stats() == 2 }, 30*time.Second, 50*time.Millisecond,
		"the consumer should log both events")
	<-done
	require.Equal(t, uint64(2), consumer.stats())
}

func TestAnomalyEventConsumerCloseReturnsWhileParked(t *testing.T) {
	if !checksfit.Supported {
		t.Skip("FIT is unavailable on this platform")
	}
	endpointValue, endpoint := newFITEndpoint(t)
	consumer := newEventConsumer(t, endpointValue)
	publisher := startEventPublisher(t, endpoint)

	// Subscribe first, then park: one event proves the subscription is live.
	done := make(chan struct{})
	go func() {
		defer close(done)
		publishEventInGoroutine(t, publisher)
	}()
	require.Eventually(t, func() bool { return consumer.stats() == 1 }, 30*time.Second, 50*time.Millisecond)
	<-done

	// The consumer is now waiting for the next event. Closing must cancel that
	// wait instead of leaving the observer's shutdown blocked on it.
	closed := make(chan error, 1)
	go func() { closed <- consumer.close() }()
	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("closing the consumer while it waits for an event did not return")
	}
}

// publishEventInGoroutine publishes one event without failing the test goroutine
// when the send waits for the subscriber.
func publishEventInGoroutine(t *testing.T, publisher *fitcore.BroadcastPublisher) {
	payload, err := checksfit.AnomalyEvent{Title: "first", Description: "first", Timestamp: 1}.Encode()
	if err != nil {
		t.Error(fmt.Errorf("encoding the event: %w", err))
		return
	}
	publisher.SendBatch([]fitcore.Record{{Kind: checksfit.TypeAnomalyEvent, Payload: payload}})
}
