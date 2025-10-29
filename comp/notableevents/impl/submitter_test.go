// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package notableeventsimpl

import (
	"testing"
)

func TestSubmitter_DrainChannel(_ *testing.T) {
	// Create event channel
	eventChan := make(chan eventPayload)

	// Create submitter
	sub := newSubmitter(eventChan)

	// Start submitter
	sub.start()

	// Send multiple test events
	numEvents := 5
	for i := 0; i < numEvents; i++ {
		eventChan <- eventPayload{
			Channel: "System",
			EventID: uint(7040 + i),
		}
	}

	// Stop submitter
	close(eventChan)
	sub.stop()
}
