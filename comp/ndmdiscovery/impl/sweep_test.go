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
	"golang.org/x/sync/semaphore"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// recordingReporter captures everything the sweeper publishes.
type recordingReporter struct {
	mu        sync.Mutex
	devices   []metadata.DiscoveredDeviceMetadata
	batches   int
	runs      []metadata.AutodiscoveryRunMetadata
	deviceErr error
}

func (r *recordingReporter) ReportDevices(_ string, d []metadata.DiscoveredDeviceMetadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.deviceErr != nil {
		return r.deviceErr
	}
	r.batches++
	r.devices = append(r.devices, d...)
	return nil
}

func (r *recordingReporter) ReportRun(_ string, run metadata.AutodiscoveryRunMetadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs = append(r.runs, run)
	return nil
}

func newTestSweeper(t *testing.T, checker connectivityChecker, reporter discoveryReporter, cursors cursorStore, workers int64) *sweeper {
	t.Helper()
	s := newSweeper(checker, reporter, cursors, semaphore.NewWeighted(workers), logmock.New(t))
	s.now = func() int64 { return 1700000000000 }
	s.newRunID = func() string { return "run-fixed" }
	return s
}

func testSweepRequest(t *testing.T, cidr string, ignored []string) sweepRequest {
	t.Helper()
	cfg := rangeConfig{
		AutodiscoveryID:    "ad-1",
		Namespace:          "default",
		CIDR:               cidr,
		IgnoredIPAddresses: ignored,
		SNMPOptions:        &connectivity.SNMPOptions{Port: 161, TimeoutMs: 2000, Retries: 1},
	}
	plan, err := newChunkPlan(cidr, ignored, 65536)
	require.NoError(t, err)
	creds := []connectivity.SNMPCredential{{ID: "cred-a", Version: "2c", Community: "public"}}
	return sweepRequest{
		Config:      cfg,
		Credentials: creds,
		Plan:        plan,
		Digest:      rangeDigest(cfg, creds),
		Workers:     2,
	}
}

// answerAll returns a checker that marks the first target of each chunk as an
// SNMP success and the rest as failures.
func answerAll() *fakeChecker {
	return &fakeChecker{respond: func(req connectivity.Request) (connectivity.Result, error) {
		devices := make([]connectivity.DeviceResult, 0, len(req.Targets))
		for i, ip := range req.Targets {
			d := connectivity.DeviceResult{IPAddress: ip}
			if i == 0 {
				d.SNMPResult = &connectivity.SNMPResult{
					CheckResult:   connectivity.CheckResult{Success: true},
					FailureReason: connectivity.FailureNone,
					CredID:        "cred-a",
					SysName:       "router-" + ip,
				}
			} else {
				d.SNMPResult = &connectivity.SNMPResult{
					CheckResult:   connectivity.CheckResult{Success: false},
					FailureReason: connectivity.FailureTimeout,
				}
			}
			devices = append(devices, d)
		}
		return connectivity.Result{Devices: devices}, nil
	}}
}

func TestSweepCompletesAndReportsRunLifecycle(t *testing.T) {
	checker := answerAll()
	reporter := &recordingReporter{}
	cursors := newMemCursorStore()
	s := newTestSweeper(t, checker, reporter, cursors, 10)

	require.NoError(t, s.sweep(context.Background(), testSweepRequest(t, "10.0.0.0/24", nil)))

	require.Len(t, reporter.runs, 2)
	assert.Equal(t, metadata.AutodiscoveryRunInProgress, reporter.runs[0].Status)
	assert.Equal(t, "run-fixed", reporter.runs[0].RunID)
	assert.Equal(t, "ad-1", reporter.runs[0].AutodiscoveryID)

	final := reporter.runs[1]
	assert.Equal(t, metadata.AutodiscoveryRunCompleted, final.Status)
	assert.Equal(t, int64(256), final.AddressesScanned)
	assert.Equal(t, int64(1700000000000), final.FinishedAtMs)
	assert.Empty(t, final.Error)

	assert.Len(t, reporter.devices, 256)
	_, ok := cursors.Load("ad-1")
	assert.False(t, ok, "a completed cycle clears its cursor")
}

func TestSweepReportsPerChunkNotAtTheEnd(t *testing.T) {
	checker := answerAll()
	reporter := &recordingReporter{}
	s := newTestSweeper(t, checker, reporter, newMemCursorStore(), 10)

	require.NoError(t, s.sweep(context.Background(), testSweepRequest(t, "10.0.0.0/22", nil)))

	assert.Equal(t, 4, reporter.batches, "one report per chunk, so memory stays bounded")
	assert.Len(t, reporter.devices, 1024)
	assert.Len(t, checker.recorded(), 4)
}

func TestSweepCountsIgnoredAddressesTowardsProgress(t *testing.T) {
	reporter := &recordingReporter{}
	s := newTestSweeper(t, answerAll(), reporter, newMemCursorStore(), 10)

	require.NoError(t, s.sweep(context.Background(), testSweepRequest(t, "10.0.0.0/24", []string{"10.0.0.1", "10.0.0.2"})))

	final := reporter.runs[len(reporter.runs)-1]
	assert.Equal(t, int64(256), final.AddressesScanned, "ignored addresses still count, so progress reaches 100%")
	assert.Len(t, reporter.devices, 254, "but they are not reported as probed devices")
}

func TestSweepResumesFromCursor(t *testing.T) {
	checker := answerAll()
	reporter := &recordingReporter{}
	cursors := newMemCursorStore()
	s := newTestSweeper(t, checker, reporter, cursors, 10)

	req := testSweepRequest(t, "10.0.0.0/22", nil)
	require.NoError(t, cursors.Save("ad-1", cursorState{
		RunID:        "run-earlier",
		NextChunk:    2,
		Scanned:      512,
		StartedAtMs:  1699000000000,
		ConfigDigest: req.Digest,
	}))

	require.NoError(t, s.sweep(context.Background(), req))

	assert.Len(t, checker.recorded(), 2, "the first two chunks were already done")
	final := reporter.runs[len(reporter.runs)-1]
	assert.Equal(t, "run-earlier", final.RunID, "the cycle keeps its original run ID")
	assert.Equal(t, int64(1699000000000), final.StartedAtMs)
	assert.Equal(t, int64(1024), final.AddressesScanned)
}

func TestSweepDiscardsCursorOnDigestChange(t *testing.T) {
	checker := answerAll()
	reporter := &recordingReporter{}
	cursors := newMemCursorStore()
	s := newTestSweeper(t, checker, reporter, cursors, 10)

	req := testSweepRequest(t, "10.0.0.0/22", nil)
	require.NoError(t, cursors.Save("ad-1", cursorState{
		RunID:        "run-earlier",
		NextChunk:    2,
		Scanned:      512,
		ConfigDigest: "a-different-digest",
	}))

	require.NoError(t, s.sweep(context.Background(), req))

	assert.Len(t, checker.recorded(), 4, "the range changed, so the partial results are void")
	assert.Equal(t, "run-fixed", reporter.runs[0].RunID)
}

func TestSweepFailedChunkKeepsCursor(t *testing.T) {
	calls := 0
	checker := &fakeChecker{respond: func(req connectivity.Request) (connectivity.Result, error) {
		calls++
		if calls == 3 {
			return connectivity.Result{}, errors.New("engine exploded")
		}
		devices := make([]connectivity.DeviceResult, 0, len(req.Targets))
		for _, ip := range req.Targets {
			devices = append(devices, connectivity.DeviceResult{IPAddress: ip})
		}
		return connectivity.Result{Devices: devices}, nil
	}}
	reporter := &recordingReporter{}
	cursors := newMemCursorStore()
	s := newTestSweeper(t, checker, reporter, cursors, 10)

	err := s.sweep(context.Background(), testSweepRequest(t, "10.0.0.0/22", nil))
	require.Error(t, err)

	final := reporter.runs[len(reporter.runs)-1]
	assert.Equal(t, metadata.AutodiscoveryRunFailed, final.Status)
	assert.Contains(t, final.Error, "engine exploded")
	assert.Equal(t, int64(512), final.AddressesScanned)

	saved, ok := cursors.Load("ad-1")
	require.True(t, ok, "a failed cycle keeps its cursor so the next tick resumes")
	assert.Equal(t, 2, saved.NextChunk)
	assert.Equal(t, "run-fixed", saved.RunID)
}

func TestSweepCancellationDoesNotReportFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	checker := &fakeChecker{respond: func(req connectivity.Request) (connectivity.Result, error) {
		cancel()
		devices := make([]connectivity.DeviceResult, 0, len(req.Targets))
		for _, ip := range req.Targets {
			devices = append(devices, connectivity.DeviceResult{IPAddress: ip})
		}
		return connectivity.Result{Devices: devices}, nil
	}}
	reporter := &recordingReporter{}
	cursors := newMemCursorStore()
	s := newTestSweeper(t, checker, reporter, cursors, 10)

	err := s.sweep(ctx, testSweepRequest(t, "10.0.0.0/22", nil))
	require.ErrorIs(t, err, context.Canceled)

	for _, run := range reporter.runs {
		assert.NotEqual(t, metadata.AutodiscoveryRunFailed, run.Status,
			"a stopping agent is not a broken run")
	}
	saved, ok := cursors.Load("ad-1")
	require.True(t, ok, "the cursor is kept so the next agent start resumes here")
	assert.Equal(t, 0, saved.NextChunk, "the interrupted chunk is not counted as done")
}

func TestSweepBuildsTheRequest(t *testing.T) {
	checker := answerAll()
	s := newTestSweeper(t, checker, &recordingReporter{}, newMemCursorStore(), 10)

	req := testSweepRequest(t, "10.0.0.0/24", nil)
	req.Workers = 4
	req.PingEnabled = true
	req.Config.PingOptions = &connectivity.PingOptions{Count: 1, IntervalMs: 1000, TimeoutMs: 1000}
	require.NoError(t, s.sweep(context.Background(), req))

	sent := checker.recorded()
	require.Len(t, sent, 1)
	assert.Equal(t, []string{connectivity.CheckPing, connectivity.CheckSNMP}, sent[0].Checks)
	assert.Equal(t, 4, sent[0].Workers)
	assert.Equal(t, req.Credentials, sent[0].Credentials)
	assert.Equal(t, req.Config.SNMPOptions, sent[0].SNMPOptions)
	assert.Equal(t, req.Config.PingOptions, sent[0].PingOptions)
	assert.Len(t, sent[0].Targets, 256)
}

func TestSweepOmitsPingWhenDisabled(t *testing.T) {
	checker := answerAll()
	s := newTestSweeper(t, checker, &recordingReporter{}, newMemCursorStore(), 10)

	req := testSweepRequest(t, "10.0.0.0/24", nil)
	req.PingEnabled = false
	req.Config.PingOptions = &connectivity.PingOptions{Count: 1}
	require.NoError(t, s.sweep(context.Background(), req))

	sent := checker.recorded()
	require.Len(t, sent, 1)
	assert.Equal(t, []string{connectivity.CheckSNMP}, sent[0].Checks)
	assert.Nil(t, sent[0].PingOptions)
}

func TestToDiscoveredDevices(t *testing.T) {
	rtt := int64(4)
	res := connectivity.Result{Devices: []connectivity.DeviceResult{
		{
			IPAddress:  "10.0.0.1",
			PingResult: &connectivity.PingResult{CheckResult: connectivity.CheckResult{Success: true, RttMs: &rtt}},
			SNMPResult: &connectivity.SNMPResult{
				CheckResult: connectivity.CheckResult{Success: true},
				CredID:      "cred-a",
				SysName:     "router-1",
			},
		},
		{
			IPAddress:  "10.0.0.2",
			PingResult: &connectivity.PingResult{CheckResult: connectivity.CheckResult{Success: false}},
			SNMPResult: &connectivity.SNMPResult{CheckResult: connectivity.CheckResult{Success: false}},
		},
		{IPAddress: "10.0.0.3"},
		// A zero-value entry: the engine pre-allocates its slice, so an
		// interrupted run can leave holes. They must be dropped.
		{},
	}}

	got := toDiscoveredDevices("ad-1", "run-1", res)
	require.Len(t, got, 3)

	assert.Equal(t, metadata.DiscoveredDeviceMetadata{
		AutodiscoveryID: "ad-1", RunID: "run-1", IPAddress: "10.0.0.1",
		Name: "router-1", PingStatus: "reachable", SNMPStatus: "reachable", SNMPCredID: "cred-a",
	}, got[0])

	assert.Equal(t, metadata.DiscoveredDeviceMetadata{
		AutodiscoveryID: "ad-1", RunID: "run-1", IPAddress: "10.0.0.2",
		PingStatus: "unreachable", SNMPStatus: "unreachable",
	}, got[1])

	assert.Equal(t, metadata.DiscoveredDeviceMetadata{
		AutodiscoveryID: "ad-1", RunID: "run-1", IPAddress: "10.0.0.3",
	}, got[2], "an unprobed check leaves its status empty rather than claiming unreachable")
}
