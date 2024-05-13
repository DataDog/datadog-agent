// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package npschedulerimpl

import (
	"time"
)

func waitForProcessedPathtests(npScheduler *npSchedulerImpl, processecCount uint64) {
	timeout := time.After(5 * time.Second)
	tick := time.Tick(500 * time.Millisecond)
	// Keep trying until we're timed out or got a result or got an error
	for {
		select {
		case <-timeout:
			return
		case <-tick:
			if npScheduler.processedCount.Load() >= processecCount {
				return
			}
		}
	}
}
