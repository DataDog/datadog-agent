// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

// Package implnoop implements a no-op version of the component
package implnoop

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	replaydef "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
)

// NewNoopTrafficCapture creates a new noopTrafficCapture instance
func NewNoopTrafficCapture() replaydef.Component {
	return &noopTrafficCapture{}
}

type noopTrafficCapture struct {
	isRunning bool
	sync.RWMutex
}

func (tc *noopTrafficCapture) IsOngoing() bool {
	tc.Lock()
	defer tc.Unlock()
	return tc.isRunning
}

// StartCapture sets isRunning to true
func (tc *noopTrafficCapture) StartCapture(_ string, _ time.Duration, _ bool) (string, error) {
	tc.Lock()
	defer tc.Unlock()
	tc.isRunning = true
	return "", nil

}

// StopCapture sets isRunning to false
func (tc *noopTrafficCapture) StopCapture() {
	tc.Lock()
	defer tc.Unlock()
	tc.isRunning = false
}

// RegisterSharedPoolManager does nothing
func (tc *noopTrafficCapture) RegisterSharedPoolManager(_ *packets.PoolManager[packets.Packet]) error {
	return nil
}

// RegisterOOBPoolManager does nothing
func (tc *noopTrafficCapture) RegisterOOBPoolManager(_ *packets.PoolManager[[]byte]) error {
	return nil
}

// Enqueue does nothing
func (tc *noopTrafficCapture) Enqueue(_ *replaydef.CaptureBuffer) bool {
	return true
}

// GetStartUpError returns nil
func (tc *noopTrafficCapture) GetStartUpError() error {
	return nil
}
