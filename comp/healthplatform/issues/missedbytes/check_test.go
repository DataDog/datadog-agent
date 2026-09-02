// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package missedbytes

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

// The tracker is a singleton, so these tests must not run in parallel.
func newTestChecker(t *testing.T, hostname string) *checker {
	t.Helper()
	logsmetrics.ResetMissedBytesForTest()
	logsmetrics.ResetPipelineMonitorForTest()
	t.Cleanup(logsmetrics.ResetMissedBytesForTest)
	t.Cleanup(logsmetrics.ResetPipelineMonitorForTest)
	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname(hostname))
	return newChecker(hn)
}

func reportSources(t *testing.T, ctx map[string]string) []sourceLoss {
	t.Helper()
	var got []sourceLoss
	require.NoError(t, json.Unmarshal([]byte(ctx[contextKeySources]), &got))
	return got
}

func reportBackpressure(t *testing.T, ctx map[string]string) backpressureWire {
	t.Helper()
	var got backpressureWire
	require.NoError(t, json.Unmarshal([]byte(ctx[contextKeyBackpressure]), &got))
	return got
}

// The tracker is empty for reasons unrelated to loss, and the scheduler reads zero
// issues as "everything resolved".
func TestCheck_LogsAgentNotRunningErrors(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.RecordMissedBytes("nginx", "web", 1024)

	reports, err := c.Run()
	require.ErrorIs(t, err, errLogsAgentNotRunning)
	assert.Empty(t, reports, "an unestablished state must not be reported as no-loss")
}

func TestCheck_NoLossReportsNothing(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()

	reports, err := c.Run()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

// The backend keeps one row per issue type, so per-tuple reports would fight for it.
func TestCheck_LossProducesOneSummaryReport(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	logsmetrics.RecordMissedBytes("nginx", "web", 4000000)
	logsmetrics.RecordMissedBytes("nginx", "web", 200000)
	logsmetrics.RecordMissedBytes("redis", "cache", 512)

	reports, err := c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1, "every tuple on the host folds into one issue")

	report := reports[0]
	assert.Equal(t, IssueName, report.IssueName)
	assert.Equal(t, issueSource, report.Source)
	assert.Equal(t, hostIssueID("host-a"), report.IssueID)

	assert.Equal(t, "4200512", report.Context[contextKeyBytes], "totals sum across every tuple")
	assert.Equal(t, "3", report.Context[contextKeyRotations])
	assert.Equal(t, "2", report.Context[contextKeySourceCount])
	assert.Equal(t, "0", report.Context[contextKeyPairsOmitted])
	assert.NotEmpty(t, report.Context[contextKeyLastLossAt])

	assert.Equal(t, []sourceLoss{
		{Source: "nginx", Service: "web", Bytes: 4200000, Rotations: 2},
		{Source: "redis", Service: "cache", Bytes: 512, Rotations: 1},
	}, reportSources(t, report.Context))
}

// One entry per service under a shared source is the norm for container file tailing.
func TestCheck_SourceCountIgnoresServices(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	logsmetrics.RecordMissedBytes("nginx", "web", 4000000)
	logsmetrics.RecordMissedBytes("nginx", "api", 200000)

	reports, err := c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	ctx := reports[0].Context

	assert.Equal(t, "1", ctx[contextKeySourceCount], "two services of one source are one source")
	assert.Len(t, reportSources(t, ctx), 2, "the breakdown still lists both tuples")

	issue, err := MissedBytesIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Lost 4.2 MB of logs from 1 source in the last 24 hours", issue.GetTitle())
}

// The breakdown is capped, so it must spend its slots on the worst offenders.
// source-00 and source-01 are dropped yet still counted, so this also covers a
// source living only in a tuple BuildIssue never sees.
func TestCheck_BreakdownKeepsLargestSourcesAndCountsTheRest(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()

	// Named so snapshot order is the reverse of byte order: source-00 loses least.
	const total = maxBreakdownSources + 2
	for i := 0; i < total; i++ {
		logsmetrics.RecordMissedBytes(fmt.Sprintf("source-%02d", i), "svc", int64(i+1)*1000)
	}

	reports, err := c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	ctx := reports[0].Context

	assert.Equal(t, strconv.Itoa(total), ctx[contextKeySourceCount], "the count covers every source, not just the listed ones")
	assert.Equal(t, "2", ctx[contextKeyPairsOmitted])

	got := reportSources(t, ctx)
	require.Len(t, got, maxBreakdownSources)
	assert.Equal(t, "source-11", got[0].Source, "largest loss first")
	assert.Equal(t, int64(12000), got[0].Bytes)
	assert.Equal(t, "source-02", got[len(got)-1].Source, "the two smallest losses are the ones dropped")

	var want int64
	for i := 0; i < total; i++ {
		want += int64(i+1) * 1000
	}
	assert.Equal(t, strconv.FormatInt(want, 10), ctx[contextKeyBytes])
}

// The report is re-sent every egress interval, so ties must not churn the payload.
func TestCheck_BreakdownOrderIsDeterministicOnTies(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	logsmetrics.RecordMissedBytes("beta", "two", 1000)
	logsmetrics.RecordMissedBytes("alpha", "two", 1000)
	logsmetrics.RecordMissedBytes("alpha", "one", 1000)

	reports, err := c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)

	got := reportSources(t, reports[0].Context)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"alpha/one", "alpha/two", "beta/two"}, []string{
		got[0].Source + "/" + got[0].Service,
		got[1].Source + "/" + got[1].Service,
		got[2].Source + "/" + got[2].Service,
	}, "ties break on source then service")
}

// An unstable id would file a new issue each tick instead of refreshing one.
func TestHostIssueID_StableAndHostScoped(t *testing.T) {
	base := hostIssueID("host-a")

	assert.Equal(t, base, hostIssueID("host-a"))
	assert.True(t, strings.HasPrefix(base, IssueID+":"), "id %q must keep the kebab-case prefix", base)
	assert.NotEqual(t, base, hostIssueID("host-b"), "hostname must scope the id")
}

// The pipeline snapshot is enrichment: a loss must still be reported when nothing can say
// why it happened.
func TestCheck_NoPipelineMonitorOmitsBackpressure(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	logsmetrics.RecordMissedBytes("nginx", "web", 1024)

	reports, err := c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)

	assert.NotContains(t, reports[0].Context, contextKeyBackpressure,
		"an unread pipeline must not be encoded as a healthy one")
	assert.Equal(t, "1024", reports[0].Context[contextKeyBytes], "the loss must still be reported")
	assert.Empty(t, reportSources(t, reports[0].Context)[0].Bottleneck)
}

func TestCheck_BackpressureCarriesBottleneck(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	logsmetrics.RegisterFakePipelineMonitorForTest([]logsmetrics.ComponentSnapshot{
		logsmetrics.SaturatedSnapshotForTest("processor", "0", 0.12, 0, false),
		logsmetrics.SaturatedSnapshotForTest("destination_reliable_0", "0", 0.98, 29*time.Minute, true),
	})
	logsmetrics.RecordMissedBytes("nginx", "web", 1024)

	reports, err := c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)

	bp := reportBackpressure(t, reports[0].Context)
	assert.Equal(t, logsmetrics.BackpressureSaturated, bp.State)
	require.NotNil(t, bp.Bottleneck)
	assert.Equal(t, "destination_reliable_0", bp.Bottleneck.Component)
	assert.Equal(t, int64(29*60), bp.Bottleneck.Saturated30mSeconds)
	assert.Equal(t, 0, bp.ComponentsOmitted)
	require.Len(t, bp.Components, 2)
	assert.Equal(t, "destination_reliable_0", bp.Components[0].Component, "worst component first")

	// The loss was recorded against the live pipeline, so the tuple carries the same stage.
	sources := reportSources(t, reports[0].Context)
	require.Len(t, sources, 1)
	assert.Equal(t, "destination_reliable_0", sources[0].Bottleneck)
	assert.Equal(t, int64(1), sources[0].BottleneckRotations)
}

// A healthy pipeline at loss time is the signal that says "raise close_timeout".
func TestCheck_HealthyPipelineRecordsNoBottleneck(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	logsmetrics.RegisterFakePipelineMonitorForTest([]logsmetrics.ComponentSnapshot{
		logsmetrics.SaturatedSnapshotForTest("processor", "0", 0.05, 0, false),
	})
	logsmetrics.RecordMissedBytes("nginx", "web", 1024)

	reports, err := c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)

	bp := reportBackpressure(t, reports[0].Context)
	assert.Equal(t, logsmetrics.BackpressureHealthy, bp.State)
	assert.Nil(t, bp.Bottleneck)
	assert.Equal(t, logsmetrics.NoBottleneck, reportSources(t, reports[0].Context)[0].Bottleneck)
}

func TestCheck_BackpressureComponentsAreCappedAndCounted(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()

	const overflow = 4
	snaps := make([]logsmetrics.ComponentSnapshot, 0, maxBackpressureComponents+overflow)
	for i := 0; i < maxBackpressureComponents+overflow; i++ {
		// Descending saturation, so the ranking has an unambiguous worst-first order.
		snaps = append(snaps, logsmetrics.SaturatedSnapshotForTest(
			fmt.Sprintf("destination_%02d", i), "0", 0.9, time.Duration(100-i)*time.Second, false))
	}
	logsmetrics.RegisterFakePipelineMonitorForTest(snaps)
	logsmetrics.RecordMissedBytes("nginx", "web", 1024)

	reports, err := c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)

	bp := reportBackpressure(t, reports[0].Context)
	assert.Len(t, bp.Components, maxBackpressureComponents)
	assert.Equal(t, overflow, bp.ComponentsOmitted)
	assert.Equal(t, "destination_00", bp.Components[0].Component,
		"the cap must keep the most saturated components, not an arbitrary ten")
}

// The backend stores one row per issue type; a payload that churns every tick is noise.
func TestCheck_BackpressureEncodingIsStable(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	logsmetrics.RegisterFakePipelineMonitorForTest([]logsmetrics.ComponentSnapshot{
		// A ratio with more precision than the wire keeps.
		logsmetrics.SaturatedSnapshotForTest("worker", "q0s0", 0.98123456789, time.Minute, true),
		logsmetrics.SaturatedSnapshotForTest("strategy", "0", 0.98123456789, time.Minute, true),
	})
	logsmetrics.RecordMissedBytes("nginx", "web", 1024)

	first, err := c.Run()
	require.NoError(t, err)
	second, err := c.Run()
	require.NoError(t, err)

	assert.Equal(t, first[0].Context[contextKeyBackpressure], second[0].Context[contextKeyBackpressure])
	assert.Contains(t, first[0].Context[contextKeyBackpressure], "0.981")
	assert.NotContains(t, first[0].Context[contextKeyBackpressure], "0.98123",
		"ratios must be rounded so float noise does not churn the payload")
}

func TestDominantBottleneck(t *testing.T) {
	tests := []struct {
		name          string
		counts        map[string]int64
		wantComponent string
		wantRotations int64
	}{
		{name: "nil counts attribute nothing", counts: nil},
		{
			name:          "the stage blamed most often wins",
			counts:        map[string]int64{"strategy": 3, "worker": 9},
			wantComponent: "worker",
			wantRotations: 9,
		},
		{
			name:          "ties break on name so ticks are byte-identical",
			counts:        map[string]int64{"worker": 5, "processor": 5, "strategy": 5},
			wantComponent: "processor",
			wantRotations: 5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			component, rotations := dominantBottleneck(tc.counts)
			assert.Equal(t, tc.wantComponent, component)
			assert.Equal(t, tc.wantRotations, rotations)
		})
	}
}
