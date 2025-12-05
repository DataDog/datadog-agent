// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"errors"
	"time"
)

// WaitForFlowsToBeFlushed waits up to timeoutDuration for at least minEvents
// flows to be flushed by the aggregator. It is intended for testing.
func WaitForFlowsToBeFlushed(aggregator *FlowAggregator, timeoutDuration time.Duration, minEvents uint64) (uint64, error) {
	timeout := time.After(timeoutDuration)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	// Keep trying until we're timed out or got a result or got an error
	for {
		select {
		// Got a timeout! fail with a timeout error
		case <-timeout:
			return 0, errors.New("timeout error waiting for events")
		// Got a tick, we should check on doSomething()
		case <-ticker.C:
			events := aggregator.flushedFlowCount.Load()
			if events >= minEvents {
				return events, nil
			}
		}
	}
}

// WaitForFlowsToAccumulate waits up to timeoutDuration for at least minEvents
// flows to be flushed by the aggregator. It is intended for testing.
func WaitForFlowsToAccumulate(aggregator *FlowAggregator, timeoutDuration time.Duration, minFlows int) error {
	timeout := time.After(timeoutDuration)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	// Keep trying until we're timed out or got a result or got an error
	for {
		select {
		// Got a timeout! fail with a timeout error
		case <-timeout:
			return errors.New("timeout error waiting for events")
		// Got a tick, we should check on doSomething()
		case <-ticker.C:
			// more hacky mutex locking, need to verify that flows accumulated by reading shared memory
			aggregator.flowAcc.flowsMutex.Lock()
			if len(aggregator.flowAcc.flows) >= minFlows {
				aggregator.flowAcc.flowsMutex.Unlock()
				return nil
			}
			aggregator.flowAcc.flowsMutex.Unlock()
		}
	}
}

// SetAggregatorTicker sets up a mock ticker for the aggregator. It returns the channels that the aggregator will use
// in its flushLoop. This implementation is tied to the order that the aggregator requests channels, if that changes
// this is likely broken.
func SetAggregatorTicker(agg *FlowAggregator) (chan time.Time, chan time.Time) {
	callCount := 0
	flushChannel := make(chan time.Time)
	rollupChannel := make(chan time.Time)
	agg.NewTicker = func(_ time.Duration) <-chan time.Time {
		callCount++
		// this isn't great logic, but it's the best we can do for now. This is highly coupled to the order that we create
		// tickers in FlowAggregator.flushLoop.
		switch callCount {
		case 1:
			return flushChannel
		case 2:
			return rollupChannel
		default:
			panic("unexpected call to NewTicker")
		}
	}

	return flushChannel, rollupChannel
}
