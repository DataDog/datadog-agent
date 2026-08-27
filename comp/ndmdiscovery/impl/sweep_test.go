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
	mu      sync.Mutex
	devices []metadata.DiscoveredDeviceMetadata
	batches int
	runs    []metadata.AutodiscoveryRunMetadata
	// deviceErr is returned by the next deviceErrLeft calls to ReportDevices.
	deviceErr     error
	deviceErrLeft int
}

// failNextDeviceReports makes the next n ReportDevices calls fail with
// deviceErr, which must be set.
func (r *recordingReporter) failNextDeviceReports(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deviceErrLeft = n
}

func (r *recordingReporter) ReportDevices(_ string, d []metadata.DiscoveredDeviceMetadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.deviceErr != nil && r.deviceErrLeft > 0 {
		r.deviceErrLeft--
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
	s := newSweeper(checker, reporter, cursors, semaphore.NewWeighted(workers), workers, logmock.New(t))
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
	require.True(t, ok, "the cursor is kept, even though it holds no completed chunk yet")
	assert.Equal(t, 0, saved.NextChunk, "the interrupted chunk is not counted as done")
	assert.False(t, saved.Failed, "a stopping agent does not end the run")
	// NextChunk is still 0, so startState discards this cursor: an interruption
	// during the very first chunk restarts as a fresh cycle rather than
	// resuming, which costs at most one chunk of re-scanning.
}

func TestSweepCancellationMidRangeResumesWithTheSameRunID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	checker := &fakeChecker{respond: func(req connectivity.Request) (connectivity.Result, error) {
		calls++
		if calls == 3 {
			// Two chunks are already done, so the cursor holds real progress.
			cancel()
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

	req := testSweepRequest(t, "10.0.0.0/22", nil)
	require.ErrorIs(t, s.sweep(ctx, req), context.Canceled)

	saved, ok := cursors.Load("ad-1")
	require.True(t, ok)
	assert.Equal(t, 2, saved.NextChunk, "the two completed chunks are not re-scanned")
	assert.Equal(t, "run-fixed", saved.RunID)
	assert.False(t, saved.Failed)

	// The next agent start picks the cycle back up where it stopped.
	resumeChecker := answerAll()
	s2 := newTestSweeper(t, resumeChecker, reporter, cursors, 10)
	s2.newRunID = func() string { return "run-should-not-be-used" }
	require.NoError(t, s2.sweep(context.Background(), req))

	assert.Len(t, resumeChecker.recorded(), 2, "only the remaining chunks are swept")
	final := reporter.runs[len(reporter.runs)-1]
	assert.Equal(t, metadata.AutodiscoveryRunCompleted, final.Status)
	assert.Equal(t, "run-fixed", final.RunID, "the resumed cycle keeps its original run ID")
	assert.Equal(t, int64(1024), final.AddressesScanned)
	for _, run := range reporter.runs {
		assert.NotEqual(t, "run-should-not-be-used", run.RunID)
	}
	assert.Equal(t, 1, countRunStatus(reporter.runs, metadata.AutodiscoveryRunInProgress),
		"a plain restart mid-run does not duplicate the in_progress record")
}

func TestSweepResumeAfterFailureOpensANewRun(t *testing.T) {
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

	req := testSweepRequest(t, "10.0.0.0/22", nil)
	require.Error(t, s.sweep(context.Background(), req))

	saved, ok := cursors.Load("ad-1")
	require.True(t, ok)
	assert.True(t, saved.Failed, "the terminal failed record is remembered on the cursor")

	// The next tick resumes the remaining chunks.
	s2 := newTestSweeper(t, answerAll(), reporter, cursors, 10)
	s2.newRunID = func() string { return "run-second" }
	require.NoError(t, s2.sweep(context.Background(), req))

	byRun := map[string][]metadata.AutodiscoveryRunStatus{}
	for _, run := range reporter.runs {
		byRun[run.RunID] = append(byRun[run.RunID], run.Status)
	}
	assert.Equal(t, []metadata.AutodiscoveryRunStatus{metadata.AutodiscoveryRunInProgress, metadata.AutodiscoveryRunFailed}, byRun["run-fixed"],
		"the failed run gets exactly one terminal record")
	assert.Equal(t, []metadata.AutodiscoveryRunStatus{metadata.AutodiscoveryRunInProgress, metadata.AutodiscoveryRunCompleted}, byRun["run-second"],
		"the remaining work runs under a new run ID with its own lifecycle")

	final := reporter.runs[len(reporter.runs)-1]
	assert.Equal(t, int64(1024), final.AddressesScanned, "progress made before the failure is preserved")

	saved, ok = cursors.Load("ad-1")
	assert.False(t, ok, "the completed cycle clears its cursor")
	assert.False(t, saved.Failed)
}

func TestSweepClampsWorkersToTheBudget(t *testing.T) {
	checker := answerAll()
	// The global budget is 4, so a range asking for 32 must not deadlock on
	// semaphore.Acquire, which never returns for n greater than the size.
	s := newTestSweeper(t, checker, &recordingReporter{}, newMemCursorStore(), 4)

	req := testSweepRequest(t, "10.0.0.0/24", nil)
	req.Workers = 32
	require.NoError(t, s.sweep(context.Background(), req))

	sent := checker.recorded()
	require.Len(t, sent, 1)
	assert.Equal(t, 4, sent[0].Workers)
}

func TestSweepClampsNonPositiveWorkersToOne(t *testing.T) {
	checker := answerAll()
	s := newTestSweeper(t, checker, &recordingReporter{}, newMemCursorStore(), 4)

	req := testSweepRequest(t, "10.0.0.0/24", nil)
	req.Workers = 0
	require.NoError(t, s.sweep(context.Background(), req))

	sent := checker.recorded()
	require.Len(t, sent, 1)
	assert.Equal(t, 1, sent[0].Workers, "a zero share would bound nothing")
}

func TestSweepContinuesWhenAChunkReportFails(t *testing.T) {
	reporter := &recordingReporter{deviceErr: errors.New("intake unavailable")}
	cursors := newMemCursorStore()
	s := newTestSweeper(t, answerAll(), reporter, cursors, 10)

	req := testSweepRequest(t, "10.0.0.0/22", nil)
	// Only the first chunk fails to report: a transport failure must not abort
	// a multi-hour cycle.
	reporter.failNextDeviceReports(1)
	require.NoError(t, s.sweep(context.Background(), req))

	final := reporter.runs[len(reporter.runs)-1]
	assert.Equal(t, metadata.AutodiscoveryRunCompleted, final.Status)
	assert.Equal(t, int64(1024), final.AddressesScanned, "the unreported chunk still counts as swept")
	assert.Equal(t, 3, reporter.batches, "the three chunks after it are reported normally")
	_, ok := cursors.Load("ad-1")
	assert.False(t, ok, "the cursor advanced past the failed report and the cycle cleared it")
}

func countRunStatus(runs []metadata.AutodiscoveryRunMetadata, status metadata.AutodiscoveryRunStatus) int {
	n := 0
	for _, run := range runs {
		if run.Status == status {
			n++
		}
	}
	return n
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
	res := connectivity.Result{Devices: []connectivity.DeviceResult{
		{
			IPAddress:  "10.0.0.1",
			PingResult: &connectivity.PingResult{CheckResult: connectivity.CheckResult{Success: true, RttMs: nil}},
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
