// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package statusimpl

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/snmptraps/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// MockModule defines a fake Component
var MockModule = fxutil.Component(
	fx.Provide(newMock),
)

// newMock returns a Component that uses plain internal values instead of expvars
func newMock() status.Component {
	return &mockManager{}
}

// mockManager mocks a manager using plain values (not expvars)
type mockManager struct {
	trapsPackets, trapsPacketsUnknownCommunityString int64
	lock                                             sync.Mutex
	err                                              error
}

func (s *mockManager) AddTrapsPackets(i int64) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.trapsPackets += i
}

func (s *mockManager) AddTrapsPacketsUnknownCommunityString(i int64) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.trapsPacketsUnknownCommunityString += i
}

func (s *mockManager) GetTrapsPackets() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.trapsPackets
}

func (s *mockManager) GetTrapsPacketsUnknownCommunityString() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.trapsPacketsUnknownCommunityString
}

func (s *mockManager) SetStartError(err error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.err = err
}

func (s *mockManager) GetStartError() error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.err
}
