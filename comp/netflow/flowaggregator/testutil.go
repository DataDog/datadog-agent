// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"fmt"
	"time"
)

// WaitForFlowsToBeFlushed waits up to timeoutDuration for at least minEvents
// flows to be flushed by the aggregator. It is intended for testing.
func WaitForFlowsToBeFlushed(aggregator *FlowAggregator, timeoutDuration time.Duration, minEvents uint64) (uint64, error) {
	timeout := time.After(timeoutDuration)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	// Keep trying until we're timed out or got a result or got an error
	for {
		select {
		// Got a timeout! fail with a timeout error
		case <-timeout:
			return 0, fmt.Errorf("timeout error waiting for events")
		// Got a tick, we should check on doSomething()
		case <-ticker.C:
			events := aggregator.flushedFlowCount.Load()
			if events >= minEvents {
				return events, nil
			}
		}
	}
}
