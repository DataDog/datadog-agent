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

// MockDiscoveryClock controls scheduling of real RC/AAA clients in integration
// tests. It injects no observations. Call before starting any worker.
func MockDiscoveryClock(s *CoreAgentService, clk clock.Clock) (func(), func(context.Context)) {
	s.clock = clk
	if s.parScopePoller == nil {
		return func() { s.orgStatusPoller.poll(s.api, s.rcType) }, func(context.Context) {}
	}
	s.parScopePoller.clock = clk
	s.parScopePoller.jitter = func(time.Duration) time.Duration { return 0 }
	s.parScopePoller.next = clk.Now()
	return func() { s.orgStatusPoller.poll(s.api, s.rcType) }, s.parScopePoller.step
}
