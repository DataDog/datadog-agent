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
)

type TrafficCapture struct {
	Writer *TrafficCaptureWriter

	sync.RWMutex
}

func NewTrafficCapture() (*TrafficCapture, error) {
	location := config.Datadog.GetString("dogstatsd_uds_capture_path")
	writer, err := NewTrafficCaptureWriter(location, config.Datadog.GetInt("dogstatsd_uds_capture_depth"))
	if err != nil {
		return nil, err
	}

	tc := &TrafficCapture{
		Writer: writer,
	}

	return tc, nil
}

func (tc *TrafficCapture) IsOngoing() bool {
	tc.RLock()
	defer tc.RUnlock()

	if tc.Writer == nil {
		return false
	}

	return tc.Writer.IsOngoing()
}

func (tc *TrafficCapture) Start(d time.Duration) error {
	if tc.IsOngoing() {
		return fmt.Errorf("Ongoing capture in progress")
	}

	go tc.Writer.Capture(d)

	return nil

}

func (tc *TrafficCapture) Stop() error {
	tc.Lock()
	defer tc.Unlock()

	err := tc.Writer.StopCapture()
	if err != nil {
		return nil
	}

	return nil
}

func (tc *TrafficCapture) Path() (string, error) {
	tc.RLock()
	defer tc.RUnlock()

	return tc.Writer.Path()
}
