// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodeunschedulable

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSource struct {
	result     Result
	err        error
	probeErr   error
	calls      int
	probeCalls int
}

func (f *fakeSource) NodeUnschedulable() (Result, error) {
	f.calls++
	return f.result, f.err
}

func (f *fakeSource) Probe() error {
	f.probeCalls++
	return f.probeErr
}

func TestNeverReadEmitsNothing(t *testing.T) {
	tr := NewTracker(time.Minute)
	assert.Equal(t, Outcome{}, tr.Outcome())

	// A first read that fails leaves nothing to reuse.
	tr.Refresh(&fakeSource{err: errors.New("connection refused")}, time.Now())
	assert.Equal(t, Outcome{}, tr.Outcome())
}

func TestSuccessStoresTheResult(t *testing.T) {
	tr := NewTracker(time.Minute)
	src := &fakeSource{result: Result{Unschedulable: true, Reasons: []string{"drain", "cordoned"}}}
	tr.Refresh(src, time.Now())
	assert.Equal(t, Outcome{Reason: ReasonUnschedulable, Tags: []string{"unschedulable_reason:cordoned", "unschedulable_reason:drain"}}, tr.Outcome())

	src.result = Result{Reasons: []string{}}
	tr.Refresh(src, time.Now().Add(time.Minute))
	assert.Equal(t, Outcome{}, tr.Outcome(), "a schedulable node reports nothing")
}

func TestRefreshIntervalGating(t *testing.T) {
	tr := NewTracker(time.Minute)
	src := &fakeSource{}
	t0 := time.Now()
	tr.Refresh(src, t0)
	tr.Refresh(src, t0.Add(30*time.Second))
	assert.Equal(t, 1, src.calls)
	tr.Refresh(src, t0.Add(time.Minute))
	assert.Equal(t, 2, src.calls)

	// Failures are gated too: one request per interval.
	src.err = errors.New("boom")
	tr.Refresh(src, t0.Add(90*time.Second))
	assert.Equal(t, 2, src.calls)
	assert.Equal(t, time.Minute, NewTracker(0).refreshInterval, "the default")
}

// A failed read keeps the last result with no time limit.
func TestFailedReadKeepsLastResultWithoutTimeLimit(t *testing.T) {
	for name, err := range map[string]error{
		"transport error": errors.New("connection refused"),
		"server error":    errors.New("remote service error: 500"),
	} {
		t.Run(name, func(t *testing.T) {
			tr := NewTracker(time.Minute)
			src := &fakeSource{result: Result{Unschedulable: true, Reasons: []string{"cordoned"}}}
			t0 := time.Now()
			tr.Refresh(src, t0)
			want := tr.Outcome()
			require.Equal(t, ReasonUnschedulable, want.Reason)

			src.err = err
			for _, d := range []time.Duration{time.Minute, time.Hour, 72 * time.Hour} {
				tr.Refresh(src, t0.Add(d))
				assert.Equal(t, want, tr.Outcome(), "after %s", d)
			}
		})
	}
}

func TestCordonedThenDCAOutageThenUncordoned(t *testing.T) {
	tr := NewTracker(time.Minute)
	src := &fakeSource{result: Result{Unschedulable: true, Reasons: []string{"cordoned"}}}
	t0 := time.Now()
	tr.Refresh(src, t0)

	src.err = errors.New("connection refused")
	tr.Refresh(src, t0.Add(time.Hour))
	assert.Equal(t, ReasonUnschedulable, tr.Outcome().Reason, "the cordon is kept through the outage")

	src.err, src.result = nil, Result{Reasons: []string{}}
	tr.Refresh(src, t0.Add(2*time.Hour))
	assert.Equal(t, Outcome{}, tr.Outcome())
}

func TestUnsupportedClearsTheResult(t *testing.T) {
	tr := NewTracker(time.Minute)
	src := &fakeSource{result: Result{Unschedulable: true, Reasons: []string{"cordoned"}}}
	t0 := time.Now()
	tr.Refresh(src, t0)
	require.Equal(t, ReasonUnschedulable, tr.Outcome().Reason)

	// The cluster agent is disabled.
	src.err = ErrUnsupported
	tr.Refresh(src, t0.Add(time.Minute))
	assert.Equal(t, Outcome{}, tr.Outcome())
	assert.Zero(t, src.probeCalls, "no probe for an unusable source")
}

func TestRejectedRequestProbe(t *testing.T) {
	cordoned := Result{Unschedulable: true, Reasons: []string{"cordoned"}}

	t.Run("probe succeeds: old cluster agent, endpoint unsupported, result cleared", func(t *testing.T) {
		tr := NewTracker(time.Minute)
		src := &fakeSource{result: cordoned}
		t0 := time.Now()
		tr.Refresh(src, t0)
		require.Equal(t, ReasonUnschedulable, tr.Outcome().Reason)

		src.err = fmt.Errorf("%w: 404", ErrRejected)
		tr.Refresh(src, t0.Add(time.Minute))
		assert.Equal(t, 1, src.probeCalls)
		assert.Equal(t, Outcome{}, tr.Outcome())
	})

	t.Run("probe fails too: failed read, last result kept", func(t *testing.T) {
		tr := NewTracker(time.Minute)
		src := &fakeSource{result: cordoned}
		t0 := time.Now()
		tr.Refresh(src, t0)

		src.err = fmt.Errorf("%w: 403", ErrRejected)
		src.probeErr = errors.New("invalid session token")
		tr.Refresh(src, t0.Add(time.Minute))
		assert.Equal(t, 1, src.probeCalls)
		assert.Equal(t, ReasonUnschedulable, tr.Outcome().Reason)
	})

	t.Run("never read, rejected: nothing either way", func(t *testing.T) {
		tr := NewTracker(time.Minute)
		src := &fakeSource{err: fmt.Errorf("%w: 404", ErrRejected), probeErr: errors.New("boom")}
		tr.Refresh(src, time.Now())
		assert.Equal(t, Outcome{}, tr.Outcome())
	})

	t.Run("supported again after an upgrade", func(t *testing.T) {
		tr := NewTracker(time.Minute)
		src := &fakeSource{err: fmt.Errorf("%w: 404", ErrRejected)}
		t0 := time.Now()
		tr.Refresh(src, t0)
		require.Equal(t, Outcome{}, tr.Outcome())

		src.err, src.result = nil, cordoned
		tr.Refresh(src, t0.Add(time.Minute))
		assert.Equal(t, ReasonUnschedulable, tr.Outcome().Reason)
	})
}

func TestSeries(t *testing.T) {
	var s Series
	t0 := time.Now()
	drainTags := []string{"unschedulable_reason:drain"}

	points := s.Step(Outcome{Reason: ReasonUnschedulable, Tags: drainTags}, t0)
	require.Len(t, points, 1)
	assert.Equal(t, Point{Tags: []string{"unavailable_reason:unschedulable", "unschedulable_reason:drain"}, Value: 1}, points[0])

	// The set of matches changes: the old tagset is closed, the new one opened.
	points = s.Step(Outcome{Reason: ReasonUnschedulable, Tags: []string{"unschedulable_reason:drain", "unschedulable_reason:maintenance"}}, t0.Add(time.Minute))
	require.Len(t, points, 2)
	assert.Equal(t, float64(1), points[0].Value)
	assert.Equal(t, Point{Tags: []string{"unavailable_reason:unschedulable", "unschedulable_reason:drain"}, Value: 0}, points[1])

	// Cleared: the open tagset is closed too.
	points = s.Step(Outcome{}, t0.Add(2*time.Minute))
	require.Len(t, points, 2)
	for _, p := range points {
		assert.Equal(t, float64(0), p.Value)
	}

	// Back to the first tagset: its pending zeros are cancelled.
	points = s.Step(Outcome{Reason: ReasonUnschedulable, Tags: drainTags}, t0.Add(3*time.Minute))
	require.Len(t, points, 2)
	assert.Equal(t, Point{Tags: []string{"unavailable_reason:unschedulable", "unschedulable_reason:drain"}, Value: 1}, points[0])
	assert.Equal(t, float64(0), points[1].Value)

	// The closing window ends in wall time.
	points = s.Step(Outcome{Reason: ReasonUnschedulable, Tags: drainTags}, t0.Add(3*time.Minute).Add(ClosingWindow))
	assert.Len(t, points, 1)

	points = s.Step(Outcome{}, t0.Add(30*time.Minute))
	require.Len(t, points, 1)
	assert.Equal(t, float64(0), points[0].Value)
	assert.Empty(t, s.Step(Outcome{}, t0.Add(30*time.Minute).Add(ClosingWindow)))
}

// kubectl uncordon clears only the cordoned reason: the other reasons keep reporting
// and the old tagset is closed with zeros.
func TestUncordonClosesOnlyTheCordonedTagset(t *testing.T) {
	tr := NewTracker(time.Minute)
	src := &fakeSource{result: Result{Unschedulable: true, Reasons: []string{"cordoned", "drain"}}}
	var s Series
	t0 := time.Now()
	tr.Refresh(src, t0)
	s.Step(tr.Outcome(), t0)

	src.result = Result{Unschedulable: true, Reasons: []string{"drain"}} // kubectl uncordon
	tr.Refresh(src, t0.Add(time.Minute))
	points := s.Step(tr.Outcome(), t0.Add(time.Minute))
	require.Len(t, points, 2)
	assert.Equal(t, Point{Tags: []string{"unavailable_reason:unschedulable", "unschedulable_reason:drain"}, Value: 1}, points[0])
	assert.Equal(t, Point{Tags: []string{"unavailable_reason:unschedulable", "unschedulable_reason:cordoned", "unschedulable_reason:drain"}, Value: 0}, points[1])
}
