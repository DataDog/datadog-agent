// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package service

import (
	"context"
	"github.com/benbjohnson/clock"
	"time"
)

// MockDiscoveryClock gives cross-package integration tests controlled scheduling
// of the real RC/AAA HTTP clients. It injects no status or scope observations.
// Call before starting any worker; drive the returned functions serially.
func MockDiscoveryClock(s *CoreAgentService, clk clock.Clock) (func(), func(context.Context)) {
	s.clock = clk
	s.orgStatusPoller.clock = clk
	s.parScopePoller.clock = clk
	s.parScopePoller.jitter = func(time.Duration) time.Duration { return 0 }
	s.parScopePoller.next = clk.Now()
	return func() { s.orgStatusPoller.poll(s.api, s.rcType) }, s.parScopePoller.step
}

// MockOrgStatus binds an INJECTED observation for consumer tests, not production.
func MockOrgStatus(key, endpoint string, value OrgStatus) OrgStatus {
	value.binding = orgStatusBinding(key, endpoint)
	return value
}

// MockPARScope binds an INJECTED grant; it is not evidence of an AAA request.
func MockPARScope(key, endpoint string, value PARScopeObservation) PARScopeObservation {
	value.binding = orgStatusBinding(key, endpoint)
	return value
}
