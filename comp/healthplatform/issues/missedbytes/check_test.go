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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

// The tracker is a singleton, so these tests must not run in parallel.
func newTestChecker(t *testing.T, hostname string) *checker {
	t.Helper()
	logsmetrics.ResetMissedBytesForTest()
	t.Cleanup(logsmetrics.ResetMissedBytesForTest)
	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname(hostname))
	return newChecker(hn)
}

func reportSources(t *testing.T, ctx map[string]string) []sourceLoss {
	t.Helper()
	var got []sourceLoss
	require.NoError(t, json.Unmarshal([]byte(ctx[contextKeySources]), &got))
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
