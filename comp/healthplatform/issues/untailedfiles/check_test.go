// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package untailedfiles

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/internal/logsissue"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

// The tracker is a singleton, so these tests must not run in parallel.
func newTestChecker(t *testing.T, hostname string) *checker {
	t.Helper()
	logsmetrics.ResetUntailedFilesForTest()
	t.Cleanup(logsmetrics.ResetUntailedFilesForTest)
	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname(hostname))
	return newChecker(hn)
}

func TestCheck_LogsAgentNotRunningErrors(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.RecordUntailedFiles(2, 2, []logsmetrics.UntailedFilesSource{{Source: "nginx", Matched: 5, Untailed: 3}})

	reports, err := c.Run()
	require.ErrorIs(t, err, errLogsAgentNotRunning)
	assert.Empty(t, reports)
}

func TestCheck_LimitNotReachedReportsNothing(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()

	reports, err := c.Run()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestCheck_LimitReachedReports(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	logsmetrics.RecordUntailedFiles(500, 500, []logsmetrics.UntailedFilesSource{
		{Source: "nginx", Service: "web", Path: "/var/log/nginx/*.log", Matched: 320, Untailed: 300},
		{Source: "redis", Service: "cache", Path: "/var/log/redis/*.log", Matched: 60, Untailed: 40},
	})

	reports, err := c.Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)

	r := reports[0]
	assert.Equal(t, IssueName, r.IssueName)
	assert.Equal(t, logsissue.HostIssueID(IssueID, "host-a"), r.IssueID)
	assert.Equal(t, "500", r.Context[contextKeyLimit])
	assert.Equal(t, "500", r.Context[contextKeyTailed])
	assert.Equal(t, "340", r.Context[contextKeyUntailed])
	assert.Equal(t, "2", r.Context[contextKeySourceCount])
	assert.Equal(t, "0", r.Context[contextKeySourcesOmitted])
	assert.NotEmpty(t, r.Context[contextKeyLastSeenAt])

	var sources []sourceFiles
	require.NoError(t, json.Unmarshal([]byte(r.Context[contextKeySources]), &sources))
	require.Len(t, sources, 2)
	assert.Equal(t, sourceFiles{Source: "nginx", Service: "web", Path: "/var/log/nginx/*.log", Matched: 320, Untailed: 300}, sources[0])

	issue, err := UntailedFilesIssue{}.BuildIssue(r.Context)
	require.NoError(t, err)
	assert.Contains(t, issue.Title, "Not tailing 340 of 840")
}

func TestCheck_CapsBreakdown(t *testing.T) {
	c := newTestChecker(t, "host-a")
	logsmetrics.MarkLogsAgentRunning()
	var sources []logsmetrics.UntailedFilesSource
	for i := 0; i < maxBreakdownSources+4; i++ {
		sources = append(sources, logsmetrics.UntailedFilesSource{Source: fmt.Sprintf("source-%02d", i), Matched: 3, Untailed: 1})
	}
	logsmetrics.RecordUntailedFiles(10, 10, sources)

	reports, err := c.Run()
	require.NoError(t, err)
	var got []sourceFiles
	require.NoError(t, json.Unmarshal([]byte(reports[0].Context[contextKeySources]), &got))
	assert.Len(t, got, maxBreakdownSources)
	assert.Equal(t, "4", reports[0].Context[contextKeySourcesOmitted])
	assert.Equal(t, "14", reports[0].Context[contextKeySourceCount])
}
