// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	ndmdiscovery "github.com/DataDog/datadog-agent/comp/ndmdiscovery/def"
)

const statusNowMs = 1700000000000

func newStatusTestDiscovery(t *testing.T, workers int64) (*ndmDiscovery, *memCursorStore) {
	t.Helper()
	cursors := newMemCursorStore()
	sw := newTestSweeper(t, answerAll(), &recordingReporter{}, cursors, workers)
	s := newScheduler(sw, logmock.New(t), schedulerOptions{
		Workers:      workers,
		MaxAddresses: 65536,
		Credentials:  testSchedulerStore(),
	})
	return &ndmDiscovery{log: logmock.New(t), sched: s, enabled: true}, cursors
}

func addStatusRange(d *ndmDiscovery, cfg rangeConfig) {
	d.sched.ranges[cfg.AutodiscoveryID] = &scheduledRange{cfg: cfg, cancel: func() {}}
}

func statusAt(d *ndmDiscovery, nowMs int64) discoveryStatus {
	return statusProvider{d: d, now: func() time.Time { return time.UnixMilli(nowMs) }}.status()
}

func TestStatusShowsAScanningRangeWithItsProgress(t *testing.T) {
	d, cursors := newStatusTestDiscovery(t, 10)
	addStatusRange(d, testRangeConfig("ad-1", "10.0.0.0/24"))
	require.NoError(t, cursors.Save("ad-1", cursorState{RunID: "run-1", Scanned: 128, StartedAtMs: statusNowMs}))

	got := statusAt(d, statusNowMs)

	require.Len(t, got.Ranges, 1)
	r := got.Ranges[0]
	assert.Equal(t, stateScanning, r.State)
	assert.Equal(t, "run-1", r.RunID)
	assert.Equal(t, int64(128), r.Scanned)
	assert.Equal(t, int64(256), r.Total)
	assert.Equal(t, int64(50), r.ProgressPct)
	assert.Equal(t, "snmp", r.Probes)
	assert.Equal(t, "2023-11-14T22:13:20Z", r.StartedAt)
	assert.Empty(t, r.NextRunAt)
	assert.Empty(t, r.NextRunIn)
}

func TestStatusShowsTheNextRunOfAWaitingRange(t *testing.T) {
	d, cursors := newStatusTestDiscovery(t, 10)
	addStatusRange(d, testRangeConfig("ad-1", "10.0.0.0/24"))
	require.NoError(t, cursors.Save("ad-1", cursorState{RunID: "run-1", Scanned: 256, StartedAtMs: statusNowMs, EndedAtMs: statusNowMs}))

	got := statusAt(d, statusNowMs+(10*time.Minute).Milliseconds())

	r := got.Ranges[0]
	assert.Equal(t, stateWaiting, r.State)
	assert.Equal(t, int64(100), r.ProgressPct)
	assert.Equal(t, "2023-11-14T23:13:20Z", r.NextRunAt)
	assert.Equal(t, "50m0s", r.NextRunIn)
}

func TestStatusShowsAnOverdueRangeAsDue(t *testing.T) {
	d, cursors := newStatusTestDiscovery(t, 10)
	addStatusRange(d, testRangeConfig("ad-1", "10.0.0.0/24"))
	require.NoError(t, cursors.Save("ad-1", cursorState{RunID: "run-1", EndedAtMs: statusNowMs}))

	got := statusAt(d, statusNowMs+(2*time.Hour).Milliseconds())

	assert.Equal(t, "due", got.Ranges[0].NextRunIn)
}

func TestStatusShowsAFailedCycle(t *testing.T) {
	d, cursors := newStatusTestDiscovery(t, 10)
	addStatusRange(d, testRangeConfig("ad-1", "10.0.0.0/24"))
	require.NoError(t, cursors.Save("ad-1", cursorState{RunID: "run-1", Failed: true, EndedAtMs: statusNowMs}))

	got := statusAt(d, statusNowMs)

	assert.Equal(t, stateFailed, got.Ranges[0].State)
}

func TestStatusShowsARangeWithoutACursorAsPending(t *testing.T) {
	d, _ := newStatusTestDiscovery(t, 10)
	addStatusRange(d, testRangeConfig("ad-1", "10.0.0.0/24"))

	got := statusAt(d, statusNowMs)

	r := got.Ranges[0]
	assert.Equal(t, statePending, r.State)
	assert.Empty(t, r.RunID)
	assert.Equal(t, int64(256), r.Total)
}

func TestStatusTotalLeavesOutIgnoredAddresses(t *testing.T) {
	d, _ := newStatusTestDiscovery(t, 10)
	cfg := testRangeConfig("ad-1", "10.0.0.0/24")
	cfg.IgnoredIPAddresses = []string{"10.0.0.7", "192.168.0.1"}
	addStatusRange(d, cfg)

	got := statusAt(d, statusNowMs)

	assert.Equal(t, int64(255), got.Ranges[0].Total)
}

func TestStatusSplitsTheWorkersAcrossRanges(t *testing.T) {
	d, _ := newStatusTestDiscovery(t, 10)
	addStatusRange(d, testRangeConfig("ad-1", "10.0.0.0/24"))
	addStatusRange(d, testRangeConfig("ad-2", "10.0.1.0/24"))

	got := statusAt(d, statusNowMs)

	assert.Equal(t, int64(10), got.Workers)
	require.Len(t, got.Ranges, 2)
	assert.Equal(t, "10.0.0.0/24", got.Ranges[0].NetworkAddress)
	assert.Equal(t, int64(5), got.Ranges[0].Workers)
	assert.Equal(t, int64(5), got.Ranges[1].Workers)
}

func TestStatusListsDroppedProbesAndCycleIssues(t *testing.T) {
	d, _ := newStatusTestDiscovery(t, 10)
	cfg := testRangeConfig("ad-1", "10.0.0.0/24")
	cfg.Probes.Dropped = []string{"dropping the unknown probe \"ssh\""}
	addStatusRange(d, cfg)
	d.sched.setPrepared("ad-1", []string{"credential \"cred-b\" is not available on this agent"})
	d.sched.setSweepError("ad-1", "boom")

	got := statusAt(d, statusNowMs)

	assert.Equal(t, []string{
		"dropping the unknown probe \"ssh\"",
		"credential \"cred-b\" is not available on this agent",
		"last sweep failed: boom",
	}, got.Ranges[0].Issues)
}

func TestRunCycleRecordsARangeWithNoUsableProbe(t *testing.T) {
	d, _ := newStatusTestDiscovery(t, 10)
	cfg := testRangeConfig("ad-1", "10.0.0.0/24")
	cfg.Probes.SNMP.CredIDs = []string{"cred-missing"}
	addStatusRange(d, cfg)

	d.sched.runCycle(context.Background(), cfg, time.Hour)

	issues := statusAt(d, statusNowMs).Ranges[0].Issues
	require.NotEmpty(t, issues)
	assert.Equal(t, "skipped: it has no usable probe", issues[len(issues)-1])
}

func TestReplacingARangeClearsItsSweepError(t *testing.T) {
	d, _ := newStatusTestDiscovery(t, 10)
	d.sched.start(context.Background())
	t.Cleanup(d.sched.stop)
	cfg := testRangeConfig("ad-1", "10.0.0.0/24")
	addStatusRange(d, cfg)
	d.sched.setSweepError("ad-1", "boom")

	require.NoError(t, d.sched.set(cfg))

	for _, issue := range statusAt(d, statusNowMs).Ranges[0].Issues {
		assert.NotContains(t, issue, "boom")
	}
}

func TestScheduleRecordsTheRangesItRejects(t *testing.T) {
	comp, _ := newScheduleTestComponent(t)
	d := comp.(*ndmDiscovery)
	snmp := map[string]json.RawMessage{"snmp": json.RawMessage(`{"cred_ids":["cred-a"]}`)}

	errs := comp.Schedule([]ndmdiscovery.Range{
		{ID: "ad-1", NetworkAddress: "10.0.0.0/24", Probes: snmp},
		{ID: "ad-big", NetworkAddress: "11.0.0.0/12", Probes: snmp},
	})

	require.Contains(t, errs, "ad-big")
	got := statusProvider{d: d}.status()
	require.Len(t, got.Rejected, 1)
	assert.Equal(t, "ad-big", got.Rejected[0].ID)
	assert.Equal(t, "11.0.0.0/12", got.Rejected[0].NetworkAddress)
	assert.Contains(t, got.Rejected[0].Error, "exceeds the maximum of 65536")
	require.Len(t, got.Ranges, 1)
	assert.Equal(t, "ad-1", got.Ranges[0].ID)

	comp.Schedule([]ndmdiscovery.Range{{ID: "ad-1", NetworkAddress: "10.0.0.0/24", Probes: snmp}})

	assert.Empty(t, statusProvider{d: d}.status().Rejected)
}

func TestStatusRendersTextAndHTML(t *testing.T) {
	d, cursors := newStatusTestDiscovery(t, 10)
	addStatusRange(d, testRangeConfig("ad-1", "10.0.0.0/24"))
	require.NoError(t, cursors.Save("ad-1", cursorState{RunID: "run-1", Scanned: 256, EndedAtMs: statusNowMs}))
	d.rejected = []rejectedRange{{ID: "ad-big", NetworkAddress: "11.0.0.0/12", Error: "too large"}}
	p := statusProvider{d: d}

	var text bytes.Buffer
	require.NoError(t, p.Text(false, &text))
	assert.Contains(t, text.String(), "10.0.0.0/24 (ad-1)")
	assert.Contains(t, text.String(), "State: waiting")
	assert.Contains(t, text.String(), "Next run:")
	assert.Contains(t, text.String(), "11.0.0.0/12 (ad-big): too large")

	var html bytes.Buffer
	require.NoError(t, p.HTML(false, &html))
	assert.Contains(t, html.String(), "10.0.0.0/24 (ad-1)")
	assert.Contains(t, html.String(), "too large")

	stats := map[string]interface{}{}
	require.NoError(t, p.JSON(false, stats))
	assert.Contains(t, stats, "ndmDiscoveryStats")
}
