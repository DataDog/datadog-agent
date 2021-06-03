// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package replay

import (
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
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
	Writer *TrafficCaptureWriter

	sync.RWMutex
}

// NewTrafficCapture creates a TrafficCapture instance.
func NewTrafficCapture() (*TrafficCapture, error) {
	location := config.Datadog.GetString("dogstatsd_capture_path")
	if location == "" {
		location = path.Join(config.Datadog.GetString("run_path"), "dsd_capture")
	}
	writer := NewTrafficCaptureWriter(location, config.Datadog.GetInt("dogstatsd_capture_depth"))
	if writer == nil {
		return nil, fmt.Errorf("unable to instantiate capture writer")
	}

	tc := &TrafficCapture{
		Writer: writer,
	}

	return tc, nil
}

// IsOngoing returns whether a capture is ongoing for this TrafficCapture instance.
func (tc *TrafficCapture) IsOngoing() bool {
	tc.RLock()
	defer tc.RUnlock()

	if tc.Writer == nil {
		return false
	}

	return tc.Writer.IsOngoing()
}

// Start starts a TrafficCapture and returns an error in the event of an issue.
func (tc *TrafficCapture) Start(d time.Duration) error {
	if tc.IsOngoing() {
		return fmt.Errorf("Ongoing capture in progress")
	}

	go tc.Writer.Capture(d)

	return nil

}

// Stop stops an ongoing TrafficCapture.
func (tc *TrafficCapture) Stop() {
	tc.Lock()
	defer tc.Unlock()

	tc.Writer.StopCapture()
}

// Path returns the path to the underlying TrafficCapture file, and an error if any.
func (tc *TrafficCapture) Path() (string, error) {
	tc.RLock()
	defer tc.RUnlock()

	return tc.Writer.Path()
}
