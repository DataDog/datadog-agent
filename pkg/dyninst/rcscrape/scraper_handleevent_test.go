// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package rcscrape

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

// Test that scraperSink.HandleEvent swallows errors and clears debouncer state.
func TestHandleEventErrorClearsDebouncer(t *testing.T) {
	// Minimal scraper with initialized internals.
	s := &Scraper{}
	s.mu.debouncer = makeDebouncer(1 * time.Millisecond)
	s.mu.processes = make(map[actuator.ProcessID]*trackedProcess)

	pid := actuator.ProcessID{PID: 123}
	s.mu.processes[pid] = &trackedProcess{}

	// Prepopulate debouncer state for pid.
	now := time.Now()
	s.mu.debouncer.addUpdate(now, pid, remoteConfigFile{
		RuntimeID:     "rid",
		ConfigPath:    "path",
		ConfigContent: "content",
	})

	// Build a sink with a zero-value decoder and an invalid event to force an error
	// from getEventDecoder (no data items in the event).
	sink := &scraperSink{
		scraper:   s,
		decoder:   &decoder{},
		processID: pid,
	}
	var ev output.Event // zero-length event triggers header parse error

	// HandleEvent should not return an error (it is swallowed) and should clear
	// the debouncer for this process.
	err := sink.HandleEvent(ev)
	require.NoError(t, err)

	updates := s.mu.debouncer.getUpdates(now.Add(1 * time.Hour))
	require.Len(t, updates, 0, "debouncer state was not cleared on error")
}
