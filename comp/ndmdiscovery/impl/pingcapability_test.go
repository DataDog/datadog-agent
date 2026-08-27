// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// fakeChecker is a scripted connectivityChecker shared by the tests in this
// package. Requests are recorded so a test can assert on what was dispatched.
type fakeChecker struct {
	mu       sync.Mutex
	requests []connectivity.Request
	respond  func(req connectivity.Request) (connectivity.Result, error)
}

func (f *fakeChecker) CheckConnectivity(_ context.Context, req connectivity.Request) (connectivity.Result, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()
	if f.respond != nil {
		return f.respond(req)
	}
	return connectivity.Result{}, nil
}

func (f *fakeChecker) recorded() []connectivity.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]connectivity.Request(nil), f.requests...)
}

func TestProbePingSucceeds(t *testing.T) {
	checker := &fakeChecker{respond: func(_ connectivity.Request) (connectivity.Result, error) {
		return connectivity.Result{Devices: []connectivity.DeviceResult{{
			IPAddress:  pingProbeTarget,
			PingResult: &connectivity.PingResult{CheckResult: connectivity.CheckResult{Success: true}},
		}}}, nil
	}}

	assert.True(t, probePing(context.Background(), checker))

	reqs := checker.recorded()
	require.Len(t, reqs, 1)
	assert.Equal(t, []string{pingProbeTarget}, reqs[0].Targets)
	assert.Equal(t, []string{connectivity.CheckPing}, reqs[0].Checks)
	require.NotNil(t, reqs[0].PingOptions)
	assert.Equal(t, 1, reqs[0].PingOptions.Count)
}

func TestProbePingFailsOnError(t *testing.T) {
	checker := &fakeChecker{respond: func(_ connectivity.Request) (connectivity.Result, error) {
		return connectivity.Result{}, errors.New("operation not permitted")
	}}
	assert.False(t, probePing(context.Background(), checker))
}

func TestProbePingFailsOnUnreachableLoopback(t *testing.T) {
	checker := &fakeChecker{respond: func(_ connectivity.Request) (connectivity.Result, error) {
		return connectivity.Result{Devices: []connectivity.DeviceResult{{
			IPAddress: pingProbeTarget,
			PingResult: &connectivity.PingResult{
				CheckResult:   connectivity.CheckResult{Success: false},
				FailureReason: connectivity.FailureUnreachable,
			},
		}}}, nil
	}}
	assert.False(t, probePing(context.Background(), checker),
		"the loopback address is always reachable, so a failure means ping is unavailable")
}

func TestProbePingFailsOnEmptyResult(t *testing.T) {
	checker := &fakeChecker{}
	assert.False(t, probePing(context.Background(), checker))
}
