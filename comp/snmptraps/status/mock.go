// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package status

import "sync"

// mockService mocks a service using plain values (not expvars)
type mockService struct {
	trapsPackets, trapsPacketsAuthErrors int64
	lock                                 sync.Mutex
}

func (s *mockService) AddTrapsPackets(i int64) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.trapsPackets += i
}

func (s *mockService) AddTrapsPacketsAuthErrors(i int64) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.trapsPacketsAuthErrors += i
}

func (s *mockService) GetTrapsPackets() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.trapsPackets
}

func (s *mockService) GetTrapsPacketsAuthErrors() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.trapsPacketsAuthErrors
}
