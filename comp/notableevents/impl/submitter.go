// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package notableeventsimpl

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// eventPayload represents a Windows Event Log event to be submitted
// TODO(WINA-1968): TBD format for event payload, finish with intake.
type eventPayload struct {
	Channel string
	EventID uint
}

// submitter receives event payloads from a channel and drains them to prevent blocking the collector
type submitter struct {
	// in
	inChan <-chan eventPayload
	// internal
	wg sync.WaitGroup
}

// newSubmitter creates a new submitter instance
func newSubmitter(inChan <-chan eventPayload) *submitter {
	return &submitter{
		inChan: inChan,
	}
}

// start begins processing events from the input channel
func (s *submitter) start() {
	s.wg.Add(1)
	go s.run()
}

// stop waits for the submitter to finish draining the channel
func (s *submitter) stop() {
	s.wg.Wait()
}

// run is the main loop that drains events from the channel
func (s *submitter) run() {
	defer s.wg.Done()

	for payload := range s.inChan {
		// For now, just log the event to prevent blocking the collector
		log.Debugf("Received notable event: channel=%s, event_id=%d", payload.Channel, payload.EventID)
	}

	log.Info("Notable events submitter input channel closed, shutting down")
}
