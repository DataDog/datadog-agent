// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package replay

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
)

type mockTrafficCapture struct {
	isRunning bool
	sync.RWMutex
}

func newMockTrafficCapture() Component {
	panic("not called")
}

func (tc *mockTrafficCapture) Configure() error {
	panic("not called")
}

func (tc *mockTrafficCapture) IsOngoing() bool {
	panic("not called")
}

//nolint:revive // TODO(AML) Fix revive linter
func (tc *mockTrafficCapture) Start(p string, d time.Duration, compressed bool) (string, error) {
	panic("not called")
}

//nolint:revive // TODO(AML) Fix revive linter
func (tc *mockTrafficCapture) Stop() {
	panic("not called")
}

//nolint:revive // TODO(AML) Fix revive linter
func (tc *mockTrafficCapture) RegisterSharedPoolManager(p *packets.PoolManager) error {
	panic("not called")
}

//nolint:revive // TODO(AML) Fix revive linter
func (tc *mockTrafficCapture) RegisterOOBPoolManager(p *packets.PoolManager) error {
	panic("not called")
}

//nolint:revive // TODO(AML) Fix revive linter
func (tc *mockTrafficCapture) Enqueue(msg *CaptureBuffer) bool {
	panic("not called")
}
