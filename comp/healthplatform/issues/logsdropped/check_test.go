// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsdropped

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/internal/logsissue"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

var (
	primary    = logsmetrics.DropTarget{Host: intake, Kind: logsmetrics.DropTargetPrimary}
	additional = logsmetrics.DropTarget{Host: "intake.eu", Kind: logsmetrics.DropTargetAdditional}
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
	now := new(time.Time)
	*now = time.Unix(1704067200, 0)
	c := newCheckerWith(hn, func() int64 { return *processed }, func() time.Time { return *now })
	return c, processed, now
}

func record(reason logsmetrics.DropReason, target logsmetrics.DropTarget, source, service string, logs int64) {
	logsmetrics.RecordDroppedLogs(reason, target, map[logsmetrics.DropOrigin]logsmetrics.DropCount{
		{Source: source, Service: service}: {Logs: logs, Bytes: logs * 100},
	})
}

func decode[T any](t *testing.T, s string) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal([]byte(s), &v))
	return v
}

// The tracker is empty for reasons unrelated to loss, and the scheduler reads zero
// issues as "everything resolved".
func TestCheck_LogsAgentNotRunningErrors(t *testing.T) {
	c := newTestChecker(t, "host-a")
	record(logsmetrics.DropReasonAuthRejected, primary, "nginx", "web", 1)

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
	record(logsmetrics.DropReasonAuthRejected, primary, "nginx", "web", 30)
	record(logsmetrics.DropReasonAuthRejected, primary, "redis", "cache", 5)
	record(logsmetrics.DropReasonBufferFull, additional, "nginx", "web", 10)

	reports, err := c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1, "every drop on the host folds into one issue")

	r := reports[0]
	assert.Equal(t, IssueName, r.IssueName)
	assert.Equal(t, logsissue.HostIssueID(IssueID, "host-a"), r.IssueID)
	assert.Equal(t, "45", r.Context[contextKeyLogs])
	assert.Equal(t, "4500", r.Context[contextKeyBytes])
	assert.Equal(t, "2", r.Context[contextKeySourceCount], "distinct sources, not rows")
	assert.Equal(t, "true", r.Context[contextKeyPrimaryAffected])
	assert.NotEmpty(t, r.Context[contextKeyLastDropAt])

	causes := decode[[]cause](t, r.Context[contextKeyCauses])
	require.Len(t, causes, 2)
	assert.Equal(t, cause{Reason: "auth_rejected", Destination: intake, Kind: "primary", Logs: 35, Bytes: 3500}, causes[0])
	assert.Equal(t, "buffer_full", causes[1].Reason)

	drops := decode[[]sourceDrop](t, r.Context[contextKeyDrops])
	require.Len(t, drops, 3)
	assert.Equal(t, int64(30), drops[0].Logs, "largest first")

	// The context must build a valid issue: the runner calls BuildIssue next.
	issue, err := LogsDroppedIssue{}.BuildIssue(r.Context)
	require.NoError(t, err)
	assert.Contains(t, issue.Title, "Dropped 35 logs", "the title counts the main endpoint, not the copy to the additional one")
}

func TestCheck_AdditionalOnlyIsNotPrimary(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	record(logsmetrics.DropReasonBufferFull, additional, "nginx", "web", 10)
	record(logsmetrics.DropReasonSendFailed, logsmetrics.DropTarget{Host: "copy", Kind: logsmetrics.DropTargetBestEffort}, "nginx", "web", 1)

	reports, err := c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, "false", reports[0].Context[contextKeyPrimaryAffected])
}

func TestCheck_DroppedBeforeSendingIsPrimary(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	record(logsmetrics.DropReasonMessageTooLarge, logsmetrics.DropTarget{Kind: logsmetrics.DropTargetAll}, "java", "api", 1)

	reports, err := c.Run()
	require.NoError(t, err)
	assert.Equal(t, "true", reports[0].Context[contextKeyPrimaryAffected])
}

func TestCheck_CapsBreakdown(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	for i := 0; i < maxBreakdownRows+3; i++ {
		record(logsmetrics.DropReasonAuthRejected, primary, fmt.Sprintf("source-%02d", i), "svc", int64(i+1))
	}
	for i := 0; i < maxCauses+2; i++ {
		record(logsmetrics.DropReasonSendFailed, logsmetrics.DropTarget{Host: fmt.Sprintf("copy-%d", i), Kind: logsmetrics.DropTargetBestEffort}, "source-00", "svc", 1)
	}

	reports, err := c.Run()
	require.NoError(t, err)
	ctx := reports[0].Context
	assert.Len(t, decode[[]sourceDrop](t, ctx[contextKeyDrops]), maxBreakdownRows)
	assert.Len(t, decode[[]cause](t, ctx[contextKeyCauses]), maxCauses)
	assert.Equal(t, strconv.Itoa(maxBreakdownRows+3+maxCauses+2-maxBreakdownRows), ctx[contextKeyRowsOmitted])
	assert.Equal(t, "3", ctx[contextKeyCausesOmitted], "1 auth cause plus 7 send causes, minus the 5 kept")
	assert.Equal(t, strconv.Itoa(maxBreakdownRows+3), ctx[contextKeySourceCount])
}

// Consecutive ticks must render the same issue, or the backend sees churn.
func TestCheck_StableAcrossTicks(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	for i := 0; i < 20; i++ {
		record(logsmetrics.DropReasonAuthRejected, primary, fmt.Sprintf("source-%02d", i), "svc", 7)
	}

	first, err := c.Run()
	require.NoError(t, err)
	second, err := c.Run()
	require.NoError(t, err)
	assert.Equal(t, first[0].Context, second[0].Context)
}

// The denominator is the logs processed since the oldest sample still inside the
// window, so severity tracks the same stretch of time as the drops.
func TestCheck_ProcessedWindow(t *testing.T) {
	c, processed, now := newTestCheckerWithCounter(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()

	// Fifteen-minute checks for 30 hours, 1000 logs processed between each.
	for i := 0; i < 120; i++ {
		*now = now.Add(15 * time.Minute)
		*processed += 1000
		c.processedInWindow()
	}
	assert.LessOrEqual(t, len(c.samples), int(processedWindow/(15*time.Minute))+2, "samples stay bounded")

	record(logsmetrics.DropReasonBufferFull, primary, "nginx", "web", 3)
	*now = now.Add(15 * time.Minute)
	*processed += 1000
	reports, err := c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	// 25 hours of 1000-log intervals.
	assert.Equal(t, "100000", reports[0].Context[contextKeyProcessed])
	assert.Equal(t, "3", reports[0].Context[contextKeyPrimaryLogs])
}

// Right after start the window is just the time since start.
func TestCheck_ProcessedSinceStart(t *testing.T) {
	c, processed, now := newTestCheckerWithCounter(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	*processed = 500
	*now = now.Add(time.Minute)
	record(logsmetrics.DropReasonAuthRejected, primary, "nginx", "web", 500)

	reports, err := c.Run()
	require.NoError(t, err)
	assert.Equal(t, "500", reports[0].Context[contextKeyProcessed])

	issue, err := LogsDroppedIssue{}.BuildIssue(reports[0].Context)
	require.NoError(t, err)
	assert.Contains(t, issue.Description, "lost 100% of the 500 logs")
}

// Rows count copies; the main-endpoint totals count logs, and every reason reaches
// the context even when the ranked causes leave it out.
func TestCheck_MainEndpointTotalsAndReasons(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	copyTarget := logsmetrics.DropTarget{Host: "copy", Kind: logsmetrics.DropTargetBestEffort}
	record(logsmetrics.DropReasonAuthRejected, primary, "nginx", "web", 1000)
	record(logsmetrics.DropReasonAuthRejected, copyTarget, "nginx", "web", 1000)
	record(logsmetrics.DropReasonBufferFull, additional, "redis", "cache", 7)
	for i := 0; i < maxCauses; i++ {
		record(logsmetrics.DropReasonSendFailed, logsmetrics.DropTarget{Host: fmt.Sprintf("big-%d", i), Kind: logsmetrics.DropTargetBestEffort}, "nginx", "web", 5000)
	}

	reports, err := c.Run()
	require.NoError(t, err)
	ctx := reports[0].Context
	assert.Equal(t, "1000", ctx[contextKeyPrimaryLogs])
	assert.Equal(t, "100000", ctx[contextKeyPrimaryBytes])
	assert.Equal(t, "1", ctx[contextKeyPrimarySources])
	assert.Equal(t, "auth_rejected,buffer_full,send_failed", ctx[contextKeyReasons])

	causes := decode[[]cause](t, ctx[contextKeyCauses])
	for _, c := range causes {
		assert.NotEqual(t, "buffer_full", c.Reason, "the smallest cause is ranked out")
	}
	issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assert.Contains(t, stepTexts(issue), reasonSteps[logsmetrics.DropReasonBufferFull])
}
