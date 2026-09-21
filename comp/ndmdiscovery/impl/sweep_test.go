// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/semaphore"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/pingprobe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/snmpprobe"
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

// scanCall is one batch the sweeper handed to the probe package.
type scanCall struct {
	Workers int
	Targets []string
	Options probe.Options
}

// recordingScanner is a scanFunc that records its calls.
type recordingScanner struct {
	mu      sync.Mutex
	calls   []scanCall
	respond func(call scanCall) ([]probe.Result, error)
}

func (r *recordingScanner) scan(_ context.Context, workers int, targets []string, opts probe.Options) ([]probe.Result, error) {
	call := scanCall{Workers: workers, Targets: targets, Options: opts}
	r.mu.Lock()
	r.calls = append(r.calls, call)
	r.mu.Unlock()
	return r.respond(call)
}

func (r *recordingScanner) recorded() []scanCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]scanCall(nil), r.calls...)
}

// answerAll marks the first target of each batch as an SNMP success and the
// rest as failures.
func answerAll() *recordingScanner {
	return &recordingScanner{respond: func(call scanCall) ([]probe.Result, error) {
		results := make([]probe.Result, 0, len(call.Targets))
		for i, ip := range call.Targets {
			r := probe.Result{Target: ip}
			if i == 0 {
				r.SNMP = &snmpprobe.Reading{Success: true, CredID: "cred-a", SysName: "router-" + ip}
			} else {
				r.SNMP = &snmpprobe.Reading{FailureReason: "timeout"}
			}
			results = append(results, r)
		}
		return results, nil
	}}
}

// silentAll reports every target as probed but unreachable.
func silentAll() *recordingScanner {
	return &recordingScanner{respond: func(call scanCall) ([]probe.Result, error) {
		results := make([]probe.Result, 0, len(call.Targets))
		for _, ip := range call.Targets {
			results = append(results, probe.Result{Target: ip})
		}
		return results, nil
	}}
}

func newTestSweeper(t *testing.T, scanner *recordingScanner, reporter discoveryReporter, cursors cursorStore, workers int64) *sweeper {
	t.Helper()
	s := newSweeper(scanner.scan, reporter, cursors, semaphore.NewWeighted(workers), workers, logmock.New(t))
	s.now = func() int64 { return 1700000000000 }
	s.newRunID = func() string { return "run-fixed" }
	return s
}

func testSNMPOptions() *snmpprobe.Options {
	return &snmpprobe.Options{
		Port:        161,
		Timeout:     2 * time.Second,
		Retries:     1,
		Credentials: []snmpprobe.Credential{{ID: "cred-a", Version: "2c", Community: "public"}},
	}
}

func testSweepRequest(t *testing.T, cidr string, ignored []string) sweepRequest {
	t.Helper()
	return testSweepRequestWithOptions(t, cidr, ignored, probe.Options{SNMP: testSNMPOptions()})
}

func testSweepRequestWithOptions(t *testing.T, cidr string, ignored []string, opts probe.Options) sweepRequest {
	t.Helper()
	cfg := rangeConfig{
		AutodiscoveryID:    "ad-1",
		Namespace:          "default",
		CIDR:               cidr,
		IgnoredIPAddresses: ignored,
	}
	plan, err := newChunkPlan(cidr, ignored, 65536)
	require.NoError(t, err)

	return sweepRequest{
		Config:  cfg,
		Options: opts,
		Plan:    plan,
		Digest:  rangeDigest(cfg, opts.Fingerprints()),
		Workers: 2,
	}
}

func TestSweepCompletesAndReportsRunLifecycle(t *testing.T) {
	scanner := answerAll()
	reporter := &recordingReporter{}
	cursors := newMemCursorStore()
	s := newTestSweeper(t, scanner, reporter, cursors, 10)

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

	// answerAll answers on the first target of each chunk only, so one /24 is
	// 256 addresses scanned and one device reported.
	assert.Len(t, reporter.devices, 1, "a silent address is absent from the run, not reported unreachable")
	_, ok := cursors.Load("ad-1")
	assert.False(t, ok, "a completed cycle clears its cursor")
}

func TestSweepReportsPerChunkNotAtTheEnd(t *testing.T) {
	scanner := answerAll()
	reporter := &recordingReporter{}
	s := newTestSweeper(t, scanner, reporter, newMemCursorStore(), 10)

	require.NoError(t, s.sweep(context.Background(), testSweepRequest(t, "10.0.0.0/22", nil)))

	assert.Equal(t, 4, reporter.batches, "one report per chunk, so memory stays bounded")
	assert.Len(t, reporter.devices, 4, "one answering address per chunk")
	assert.Len(t, scanner.recorded(), 4)
}

func TestSweepCountsIgnoredAddressesTowardsProgress(t *testing.T) {
	reporter := &recordingReporter{}
	s := newTestSweeper(t, answerAll(), reporter, newMemCursorStore(), 10)

	require.NoError(t, s.sweep(context.Background(), testSweepRequest(t, "10.0.0.0/24", []string{"10.0.0.1", "10.0.0.2"})))

	final := reporter.runs[len(reporter.runs)-1]
	assert.Equal(t, int64(256), final.AddressesScanned, "ignored addresses still count, so progress reaches 100%")
	assert.Len(t, reporter.devices, 1, "scanned counts every address; reported counts only the answers")
}

func TestSweepFullyIgnoredChunkHasNoTargets(t *testing.T) {
	ignored := make([]string, 0, 256)
	for i := 0; i < 256; i++ {
		ignored = append(ignored, "10.0.0."+strconv.Itoa(i))
	}

	scanner := answerAll()
	reporter := &recordingReporter{}
	s := newTestSweeper(t, scanner, reporter, newMemCursorStore(), 10)

	require.NoError(t, s.sweep(context.Background(), testSweepRequest(t, "10.0.0.0/24", ignored)))

	final := reporter.runs[len(reporter.runs)-1]
	assert.Equal(t, metadata.AutodiscoveryRunCompleted, final.Status)
	assert.Equal(t, int64(256), final.AddressesScanned)
	assert.Equal(t, 0, reporter.batches, "a chunk with no targets is not probed and reports nothing")
	assert.Empty(t, scanner.recorded(), "the probe package is never called for a fully ignored chunk")
}

func TestSweepResumesFromCursor(t *testing.T) {
	scanner := answerAll()
	reporter := &recordingReporter{}
	cursors := newMemCursorStore()
	s := newTestSweeper(t, scanner, reporter, cursors, 10)

	req := testSweepRequest(t, "10.0.0.0/22", nil)
	require.NoError(t, cursors.Save("ad-1", cursorState{
		RunID:        "run-earlier",
		NextChunk:    2,
		Scanned:      512,
		StartedAtMs:  1699000000000,
		ConfigDigest: req.Digest,
	}))

	require.NoError(t, s.sweep(context.Background(), req))

	assert.Len(t, scanner.recorded(), 2, "the first two chunks were already done")
	final := reporter.runs[len(reporter.runs)-1]
	assert.Equal(t, "run-earlier", final.RunID, "the cycle keeps its original run ID")
	assert.Equal(t, int64(1699000000000), final.StartedAtMs)
	assert.Equal(t, int64(1024), final.AddressesScanned)
}

func TestSweepDiscardsCursorOnDigestChange(t *testing.T) {
	scanner := answerAll()
	reporter := &recordingReporter{}
	cursors := newMemCursorStore()
	s := newTestSweeper(t, scanner, reporter, cursors, 10)

	req := testSweepRequest(t, "10.0.0.0/22", nil)
	require.NoError(t, cursors.Save("ad-1", cursorState{
		RunID:        "run-earlier",
		NextChunk:    2,
		Scanned:      512,
		ConfigDigest: "a-different-digest",
	}))

	require.NoError(t, s.sweep(context.Background(), req))

	assert.Len(t, scanner.recorded(), 4, "the range changed, so the partial results are void")
	assert.Equal(t, "run-fixed", reporter.runs[0].RunID)
}

func TestSweepFailedChunkKeepsCursor(t *testing.T) {
	calls := 0
	scanner := &recordingScanner{respond: func(call scanCall) ([]probe.Result, error) {
		calls++
		if calls == 3 {
			return nil, errors.New("the scan exploded")
		}
		results := make([]probe.Result, 0, len(call.Targets))
		for _, ip := range call.Targets {
			results = append(results, probe.Result{Target: ip})
		}
		return results, nil
	}}
	reporter := &recordingReporter{}
	cursors := newMemCursorStore()
	s := newTestSweeper(t, scanner, reporter, cursors, 10)

	err := s.sweep(context.Background(), testSweepRequest(t, "10.0.0.0/22", nil))
	require.Error(t, err)

	final := reporter.runs[len(reporter.runs)-1]
	assert.Equal(t, metadata.AutodiscoveryRunFailed, final.Status)
	assert.Contains(t, final.Error, "the scan exploded")
	assert.Equal(t, int64(512), final.AddressesScanned)

	saved, ok := cursors.Load("ad-1")
	require.True(t, ok, "a failed cycle keeps its cursor so the next tick resumes")
	assert.Equal(t, 2, saved.NextChunk)
	assert.Equal(t, "run-fixed", saved.RunID)
}

func TestSweepCancellationDoesNotReportFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	scanner := &recordingScanner{respond: func(_ scanCall) ([]probe.Result, error) {
		cancel()
		return nil, context.Canceled
	}}
	reporter := &recordingReporter{}
	cursors := newMemCursorStore()
	s := newTestSweeper(t, scanner, reporter, cursors, 10)

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
}

func TestSweepCancellationMidRangeResumesWithTheSameRunID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	scanner := &recordingScanner{respond: func(call scanCall) ([]probe.Result, error) {
		calls++
		if calls == 3 {
			// Two chunks are already done, so the cursor holds real progress.
			cancel()
			return nil, context.Canceled
		}
		results := make([]probe.Result, 0, len(call.Targets))
		for _, ip := range call.Targets {
			results = append(results, probe.Result{Target: ip})
		}
		return results, nil
	}}
	reporter := &recordingReporter{}
	cursors := newMemCursorStore()
	s := newTestSweeper(t, scanner, reporter, cursors, 10)

	req := testSweepRequest(t, "10.0.0.0/22", nil)
	require.ErrorIs(t, s.sweep(ctx, req), context.Canceled)

	saved, ok := cursors.Load("ad-1")
	require.True(t, ok)
	assert.Equal(t, 2, saved.NextChunk, "the two completed chunks are not re-scanned")
	assert.Equal(t, "run-fixed", saved.RunID)
	assert.False(t, saved.Failed)

	// The next agent start picks the cycle back up where it stopped.
	resumeScanner := answerAll()
	s2 := newTestSweeper(t, resumeScanner, reporter, cursors, 10)
	s2.newRunID = func() string { return "run-should-not-be-used" }
	require.NoError(t, s2.sweep(context.Background(), req))

	assert.Len(t, resumeScanner.recorded(), 2, "only the remaining chunks are swept")
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
	scanner := &recordingScanner{respond: func(call scanCall) ([]probe.Result, error) {
		calls++
		if calls == 3 {
			return nil, errors.New("the scan exploded")
		}
		results := make([]probe.Result, 0, len(call.Targets))
		for _, ip := range call.Targets {
			results = append(results, probe.Result{Target: ip})
		}
		return results, nil
	}}
	reporter := &recordingReporter{}
	cursors := newMemCursorStore()
	s := newTestSweeper(t, scanner, reporter, cursors, 10)

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
	scanner := answerAll()
	// The global budget is 4, so a range asking for 32 must not deadlock on
	// semaphore.Acquire, which never returns for n greater than the size.
	s := newTestSweeper(t, scanner, &recordingReporter{}, newMemCursorStore(), 4)

	req := testSweepRequest(t, "10.0.0.0/24", nil)
	req.Workers = 32
	require.NoError(t, s.sweep(context.Background(), req))

	sent := scanner.recorded()
	require.Len(t, sent, 1)
	assert.Equal(t, 4, sent[0].Workers)
}

func TestSweepClampsNonPositiveWorkersToOne(t *testing.T) {
	scanner := answerAll()
	s := newTestSweeper(t, scanner, &recordingReporter{}, newMemCursorStore(), 4)

	req := testSweepRequest(t, "10.0.0.0/24", nil)
	req.Workers = 0
	require.NoError(t, s.sweep(context.Background(), req))

	sent := scanner.recorded()
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

func TestSweepPassesTheResolvedOptionsToTheScanner(t *testing.T) {
	scanner := answerAll()
	s := newTestSweeper(t, scanner, &recordingReporter{}, newMemCursorStore(), 10)

	req := testSweepRequestWithOptions(t, "10.0.0.0/24", nil, probe.Options{
		Ping: &pingprobe.Options{Count: 1, Interval: time.Second, Timeout: time.Second},
		SNMP: testSNMPOptions(),
	})
	req.Workers = 4
	require.NoError(t, s.sweep(context.Background(), req))

	sent := scanner.recorded()
	require.Len(t, sent, 1)
	require.NotNil(t, sent[0].Options.Ping)
	require.NotNil(t, sent[0].Options.SNMP)
	assert.Equal(t, 4, sent[0].Workers)
	assert.Len(t, sent[0].Targets, 256)
}

func TestSweepReportsNothingWhenNoAddressAnswers(t *testing.T) {
	reporter := &recordingReporter{}
	s := newTestSweeper(t, silentAll(), reporter, newMemCursorStore(), 10)

	require.NoError(t, s.sweep(context.Background(), testSweepRequest(t, "10.0.0.0/24", nil)))

	assert.Empty(t, reporter.devices)
	assert.Equal(t, 0, reporter.batches)
}

func ms(v int64) *int64 { return &v }

func TestToDiscoveredDevices(t *testing.T) {
	results := []probe.Result{
		{
			Target: "10.0.0.1",
			Ping:   &pingprobe.Reading{Success: true, RTT: 3 * time.Millisecond},
			SNMP:   &snmpprobe.Reading{Success: true, RTT: 7 * time.Millisecond, CredID: "cred-a", SysName: "router-1"},
		},
		{
			Target: "10.0.0.2",
			Ping:   &pingprobe.Reading{FailureReason: "timeout"},
			SNMP:   &snmpprobe.Reading{FailureReason: "timeout"},
		},
		{Target: "10.0.0.3"},
		{},
	}

	got := toDiscoveredDevices("ad-1", "run-1", results)
	require.Len(t, got, 1, "only addresses that answered a probe are reported")

	assert.Equal(t, metadata.DiscoveredDeviceMetadata{
		AutodiscoveryID: "ad-1", RunID: "run-1", IPAddress: "10.0.0.1", Name: "router-1",
		ProbeResults: []metadata.ProbeResult{
			{Kind: kindPing, Status: statusReachable, RttMs: ms(3)},
			{Kind: kindSNMP, Status: statusReachable, CredID: "cred-a", RttMs: ms(7)},
		},
	}, got[0])
}

func TestToDiscoveredDevicesReportsAddressesThatAnswerOnlyOneProbe(t *testing.T) {
	results := []probe.Result{
		{
			// A device is there, but the range's credentials do not open it.
			Target: "10.0.0.1",
			Ping:   &pingprobe.Reading{Success: true},
			SNMP:   &snmpprobe.Reading{FailureReason: "authentication_error"},
		},
		{
			Target: "10.0.0.2",
			Ping:   &pingprobe.Reading{FailureReason: "timeout"},
			SNMP:   &snmpprobe.Reading{Success: true, CredID: "cred-a", SysName: "switch-1"},
		},
	}

	got := toDiscoveredDevices("ad-1", "run-1", results)
	require.Len(t, got, 2)

	assert.Equal(t, metadata.DiscoveredDeviceMetadata{
		AutodiscoveryID: "ad-1", RunID: "run-1", IPAddress: "10.0.0.1",
		ProbeResults: []metadata.ProbeResult{
			{Kind: kindPing, Status: statusReachable, RttMs: ms(0)},
			{Kind: kindSNMP, Status: statusUnreachable, FailureReason: "authentication_error"},
		},
	}, got[0])

	assert.Equal(t, metadata.DiscoveredDeviceMetadata{
		AutodiscoveryID: "ad-1", RunID: "run-1", IPAddress: "10.0.0.2", Name: "switch-1",
		ProbeResults: []metadata.ProbeResult{
			{Kind: kindPing, Status: statusUnreachable, FailureReason: "timeout"},
			{Kind: kindSNMP, Status: statusReachable, CredID: "cred-a", RttMs: ms(0)},
		},
	}, got[1])
}

func TestToDiscoveredDevicesSkipsAProbeThatDidNotRun(t *testing.T) {
	results := []probe.Result{{Target: "10.0.0.1", Ping: &pingprobe.Reading{Success: true}}}

	got := toDiscoveredDevices("ad-1", "run-1", results)
	require.Len(t, got, 1)
	assert.Equal(t, []metadata.ProbeResult{{Kind: kindPing, Status: statusReachable, RttMs: ms(0)}}, got[0].ProbeResults,
		"a probe that did not run contributes no result entry")
}

func TestToDiscoveredDevicesReportsTheKindsInProbeOrder(t *testing.T) {
	results := []probe.Result{{
		Target: "10.0.0.1",
		Ping:   &pingprobe.Reading{Success: true},
		SNMP:   &snmpprobe.Reading{Success: true, SysName: "from-snmp"},
	}}

	got := toDiscoveredDevices("ad-1", "run-1", results)
	require.Len(t, got, 1)
	assert.Equal(t, "from-snmp", got[0].Name)
	assert.Equal(t, []string{kindPing, kindSNMP},
		[]string{got[0].ProbeResults[0].Kind, got[0].ProbeResults[1].Kind})
}
