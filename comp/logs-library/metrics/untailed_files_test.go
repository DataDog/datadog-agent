// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestUntailedFilesTracker() (*untailedFilesTracker, *clock.Mock) {
	clk := clock.NewMock()
	clk.Set(missedBytesBase)
	return newUntailedFilesTracker(clk), clk
}

func TestUntailedFilesRecordAndCurrent(t *testing.T) {
	tr, clk := newTestUntailedFilesTracker()

	tr.record(500, 500, []UntailedFilesSource{
		{Source: "nginx", Service: "web", Path: "/var/log/nginx/*.log", Matched: 320, Untailed: 300},
		{Source: "redis", Service: "cache", Path: "/var/log/redis/*.log", Matched: 60, Untailed: 40},
		{Source: "app", Service: "app", Path: "/var/log/app.log", Matched: 1, Untailed: 0},
	})

	got, ok := tr.current()
	require.True(t, ok)
	assert.Equal(t, 500, got.Limit)
	assert.Equal(t, 500, got.Tailed)
	assert.Equal(t, 340, got.Untailed)
	require.Len(t, got.Sources, 2, "a source that lost nothing is not listed")
	assert.Equal(t, "nginx", got.Sources[0].Source, "largest first")
	assert.Equal(t, clk.Now(), got.LastSeenAt)
}

func TestUntailedFilesCleanScanRecordsNothing(t *testing.T) {
	tr, _ := newTestUntailedFilesTracker()
	tr.record(500, 10, nil)
	tr.record(500, 10, []UntailedFilesSource{{Source: "nginx", Matched: 10}})

	_, ok := tr.current()
	assert.False(t, ok)
}

// A clean scan does not clear the state: the hour of hold-down is what stops the
// issue flapping when wildcards hover around the limit.
func TestUntailedFilesHoldsForTheWindow(t *testing.T) {
	tr, clk := newTestUntailedFilesTracker()
	tr.record(2, 2, []UntailedFilesSource{{Source: "nginx", Matched: 5, Untailed: 3}})

	clk.Add(untailedFilesWindow - time.Minute)
	tr.record(2, 1, nil)
	_, ok := tr.current()
	assert.True(t, ok, "still inside the window")

	clk.Add(2 * time.Minute)
	_, ok = tr.current()
	assert.False(t, ok, "an hour after the last hit, the state ages out")
	assert.Nil(t, tr.state)
}

func TestUntailedFilesLatestHitWins(t *testing.T) {
	tr, clk := newTestUntailedFilesTracker()
	tr.record(2, 2, []UntailedFilesSource{{Source: "nginx", Matched: 5, Untailed: 3}})
	clk.Add(time.Minute)
	tr.record(4, 4, []UntailedFilesSource{{Source: "redis", Matched: 5, Untailed: 1}})

	got, ok := tr.current()
	require.True(t, ok)
	assert.Equal(t, 4, got.Limit)
	assert.Equal(t, "redis", got.Sources[0].Source)
}

func TestUntailedFilesBounded(t *testing.T) {
	tr, _ := newTestUntailedFilesTracker()
	sources := make([]UntailedFilesSource, 0, lossMaxKeys+7)
	for i := 0; i < lossMaxKeys+7; i++ {
		sources = append(sources, UntailedFilesSource{
			Source:   fmt.Sprintf("source-%03d", i) + strings.Repeat("x", 500),
			Path:     strings.Repeat("/p", 500),
			Matched:  2,
			Untailed: 1,
		})
	}
	tr.record(10, 10, sources)

	got, ok := tr.current()
	require.True(t, ok)
	assert.Len(t, got.Sources, lossMaxKeys)
	assert.Equal(t, 7, got.SourcesOmitted)
	assert.Equal(t, lossMaxKeys+7, got.Untailed, "totals cover every source, listed or not")
	assert.Len(t, []rune(got.Sources[0].Source), lossMaxNameLen)
	assert.Len(t, got.Sources[0].Path, untailedFilesMaxPathLen)
}

// The state handed out is a copy: a later record must not change it.
func TestUntailedFilesCurrentReturnsCopy(t *testing.T) {
	tr, _ := newTestUntailedFilesTracker()
	tr.record(2, 2, []UntailedFilesSource{{Source: "nginx", Matched: 5, Untailed: 3}})
	got, _ := tr.current()
	got.Sources[0].Source = "mutated"

	again, _ := tr.current()
	assert.Equal(t, "nginx", again.Sources[0].Source)
}

// The kept list must not share the caller-sized backing array it was cut from.
func TestUntailedFilesCapDoesNotRetainOmitted(t *testing.T) {
	tr, _ := newTestUntailedFilesTracker()
	sources := make([]UntailedFilesSource, lossMaxKeys*3)
	for i := range sources {
		sources[i] = UntailedFilesSource{Source: fmt.Sprintf("s-%04d", i), Matched: 1, Untailed: 1}
	}
	tr.record(1, 1, sources)
	assert.Equal(t, lossMaxKeys, cap(tr.state.Sources))
}
