// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
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

func pingCapableChecker() *fakeChecker {
	return &fakeChecker{respond: func(_ connectivity.Request) (connectivity.Result, error) {
		return connectivity.Result{Devices: []connectivity.DeviceResult{{
			IPAddress:  pingProbeTarget,
			PingResult: &connectivity.PingResult{CheckResult: connectivity.CheckResult{Success: true}},
		}}}, nil
	}}
}

func newTestPingProbe(t *testing.T, checker connectivityChecker) *pingProbe {
	t.Helper()
	p := newPingProbe(checker, logmock.New(t))
	p.detect(context.Background())
	return p
}

func pingRunFor(t *testing.T, raw string) *pingRun {
	t.Helper()
	cfg, err := newTestPingProbe(t, pingCapableChecker()).parse(json.RawMessage(raw))
	require.NoError(t, err)
	run, err := cfg.prepare()
	require.NoError(t, err)
	ping, ok := run.(*pingRun)
	require.True(t, ok)
	return ping
}

func TestPingProbeDetectSetsAvailability(t *testing.T) {
	capable := newPingProbe(pingCapableChecker(), logmock.New(t))
	assert.False(t, capable.available(), "availability is unknown before detect")
	capable.detect(context.Background())
	assert.True(t, capable.available())
	assert.Equal(t, "ping", capable.kind())

	incapable := newPingProbe(&fakeChecker{}, logmock.New(t))
	incapable.detect(context.Background())
	assert.False(t, incapable.available())
}

func TestPingProbeParseFull(t *testing.T) {
	run := pingRunFor(t, `{"count":2,"interval_ms":500,"timeout_ms":1500}`)

	assert.Equal(t, 2, run.options.Count)
	assert.Equal(t, 500, run.options.IntervalMs)
	assert.Equal(t, 1500, run.options.TimeoutMs)
}

func TestPingProbeParseDefaults(t *testing.T) {
	run := pingRunFor(t, `{}`)

	assert.Equal(t, defaultPingCount, run.options.Count)
	assert.Equal(t, defaultPingIntervalMs, run.options.IntervalMs)
	assert.Equal(t, defaultPingTimeoutMs, run.options.TimeoutMs)
}

func TestPingProbeParseFallsBackOnAnOutOfBoundsKnob(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"count too large", `{"count":100000}`},
		{"count negative", `{"count":-1}`},
		{"interval too large", `{"interval_ms":100000000}`},
		{"interval negative", `{"interval_ms":-1}`},
		{"timeout too large", `{"timeout_ms":100000000}`},
		{"timeout negative", `{"timeout_ms":-1}`},
	}

	want := connectivity.PingOptions{Count: defaultPingCount, IntervalMs: defaultPingIntervalMs, TimeoutMs: defaultPingTimeoutMs}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, want, pingRunFor(t, tt.raw).options)
		})
	}
}

func TestPingProbeParseRejectsAMalformedBlock(t *testing.T) {
	_, err := newTestPingProbe(t, pingCapableChecker()).parse(json.RawMessage(`"on"`))
	require.Error(t, err)
}

func TestPingRunApplyAppendsItsCheckAndOptions(t *testing.T) {
	run := pingRunFor(t, `{"count":2}`)

	req := connectivity.Request{}
	run.apply(&req)

	assert.Equal(t, []string{connectivity.CheckPing}, req.Checks)
	require.NotNil(t, req.PingOptions)
	assert.Equal(t, 2, req.PingOptions.Count)
}

func TestPingRunFingerprintCoversTheOptions(t *testing.T) {
	base := pingRunFor(t, `{"count":2}`).fingerprint()

	assert.Equal(t, base, pingRunFor(t, `{"count":2}`).fingerprint())
	assert.NotEqual(t, base, pingRunFor(t, `{"count":3}`).fingerprint())
	assert.True(t, strings.HasPrefix(base, "ping:"), "a fingerprint names its own kind")
}

func TestPingRunRead(t *testing.T) {
	rtt := int64(3)
	run := pingRunFor(t, `{}`)

	answered := run.read(connectivity.DeviceResult{
		IPAddress:  "10.0.0.1",
		PingResult: &connectivity.PingResult{CheckResult: connectivity.CheckResult{Success: true, RttMs: &rtt}, FailureReason: connectivity.FailureNone},
	})
	require.NotNil(t, answered)
	assert.Equal(t, metadata.ProbeResult{Kind: "ping", Status: statusReachable, RttMs: &rtt}, answered.Result)
	assert.Empty(t, answered.Name, "ping learns no device name")

	silent := run.read(connectivity.DeviceResult{
		IPAddress:  "10.0.0.2",
		PingResult: &connectivity.PingResult{CheckResult: connectivity.CheckResult{Success: false}, FailureReason: connectivity.FailureUnreachable},
	})
	require.NotNil(t, silent)
	assert.Equal(t, metadata.ProbeResult{Kind: "ping", Status: statusUnreachable, FailureReason: connectivity.FailureUnreachable}, silent.Result)

	assert.Nil(t, run.read(connectivity.DeviceResult{IPAddress: "10.0.0.3"}), "no ping answer is no ping reading")
}
