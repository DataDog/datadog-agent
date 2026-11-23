// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsdropped

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/internal/logsissue"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

const intake = "agent-http-intake.Logs.datadoghq.com"

var (
	primary       = logsmetrics.DropTarget{Host: intake, Main: true}
	additional    = logsmetrics.DropTarget{Host: "intake.eu"}
	beforeSending = logsmetrics.DropTarget{Main: true}
)

// The tracker is a singleton, so these tests must not run in parallel.
func newTestChecker(t *testing.T, hostname string) *checker {
	t.Helper()
	c, _, _ := newTestCheckerWithCounter(t, hostname)
	return c
}

// newTestCheckerWithCounter returns a checker reading a fake logs-processed counter
// and clock, both of which the test advances.
func newTestCheckerWithCounter(t *testing.T, hostname string) (*checker, *int64, *time.Time) {
	t.Helper()
	logsmetrics.ResetDroppedLogsForTest()
	t.Cleanup(logsmetrics.ResetDroppedLogsForTest)
	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname(hostname))
	processed := new(int64)
	// The tracker stamps incidents with the real clock, so the fake one starts there.
	now := new(time.Time)
	*now = time.Now()
	c := newCheckerWith(hn, func() int64 { return *processed }, func() time.Time { return *now })
	return c, processed, now
}

func record(reason logsmetrics.DropReason, target logsmetrics.DropTarget, source, service string, logs int64) {
	logsmetrics.RecordDroppedLogs(reason, target, map[logsmetrics.DropOrigin]logsmetrics.DropCount{
		{Source: source, Service: service}: {Logs: logs, Bytes: logs * 100},
	})
}

func runOnce(t *testing.T, c *checker) map[string]string {
	t.Helper()
	reports, err := c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1, "every drop on the host folds into one issue")
	return reports[0].Context
}

// The tracker is empty for reasons unrelated to loss, and the scheduler reads zero
// issues as "everything resolved".
func TestCheck_LogsAgentNotRunningErrors(t *testing.T) {
	c := newTestChecker(t, "host-a")
	record(logsmetrics.DropReasonAuth, primary, "nginx", "web", 1)

	reports, err := c.Run()
	require.ErrorIs(t, err, errLogsAgentNotRunning)
	assert.Empty(t, reports)
}

func TestCheck_NoDropsReportsNothing(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()

	reports, err := c.Run()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestCheck_DropsProduceOneSummaryReport(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	record(logsmetrics.DropReasonAuth, primary, "nginx", "web", 30)
	record(logsmetrics.DropReasonAuth, primary, "redis", "cache", 5)
	record(logsmetrics.DropReasonBufferFull, additional, "nginx", "web", 10)

	reports, err := c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	r := reports[0]
	assert.Equal(t, IssueName, r.IssueName)
	assert.Equal(t, logsissue.HostIssueID(IssueID, "host-a"), r.IssueID)

	ctx := r.Context
	f := parseFacts(ctx)
	assert.Equal(t, int64(35), f.MainLogs)
	assert.Equal(t, int64(3500), f.MainBytes)
	assert.Equal(t, int64(10), f.CopyLogs)
	assert.Equal(t, 2, f.SourceCount, "distinct sources, not rows")
	assert.Equal(t, []sourceLoss{{"nginx", "web", 40}, {"redis", "cache", 5}}, f.TopSources, "largest first, copies included")
	assert.Equal(t, []logsmetrics.DropReason{logsmetrics.DropReasonAuth, logsmetrics.DropReasonBufferFull}, f.Reasons)
	assert.Equal(t, logsmetrics.DropReasonAuth, f.TopReason)
	assert.Equal(t, []string{intake}, f.Endpoints[logsmetrics.DropReasonAuth])
	assert.Equal(t, []string{"intake.eu"}, f.Endpoints[logsmetrics.DropReasonBufferFull])
	assert.False(t, f.LastDropAt.IsZero())

	// The context must build a valid issue: the runner calls BuildIssue next.
	issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assert.Contains(t, issue.Title, "Dropped 35 logs", "the title counts the main endpoint, not the copy")
}

func TestCheck_AdditionalOnlyIsNotMain(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	record(logsmetrics.DropReasonBufferFull, additional, "nginx", "web", 10)
	record(logsmetrics.DropReasonSendFailed, logsmetrics.DropTarget{Host: "copy"}, "nginx", "web", 1)

	f := parseFacts(runOnce(t, c))
	assert.Zero(t, f.MainLogs)
	assert.Equal(t, int64(11), f.CopyLogs)
}

func TestCheck_DroppedBeforeSendingIsMainWithoutEndpoint(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	record(logsmetrics.DropReasonTooLarge, beforeSending, "java", "api", 1)

	f := parseFacts(runOnce(t, c))
	assert.Equal(t, int64(1), f.MainLogs)
	_, ok := f.Endpoints[logsmetrics.DropReasonTooLarge]
	assert.False(t, ok, "no host to name")
}

// Main-endpoint loss picks the headline reason even when copies lost more.
func TestCheck_TopReasonPrefersMainEndpoint(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	record(logsmetrics.DropReasonBufferFull, additional, "nginx", "web", 5000)
	record(logsmetrics.DropReasonAuth, primary, "nginx", "web", 3)

	assert.Equal(t, logsmetrics.DropReasonAuth, parseFacts(runOnce(t, c)).TopReason)
}

func TestCheck_CapsSourcesAndEndpoints(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	for i := 0; i < maxTopSources+3; i++ {
		record(logsmetrics.DropReasonAuth, primary, fmt.Sprintf("source-%02d", i), "svc", int64(i+1))
	}
	for i := 0; i < maxEndpoints+2; i++ {
		record(logsmetrics.DropReasonSendFailed, logsmetrics.DropTarget{Host: fmt.Sprintf("copy-%d", i)}, "source-00", "svc", 1)
	}

	f := parseFacts(runOnce(t, c))
	assert.Len(t, f.TopSources, maxTopSources)
	assert.Equal(t, "source-00", f.TopSources[0].Source, "1 main log plus 7 copies ties source-07, and wins on name")
	assert.Equal(t, maxTopSources+3, f.SourceCount)
	assert.Len(t, f.Endpoints[logsmetrics.DropReasonSendFailed], maxEndpoints)
}

// Consecutive ticks must render the same issue, or the backend sees churn.
func TestCheck_StableAcrossTicks(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	for i := 0; i < 20; i++ {
		record(logsmetrics.DropReasonAuth, primary, fmt.Sprintf("source-%02d", i), "svc", 7)
		record(logsmetrics.DropReasonSendFailed, logsmetrics.DropTarget{Host: fmt.Sprintf("copy-%02d", i)}, "a", "b", 7)
	}

	assert.Equal(t, runOnce(t, c), runOnce(t, c))
}

// The denominator is the logs processed since the incident started, not since the
// agent started: a drop after a day of clean traffic is measured against the
// traffic around it.
func TestCheck_ProcessedSinceIncidentStart(t *testing.T) {
	logsmetrics.ResetDroppedLogsForTest()
	t.Cleanup(logsmetrics.ResetDroppedLogsForTest)
	logsmetrics.MarkLogsAgentRunning()
	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname("host-a"))

	// Fifteen-minute checks for the 30 hours before now, 1000 logs between each.
	var processed int64
	now := time.Now().Add(-30 * time.Hour)
	c := newCheckerWith(hn, func() int64 { return processed }, func() time.Time { return now })
	for i := 0; i < 119; i++ {
		now = now.Add(15 * time.Minute)
		processed += 1000
		c.sample()
	}
	assert.LessOrEqual(t, len(c.samples), int(processedRetention/(15*time.Minute))+2, "samples stay bounded")

	record(logsmetrics.DropReasonBufferFull, primary, "nginx", "web", 3)
	now = time.Now().Add(15 * time.Minute)
	processed += 1000
	ctx := runOnce(t, c)
	f := parseFacts(ctx)
	assert.Equal(t, int64(1000), f.Processed, "only the interval the incident started in")
	assert.Equal(t, int64(3), f.MainLogs)
}

// Copies lost at other endpoints don't set the ratio's stretch: it follows the main
// endpoint's earliest incident.
func TestMainIncidentStart(t *testing.T) {
	t0 := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	drops := []logsmetrics.DroppedLogsSummary{
		{Target: additional, StartedAt: t0},
		{Target: primary, StartedAt: t0.Add(2 * time.Hour)},
		{Target: beforeSending, StartedAt: t0.Add(time.Hour)},
	}
	assert.Equal(t, t0.Add(time.Hour), mainIncidentStart(drops))
	assert.True(t, mainIncidentStart(drops[:1]).IsZero(), "no main-endpoint loss, no stretch")
}

// Right after start the window is just the time since start.
func TestCheck_ProcessedSinceStart(t *testing.T) {
	c, processed, now := newTestCheckerWithCounter(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	*processed = 500
	*now = now.Add(time.Minute)
	record(logsmetrics.DropReasonAuth, primary, "nginx", "web", 500)

	ctx := runOnce(t, c)
	assert.Equal(t, int64(500), parseFacts(ctx).Processed)

	issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assert.Contains(t, issue.Description, "lost 100% of the 500 logs")
}
