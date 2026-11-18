// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package connectivity

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func TestFailingFor(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)}
	tr := NewTracker(clock.now)

	_, failing := tr.FailingFor(0)
	assert.False(t, failing, "no failure recorded")

	start := clock.t
	tr.ReportFailure(errors.New("first"))
	clock.t = clock.t.Add(time.Minute - time.Second)
	_, failing = tr.FailingFor(time.Minute)
	assert.False(t, failing, "below threshold")

	clock.t = clock.t.Add(time.Second)
	tr.ReportFailure(errors.New("connection refused"))
	f, failing := tr.FailingFor(time.Minute)
	require.True(t, failing)
	assert.Equal(t, start, f.Since, "later failures keep the original start")
	assert.Equal(t, "connection refused", f.LastError)

	tr.ReportSuccess()
	_, failing = tr.FailingFor(0)
	assert.False(t, failing, "success clears the failure")

	tr.ReportFailure(errors.New("connection refused"))
	f, failing = tr.FailingFor(0)
	require.True(t, failing)
	assert.Equal(t, clock.t, f.Since, "a failure after a success restarts the clock")
}

func TestReportFailure_ErrorText(t *testing.T) {
	tr := NewTracker(time.Now)
	tr.ReportFailure(nil)
	f, _ := tr.FailingFor(0)
	assert.Equal(t, "unknown error", f.LastError)

	tr.ReportFailure(errors.New(strings.Repeat("x", 2*maxErrorLength)))
	f, _ = tr.FailingFor(0)
	assert.Len(t, f.LastError, maxErrorLength+len("..."))
}
