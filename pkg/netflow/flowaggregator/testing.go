// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"fmt"
	"time"
)

func WaitForFlowsToBeFlushed(aggregator *FlowAggregator, timeoutDuration time.Duration, minEvents uint64) (uint64, error) {
	timeout := time.After(timeoutDuration)
	tick := time.Tick(500 * time.Millisecond)
	// Keep trying until we're timed out or got a result or got an error
	for {
		select {
		// Got a timeout! fail with a timeout error
		case <-timeout:
			return 0, fmt.Errorf("timeout error waiting for events")
		// Got a tick, we should check on doSomething()
		case <-tick:
			events := aggregator.flushedFlowCount.Load()
			if events >= minEvents {
				return events, nil
			}
		}
	}
}
