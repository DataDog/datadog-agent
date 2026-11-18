// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A fixed start so incident times are predictable.
var missedBytesBase = time.Unix(1704067200, 0)

func newTestMissedBytesTracker() (*missedBytesTracker, *clock.Mock) {
	clk := clock.NewMock()
	clk.Set(missedBytesBase)
	return newMissedBytesTracker(clk), clk
}

func summarize(summaries []MissedBytesSummary) []string {
	out := make([]string, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, fmt.Sprintf("%s:%s %d/%d", s.Source, s.Service, s.Bytes, s.Rotations))
	}
	return out
}

func findMissedBytes(t *testing.T, summaries []MissedBytesSummary, source, service string) MissedBytesSummary {
	t.Helper()
	for _, s := range summaries {
		if s.Source == source && s.Service == service {
			return s
		}
	}
	t.Fatalf("no summary for %s:%s", source, service)
	return MissedBytesSummary{}
}

func TestMissedBytesRecord(t *testing.T) {
	tests := []struct {
		name   string
		record func(tr *missedBytesTracker, clk *clock.Mock)
		want   []string
	}{
		{
			name:   "one loss carries its bytes and rotation count",
			record: func(tr *missedBytesTracker, _ *clock.Mock) { tr.record("nginx", "web", 4096) },
			want:   []string{"nginx:web 4096/1"},
		},
		{
			name: "repeated losses for one tuple accumulate",
			record: func(tr *missedBytesTracker, clk *clock.Mock) {
				for _, n := range []int64{100, 250, 25} {
					tr.record("nginx", "web", n)
					clk.Add(time.Minute)
				}
			},
			want: []string{"nginx:web 375/3"},
		},
		{
			name: "distinct tuples stay separate, sorted by source then service",
			record: func(tr *missedBytesTracker, _ *clock.Mock) {
				tr.record("nginx", "web", 10)
				tr.record("apache", "web", 20)
				tr.record("nginx", "api", 30)
				tr.record("apache", "api", 40)
			},
			want: []string{"apache:api 40/1", "apache:web 20/1", "nginx:api 30/1", "nginx:web 10/1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr, clk := newTestMissedBytesTracker()
			tc.record(tr, clk)
			assert.Equal(t, tc.want, summarize(tr.collectAndPrune()))
		})
	}
}

func TestMissedBytesLastLossAt(t *testing.T) {
	tr, clk := newTestMissedBytesTracker()

	start := clk.Now()
	tr.record("nginx", "web", 100)
	clk.Add(time.Minute)
	tr.record("nginx", "web", 100)

	summaries := tr.collectAndPrune()
	require.Len(t, summaries, 1)
	assert.Equal(t, start.UnixNano(), summaries[0].StartedAt.UnixNano())
	assert.Equal(t, clk.Now().UnixNano(), summaries[0].LastLossAt.UnixNano())
}

// An hour without a new loss ends the incident, which is what resolves the issue.
func TestMissedBytesQuietPeriodEndsIncident(t *testing.T) {
	tr, clk := newTestMissedBytesTracker()

	tr.record("nginx", "web", 512)

	clk.Add(lossQuietPeriod - time.Minute)
	require.Equal(t, []string{"nginx:web 512/1"}, summarize(tr.collectAndPrune()))

	clk.Add(time.Minute)
	assert.Empty(t, tr.collectAndPrune())
	assert.Empty(t, tr.entries, "a quiet tuple must be dropped from the map, not retained")
}

// A loss after the quiet period starts a new incident; the old one's totals don't
// carry over even if nothing read the tracker in between.
func TestMissedBytesNewIncidentAfterQuiet(t *testing.T) {
	tr, clk := newTestMissedBytesTracker()

	tr.record("nginx", "web", 512)
	clk.Add(lossQuietPeriod + time.Minute)
	tr.record("nginx", "web", 64)

	assert.Equal(t, []string{"nginx:web 64/1"}, summarize(tr.collectAndPrune()))
}

// collectAndPrune only runs when the health check is scheduled, so record has to
// hold the line alone when health_platform.enabled is false.
func TestMissedBytesBoundedWithoutCollect(t *testing.T) {
	tr, clk := newTestMissedBytesTracker()

	// Ten days of rotations every 30 minutes: one steady tuple plus a fresh one each
	// time to push against the key cap. collectAndPrune is never called in the loop.
	for tick := 0; tick < 10*48; tick++ {
		tr.record("nginx", "web", 1)
		tr.record(fmt.Sprintf("churn-%03d", tick), "svc", 1)
		clk.Add(30 * time.Minute)

		require.LessOrEqual(t, len(tr.entries), missedBytesMaxKeys+1,
			"tracked tuples must stay capped at tick %d", tick)
	}

	// The steady tuple never went quiet, so it rolled over and holds at most a day.
	steady := findMissedBytes(t, tr.collectAndPrune(), "nginx", "web")
	assert.LessOrEqual(t, steady.Rotations, int64(lossMaxIncident/(30*time.Minute)))
	assert.WithinDuration(t, clk.Now(), steady.StartedAt, lossMaxIncident)
}

func TestMissedBytesOverflowFoldsIntoSharedKey(t *testing.T) {
	tr, _ := newTestMissedBytesTracker()

	const overflow = 5
	for i := 0; i < missedBytesMaxKeys+overflow; i++ {
		tr.record(fmt.Sprintf("source-%03d", i), "svc", 10)
	}

	require.Len(t, tr.entries, missedBytesMaxKeys+1,
		"the map must hold at most the cap plus the shared overflow key")

	summaries := tr.collectAndPrune()
	other := findMissedBytes(t, summaries, missedBytesOverflowLabel, missedBytesOverflowLabel)
	assert.Equal(t, int64(overflow*10), other.Bytes, "every tuple past the cap must land in the overflow summary")
	assert.Equal(t, int64(overflow), other.Rotations)
}

// Names are raw config strings, so the entry cap alone does not bound memory.
func TestMissedBytesBoundsKeyLength(t *testing.T) {
	tr, _ := newTestMissedBytesTracker()

	long := strings.Repeat("a", 4096)
	tr.record(long, strings.Repeat("é", 4096), 10)

	require.Len(t, tr.entries, 1)
	for key := range tr.entries {
		assert.Equal(t, missedBytesMaxNameLen, utf8.RuneCountInString(key.source))
		assert.Equal(t, missedBytesMaxNameLen, utf8.RuneCountInString(key.service))
		assert.True(t, utf8.ValidString(key.service), "a multi-byte name must not be cut mid-rune")
	}
}

func TestMissedBytesNonPositiveIgnored(t *testing.T) {
	tr, _ := newTestMissedBytesTracker()

	tr.record("nginx", "web", 0)
	tr.record("nginx", "web", -100)

	assert.Empty(t, tr.collectAndPrune())
	assert.Empty(t, tr.entries, "a rotation that lost no bytes must not allocate an entry")

	tr.record("nginx", "web", 200)
	tr.record("nginx", "web", 0)

	assert.Equal(t, []string{"nginx:web 200/1"}, summarize(tr.collectAndPrune()),
		"a zero-byte rotation must not count as a rotation")
}

// Exists for -race: nothing else exercises collectAndPrune against a concurrent
// record. Totals are left unasserted, being forced by the unconditional mutex.
func TestMissedBytesConcurrentRecord(t *testing.T) {
	tr, _ := newTestMissedBytesTracker()

	stopReader, readerDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stopReader:
				return
			default:
				tr.collectAndPrune()
			}
		}
	}()

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				tr.record(fmt.Sprintf("source-%d", (g+i)%4), "svc", 3)
			}
		}(g)
	}
	wg.Wait()
	close(stopReader)
	<-readerDone

	require.Len(t, tr.collectAndPrune(), 4)
}
