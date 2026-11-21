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

var primaryIntake = DropTarget{Host: "agent-http-intake.logs.datadoghq.com", Main: true}

func newTestDroppedLogsTracker() (*lossTracker, *clock.Mock) {
	clk := clock.NewMock()
	clk.Set(missedBytesBase)
	return newDroppedLogsTracker(clk), clk
}

func TestDroppedLogsRecordAndSnapshot(t *testing.T) {
	tr, clk := newTestDroppedLogsTracker()
	start := clk.Now()

	recordDroppedLogs(tr, DropReasonAuth, primaryIntake, map[DropOrigin]DropCount{
		{Source: "nginx", Service: "web"}:   {Logs: 3, Bytes: 300},
		{Source: "redis", Service: "cache"}: {Logs: 1, Bytes: 50},
	})
	clk.Add(20 * time.Minute)
	recordDroppedLogs(tr, DropReasonAuth, primaryIntake, map[DropOrigin]DropCount{
		{Source: "nginx", Service: "web"}: {Logs: 2, Bytes: 200},
	})

	got := droppedLogsSnapshot(tr)
	require.Len(t, got, 2)

	// Largest first, and summed over the incident.
	assert.Equal(t, "nginx", got[0].Source)
	assert.Equal(t, "web", got[0].Service)
	assert.Equal(t, DropReasonAuth, got[0].Reason)
	assert.Equal(t, primaryIntake, got[0].Target)
	assert.Equal(t, int64(5), got[0].Logs)
	assert.Equal(t, int64(500), got[0].Bytes)
	assert.Equal(t, start, got[0].StartedAt)
	assert.Equal(t, clk.Now(), got[0].LastDropAt)

	assert.Equal(t, "redis", got[1].Source)
	assert.Equal(t, int64(1), got[1].Logs)
}

// The same origin dropped for two reasons or at two destinations needs two
// remediation branches, so it must stay two keys.
func TestDroppedLogsKeepsReasonAndDestinationApart(t *testing.T) {
	tr, _ := newTestDroppedLogsTracker()
	origin := map[DropOrigin]DropCount{{Source: "nginx", Service: "web"}: {Logs: 1, Bytes: 10}}

	recordDroppedLogs(tr, DropReasonAuth, primaryIntake, origin)
	recordDroppedLogs(tr, DropReasonTooLarge, primaryIntake, origin)
	recordDroppedLogs(tr, DropReasonAuth, DropTarget{Host: "intake.eu"}, origin)

	assert.Len(t, droppedLogsSnapshot(tr), 3)
}

// The main flag is part of the key: the same host can be the main endpoint and,
// through a misconfiguration, an additional one.
func TestDroppedLogsKeepsMainApart(t *testing.T) {
	tr, _ := newTestDroppedLogsTracker()
	origin := map[DropOrigin]DropCount{{Source: "nginx", Service: "web"}: {Logs: 1, Bytes: 10}}

	recordDroppedLogs(tr, DropReasonAuth, primaryIntake, origin)
	recordDroppedLogs(tr, DropReasonAuth, DropTarget{Host: primaryIntake.Host}, origin)

	assert.Len(t, droppedLogsSnapshot(tr), 2)
}

func TestDroppedLogsSnapshotLargestFirst(t *testing.T) {
	tr, _ := newTestDroppedLogsTracker()
	recordDroppedLogs(tr, DropReasonAuth, primaryIntake, map[DropOrigin]DropCount{
		{Source: "a", Service: "svc"}: {Logs: 1},
		{Source: "z", Service: "svc"}: {Logs: 9},
		{Source: "m", Service: "svc"}: {Logs: 9},
	})

	got := droppedLogsSnapshot(tr)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"m", "z", "a"}, []string{got[0].Source, got[1].Source, got[2].Source}, "ties break on source")
}

func TestDroppedLogsAgesOut(t *testing.T) {
	tr, clk := newTestDroppedLogsTracker()
	recordDroppedLogs(tr, DropReasonSendFailed, primaryIntake, map[DropOrigin]DropCount{
		{Source: "nginx", Service: "web"}: {Logs: 1, Bytes: 10},
	})
	clk.Add(lossQuietPeriod - time.Minute)
	require.Len(t, droppedLogsSnapshot(tr), 1, "still inside the quiet period")

	clk.Add(time.Minute)
	assert.Empty(t, droppedLogsSnapshot(tr), "an hour without a drop ends the incident")
	assert.Empty(t, tr.entries, "a quiet key must be dropped from the map, not retained")
}

// A drop after the quiet period starts a new incident even if nothing read the
// tracker in between: the old incident's totals must not carry over.
func TestDroppedLogsNewIncidentAfterQuiet(t *testing.T) {
	tr, clk := newTestDroppedLogsTracker()
	origin := func(n int64) map[DropOrigin]DropCount {
		return map[DropOrigin]DropCount{{Source: "nginx", Service: "web"}: {Logs: n, Bytes: n}}
	}

	recordDroppedLogs(tr, DropReasonAuth, primaryIntake, origin(5))
	clk.Add(lossQuietPeriod + time.Minute)
	restart := clk.Now()
	recordDroppedLogs(tr, DropReasonAuth, primaryIntake, origin(2))

	got := droppedLogsSnapshot(tr)
	require.Len(t, got, 1)
	assert.Equal(t, int64(2), got[0].Logs)
	assert.Equal(t, restart, got[0].StartedAt)
}

// Drops that never stop roll into a new incident every lossMaxIncident, so the
// totals stay on a one-day scale.
func TestDroppedLogsIncidentRollsOver(t *testing.T) {
	tr, clk := newTestDroppedLogsTracker()
	origin := map[DropOrigin]DropCount{{Source: "nginx", Service: "web"}: {Logs: 1, Bytes: 1}}

	// One drop every 30 minutes for 30 hours: never quiet.
	for i := 0; i < 60; i++ {
		recordDroppedLogs(tr, DropReasonAuth, primaryIntake, origin)
		clk.Add(30 * time.Minute)
	}

	got := droppedLogsSnapshot(tr)
	require.Len(t, got, 1)
	assert.LessOrEqual(t, got[0].Logs, int64(lossMaxIncident/(30*time.Minute)), "at most one day of drops")
	assert.WithinDuration(t, clk.Now(), got[0].StartedAt, lossMaxIncident)
}

func TestDroppedLogsIgnoresEmptyInput(t *testing.T) {
	tr, _ := newTestDroppedLogsTracker()
	recordDroppedLogs(tr, DropReasonSendFailed, primaryIntake, nil)
	recordDroppedLogs(tr, DropReasonSendFailed, primaryIntake, map[DropOrigin]DropCount{
		{Source: "nginx", Service: "web"}: {},
	})
	assert.Empty(t, tr.entries, "nothing dropped must not allocate an entry")
}

// Past the cap, source and service fold but reason and destination survive, so the
// remediation for the overflow is still right.
func TestDroppedLogsOverflowKeepsReasonAndDestination(t *testing.T) {
	tr, _ := newTestDroppedLogsTracker()
	extra := DropTarget{Host: "intake.eu"}

	for i := 0; i < lossMaxKeys; i++ {
		recordDroppedLogs(tr, DropReasonAuth, primaryIntake, map[DropOrigin]DropCount{
			{Source: fmt.Sprintf("source-%03d", i), Service: "svc"}: {Logs: 1, Bytes: 1},
		})
	}
	recordDroppedLogs(tr, DropReasonAuth, primaryIntake, map[DropOrigin]DropCount{
		{Source: "late-1", Service: "svc"}: {Logs: 2, Bytes: 20},
		{Source: "late-2", Service: "svc"}: {Logs: 3, Bytes: 30},
	})
	recordDroppedLogs(tr, DropReasonBufferFull, extra, map[DropOrigin]DropCount{
		{Source: "late-3", Service: "svc"}: {Logs: 4, Bytes: 40},
	})

	require.Len(t, tr.entries, lossMaxKeys+2, "one overflow key per (reason, destination)")

	var auth, buffer *DroppedLogsSummary
	for _, s := range droppedLogsSnapshot(tr) {
		if s.Source != lossOverflowLabel {
			continue
		}
		switch s.Reason {
		case DropReasonAuth:
			auth = &s
		case DropReasonBufferFull:
			buffer = &s
		}
	}
	require.NotNil(t, auth)
	require.NotNil(t, buffer)
	assert.Equal(t, int64(5), auth.Logs)
	assert.Equal(t, primaryIntake, auth.Target)
	assert.Equal(t, int64(4), buffer.Logs)
	assert.False(t, buffer.Target.Main)
}

// Sustained churn with no reader: what runs when health_platform.enabled is false.
func TestDroppedLogsBoundedWithoutCollect(t *testing.T) {
	tr, clk := newTestDroppedLogsTracker()
	reasons := []DropReason{DropReasonAuth, DropReasonTooLarge, DropReasonSendFailed}

	// 50 new keys every 10 minutes for three days.
	for tick := 0; tick < 3*24*6; tick++ {
		for i := 0; i < 50; i++ {
			recordDroppedLogs(tr, reasons[i%len(reasons)], primaryIntake, map[DropOrigin]DropCount{
				{Source: fmt.Sprintf("churn-%d-%d", tick, i), Service: "svc"}: {Logs: 1, Bytes: 1},
			})
		}
		clk.Add(10 * time.Minute)

		require.LessOrEqual(t, len(tr.entries), lossMaxKeys+len(reasons), "tick %d", tick)
	}
}

// At the cap, keys that went quiet make room before a new key folds into the
// overflow, so a tracker nobody reads still names the sources losing logs now.
func TestDroppedLogsEvictsQuietKeysAtCap(t *testing.T) {
	tr, clk := newTestDroppedLogsTracker()
	for i := 0; i < lossMaxKeys; i++ {
		recordDroppedLogs(tr, DropReasonAuth, primaryIntake, map[DropOrigin]DropCount{
			{Source: fmt.Sprintf("old-%03d", i), Service: "svc"}: {Logs: 1, Bytes: 1},
		})
	}
	clk.Add(lossQuietPeriod)

	recordDroppedLogs(tr, DropReasonAuth, primaryIntake, map[DropOrigin]DropCount{
		{Source: "new", Service: "svc"}: {Logs: 1, Bytes: 1},
	})

	got := droppedLogsSnapshot(tr)
	require.Len(t, got, 1)
	assert.Equal(t, "new", got[0].Source, "not folded into %q", lossOverflowLabel)
}

func TestDroppedLogsBoundsNames(t *testing.T) {
	tr, _ := newTestDroppedLogsTracker()
	long := strings.Repeat("é", 4096)

	recordDroppedLogs(tr, DropReasonSendFailed, DropTarget{Host: strings.Repeat("h", 4096), Main: true},
		map[DropOrigin]DropCount{
			{Source: long, Service: long + "a"}: {Logs: 1, Bytes: 1},
			{Source: long, Service: long + "b"}: {Logs: 2, Bytes: 2},
		})

	// Both services share the bounded prefix, so they fold and add up.
	got := droppedLogsSnapshot(tr)
	require.Len(t, got, 1)
	assert.Equal(t, int64(3), got[0].Logs)
	assert.Equal(t, lossMaxNameLen, utf8.RuneCountInString(got[0].Source))
	assert.True(t, utf8.ValidString(got[0].Service))
	assert.Len(t, got[0].Target.Host, droppedLogsMaxHostLen)
}

// Exists for -race.
func TestDroppedLogsConcurrentRecord(t *testing.T) {
	tr, _ := newTestDroppedLogsTracker()

	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				droppedLogsSnapshot(tr)
			}
		}
	}()

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				recordDroppedLogs(tr, DropReasonSendFailed, primaryIntake, map[DropOrigin]DropCount{
					{Source: fmt.Sprintf("source-%d", (g+i)%4), Service: "svc"}: {Logs: 1, Bytes: 3},
				})
			}
		}(g)
	}
	wg.Wait()
	close(stop)
	<-done

	require.Len(t, droppedLogsSnapshot(tr), 4)
}

// The 403-storm path: every payload of a fully rejected pipeline lands here.
func BenchmarkRecordDroppedLogs(b *testing.B) {
	tr, _ := newTestDroppedLogsTracker()
	drops := make(map[DropOrigin]DropCount, 8)
	for i := 0; i < 8; i++ {
		drops[DropOrigin{Source: fmt.Sprintf("source-%d", i), Service: "svc"}] = DropCount{Logs: 125, Bytes: 125 * 512}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recordDroppedLogs(tr, DropReasonAuth, primaryIntake, drops)
	}
}
