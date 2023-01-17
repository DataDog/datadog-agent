// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package replay

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/packets"
)

const (
	// GUID will be used as the GUID during capture replays
	// This is a magic number chosen for no particular reason other than the fact its
	// quite large an improbable to match an actual Group ID on any given box. We
	// need this number to identify replayed Unix socket ancillary credentials.
	GUID = 999888777
)

// TrafficCapture allows capturing traffic from our listeners and writing it to file
type TrafficCapture struct {
	writer *TrafficCaptureWriter

	sync.RWMutex
}

// NewTrafficCapture creates a TrafficCapture instance.
func NewTrafficCapture() (*TrafficCapture, error) {
	writer := NewTrafficCaptureWriter(config.Datadog.GetInt("dogstatsd_capture_depth"))
	if writer == nil {
		return nil, fmt.Errorf("unable to instantiate capture writer")
	}

	tc := &TrafficCapture{
		writer: writer,
	}

	return tc, nil
}

// IsOngoing returns whether a capture is ongoing for this TrafficCapture instance.
func (tc *TrafficCapture) IsOngoing() bool {
	tc.RLock()
	defer tc.RUnlock()

	if tc.writer == nil {
		return false
	}

	return tc.writer.IsOngoing()
}

// Start starts a TrafficCapture and returns an error in the event of an issue.
func (tc *TrafficCapture) Start(p string, d time.Duration, compressed bool) error {
	if tc.IsOngoing() {
		return fmt.Errorf("Ongoing capture in progress")
	}

	_, err := ValidateLocation(p)
	if err != nil {
		return err
	}

	go tc.writer.Capture(p, d, compressed)

	return nil

}

// Stop stops an ongoing TrafficCapture.
func (tc *TrafficCapture) Stop() {
	tc.Lock()
	defer tc.Unlock()

	tc.writer.StopCapture()
}

// Path returns the path to the underlying TrafficCapture file, and an error if any.
func (tc *TrafficCapture) Path() (string, error) {
	tc.RLock()
	defer tc.RUnlock()

	return tc.writer.Path()
}

// RegisterSharedPoolManager registers the shared pool manager with the TrafficCapture.
func (tc *TrafficCapture) RegisterSharedPoolManager(p *packets.PoolManager) error {
	tc.Lock()
	defer tc.Unlock()
	return tc.writer.RegisterSharedPoolManager(p)
}

// RegisterOOBPoolManager registers the OOB shared pool manager with the TrafficCapture.
func (tc *TrafficCapture) RegisterOOBPoolManager(p *packets.PoolManager) error {
	tc.Lock()
	defer tc.Unlock()
	return tc.writer.RegisterOOBPoolManager(p)
}

// Enqueue enqueues a capture buffer so it's written to file.
func (tc *TrafficCapture) Enqueue(msg *CaptureBuffer) bool {
	tc.RLock()
	defer tc.RUnlock()
	return tc.writer.Enqueue(msg)
}
