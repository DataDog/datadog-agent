// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

//go:build test

//nolint:revive // TODO(AML) Fix revive linter
package mock

import (
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
)

// Mock implements mock-specific methods.
type Mock interface {
	replay.Component
}

//nolint:revive // TODO(AML) Fix revive linter
type Requires struct {
	T testing.TB
}

//nolint:revive // TODO(AML) Fix revive linter
func NewTrafficCapture(deps Requires) replay.Component {
	tc := &mockTrafficCapture{}
	return tc
}

type mockTrafficCapture struct {
	isRunning bool
	sync.RWMutex
}

func (tc *mockTrafficCapture) IsOngoing() bool {
	tc.Lock()
	defer tc.Unlock()
	return tc.isRunning
}

// StartCapture does nothign on the mock
func (tc *mockTrafficCapture) StartCapture(_ string, _ time.Duration, _ bool) (string, error) {
	tc.Lock()
	defer tc.Unlock()
	tc.isRunning = true
	return "", nil

}

// StopCapture does nothign on the mock
func (tc *mockTrafficCapture) StopCapture() {
	tc.Lock()
	defer tc.Unlock()
	tc.isRunning = false
}

//nolint:revive // TODO(AML) Fix revive linter
func (tc *mockTrafficCapture) RegisterSharedPoolManager(p *packets.PoolManager[packets.Packet]) error {
	return nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (tc *mockTrafficCapture) RegisterOOBPoolManager(p *packets.PoolManager[[]byte]) error {
	return nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (tc *mockTrafficCapture) Enqueue(msg *replay.CaptureBuffer) bool {
	return true
}

//nolint:revive // TODO(AML) Fix revive linter
func (tc *mockTrafficCapture) GetStartUpError() error {
	return nil
}
