// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package replay

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/packets"
)

type mockTrafficCapture struct {
	isRunning bool
	sync.RWMutex
}

func newMockTrafficCapture() Component {
	return &mockTrafficCapture{}
}

func (tc *mockTrafficCapture) Configure() error {
	return nil
}

func (tc *mockTrafficCapture) IsOngoing() bool {
	tc.Lock()
	defer tc.Unlock()
	return tc.isRunning
}

func (tc *mockTrafficCapture) Start(p string, d time.Duration, compressed bool) (string, error) {
	tc.Lock()
	defer tc.Unlock()
	tc.isRunning = true
	return "", nil

}

func (tc *mockTrafficCapture) Stop() {
	tc.Lock()
	defer tc.Unlock()
	tc.isRunning = false
}

func (tc *mockTrafficCapture) RegisterSharedPoolManager(p *packets.PoolManager) error {
	return nil
}

func (tc *mockTrafficCapture) RegisterOOBPoolManager(p *packets.PoolManager) error {
	return nil
}

func (tc *mockTrafficCapture) Enqueue(msg *CaptureBuffer) bool {
	return true
}
