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

func TestCheck_NoFailure(t *testing.T) {
	c, _, _ := newTestChecker()
	reports, err := c.Check()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestCheck_FailureBelowThreshold(t *testing.T) {
	c, tr, clock := newTestChecker()
	tr.ReportFailure(errors.New("connection refused"))
	clock.t = clock.t.Add(failureThreshold - time.Second)

	reports, err := c.Check()
	require.NoError(t, err)
	assert.Empty(t, reports)
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

func TestHostIssueID_PerHost(t *testing.T) {
	assert.NotEqual(t, hostIssueID("node-1"), hostIssueID("node-2"))
	assert.Equal(t, hostIssueID("node-1"), hostIssueID("node-1"))
}
