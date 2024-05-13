// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package replay

import (
	"context"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
)

type mockTrafficCapture struct {
	isRunning bool
	sync.RWMutex
}

func newMockTrafficCapture(deps dependencies) Component {
	tc := &mockTrafficCapture{}
	deps.Lc.Append(fx.Hook{
		OnStart: tc.configure,
	})
	return tc
}

func (tc *mockTrafficCapture) configure(_ context.Context) error {
	return nil
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
func (tc *mockTrafficCapture) RegisterSharedPoolManager(p *packets.PoolManager) error {
	return nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (tc *mockTrafficCapture) RegisterOOBPoolManager(p *packets.PoolManager) error {
	return nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (tc *mockTrafficCapture) Enqueue(msg *CaptureBuffer) bool {
	return true
}

//nolint:revive // TODO(AML) Fix revive linter
func (tc *mockTrafficCapture) GetStartUpError() error {
	return nil
}
