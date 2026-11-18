// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package clusteragentunreachable

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent/connectivity"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newTestChecker() (*checker, *connectivity.Tracker, *fakeClock) {
	clock := &fakeClock{t: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)}
	tr := connectivity.NewTracker(clock.now)
	hostname, _ := hostnamemock.NewMock("node-1")
	return newChecker(tr, hostname), tr, clock
}

func TestCheck_NotReadyDuringStartup(t *testing.T) {
	c, tr, clock := newTestChecker()
	reports, err := c.Check()
	assert.ErrorIs(t, err, runnerdef.ErrNotReady)
	assert.Empty(t, reports)

	tr.ReportFailure(errors.New("connection refused"))
	clock.t = clock.t.Add(failureThreshold / 2)
	reports, err = c.Check()
	assert.ErrorIs(t, err, runnerdef.ErrNotReady, "an unconfirmed failure keeps a restored issue")
	assert.Empty(t, reports)
}

func TestCheck_NoFailureAfterStartup(t *testing.T) {
	c, _, clock := newTestChecker()
	clock.t = clock.t.Add(startupGrace)
	reports, err := c.Check()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestCheck_SuccessEndsStartup(t *testing.T) {
	c, tr, _ := newTestChecker()
	tr.ReportSuccess()
	reports, err := c.Check()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestCheck_RestartDuringOutage(t *testing.T) {
	c, tr, clock := newTestChecker()
	for i := 0; i <= 6; i++ {
		tr.ReportFailure(errors.New("connection refused"))
		reports, err := c.Check()
		if i < 6 {
			assert.ErrorIs(t, err, runnerdef.ErrNotReady, "tick %d", i)
			assert.Empty(t, reports)
		} else {
			require.NoError(t, err)
			assert.Len(t, reports, 1, "the outage is confirmed once failures span the threshold")
		}
		clock.t = clock.t.Add(10 * time.Second)
	}
}

func TestCheck_FailureBelowThreshold(t *testing.T) {
	c, tr, clock := newTestChecker()
	tr.ReportSuccess()
	tr.ReportFailure(errors.New("connection refused"))
	clock.t = clock.t.Add(failureThreshold - time.Second)
	tr.ReportFailure(errors.New("connection refused"))

	reports, err := c.Check()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestCheck_LoneFailureDoesNotFire(t *testing.T) {
	c, tr, clock := newTestChecker()
	tr.ReportSuccess()
	tr.ReportFailure(errors.New("context deadline exceeded"))
	clock.t = clock.t.Add(10 * failureThreshold)

	reports, err := c.Check()
	require.NoError(t, err)
	assert.Empty(t, reports, "one failure followed by silence is not an outage")
}

func TestCheck_SustainedFailure(t *testing.T) {
	c, tr, clock := newTestChecker()
	start := clock.t
	tr.ReportFailure(errors.New("first"))
	clock.t = clock.t.Add(failureThreshold)
	tr.ReportFailure(errors.New("dial tcp 10.0.0.1:5005: connect: connection refused"))

	reports, err := c.Check()
	require.NoError(t, err)
	require.Len(t, reports, 1)

	r := reports[0]
	assert.Equal(t, hostIssueID("node-1"), r.IssueID)
	assert.True(t, strings.HasPrefix(r.IssueID, IssueID+":"))
	assert.Equal(t, IssueName, r.IssueName)
	assert.Equal(t, "dial tcp 10.0.0.1:5005: connect: connection refused", r.Context[contextKeyError])
	assert.Equal(t, start.Format(time.RFC3339), r.Context[contextKeyFailingSince])
	assert.Equal(t, "node-1", r.Context[contextKeyHostname])
}

func TestCheck_SuccessClearsFailure(t *testing.T) {
	c, tr, clock := newTestChecker()
	tr.ReportFailure(errors.New("connection refused"))
	clock.t = clock.t.Add(failureThreshold)
	tr.ReportSuccess()

	reports, err := c.Check()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestCheck_ScrubsAndTruncatesError(t *testing.T) {
	c, tr, clock := newTestChecker()
	secretErr := errors.New(`Get "https://user:hunter2@dca:5005/version": connection refused`)
	tr.ReportFailure(secretErr)
	clock.t = clock.t.Add(failureThreshold)
	tr.ReportFailure(secretErr)

	reports, err := c.Check()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.NotContains(t, reports[0].Context[contextKeyError], "hunter2")
	assert.Contains(t, reports[0].Context[contextKeyError], "connection refused")

	tr.ReportFailure(errors.New(strings.Repeat("x", 2*maxErrorLength)))
	reports, err = c.Check()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Len(t, reports[0].Context[contextKeyError], maxErrorLength+len("..."))
}

func TestHostIssueID_PerHost(t *testing.T) {
	assert.NotEqual(t, hostIssueID("node-1"), hostIssueID("node-2"))
	assert.Equal(t, hostIssueID("node-1"), hostIssueID("node-1"))
}
