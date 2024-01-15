// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package status

import "sync"

// NewMock returns a Manager that uses plain internal values instead of expvars
func NewMock() Manager {
	return &mockManager{}
}

// mockManager mocks a Manager using plain values (not expvars)
type mockManager struct {
	trapsPackets, trapsPacketsAuthErrors int64
	lock                                 sync.Mutex
}

func (s *mockManager) AddTrapsPackets(i int64) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.trapsPackets += i
}

func (s *mockManager) AddTrapsPacketsAuthErrors(i int64) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.trapsPacketsAuthErrors += i
}

func (s *mockManager) GetTrapsPackets() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.trapsPackets
}

func (s *mockManager) GetTrapsPacketsAuthErrors() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.trapsPacketsAuthErrors
}
