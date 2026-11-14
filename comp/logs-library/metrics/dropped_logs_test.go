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
	"unicode/utf8"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var primaryIntake = DropTarget{Host: "agent-http-intake.logs.datadoghq.com", Kind: DropTargetPrimary}

func newTestDroppedLogsTracker() (*lossTracker[droppedLogsKey], *clock.Mock) {
	clk := clock.NewMock()
	clk.Set(missedBytesBase)
	return newDroppedLogsTracker(clk), clk
}

func TestDroppedLogsRecordAndSnapshot(t *testing.T) {
	tr, clk := newTestDroppedLogsTracker()

	recordDroppedLogs(tr, DropReasonAuthRejected, primaryIntake, map[DropOrigin]DropCount{
		{Source: "nginx", Service: "web"}:   {Logs: 3, Bytes: 300},
		{Source: "redis", Service: "cache"}: {Logs: 1, Bytes: 50},
	})
	clk.Add(lossBucketSize)
	recordDroppedLogs(tr, DropReasonAuthRejected, primaryIntake, map[DropOrigin]DropCount{
		{Source: "nginx", Service: "web"}: {Logs: 2, Bytes: 200},
	})

	got := droppedLogsSnapshot(tr)
	require.Len(t, got, 2)

	// Sorted by source first, and summed across buckets.
	assert.Equal(t, "nginx", got[0].Source)
	assert.Equal(t, "web", got[0].Service)
	assert.Equal(t, DropReasonAuthRejected, got[0].Reason)
	assert.Equal(t, primaryIntake.Host, got[0].Destination)
	assert.Equal(t, DropTargetPrimary, got[0].Kind)
	assert.Equal(t, int64(5), got[0].Logs)
	assert.Equal(t, int64(500), got[0].Bytes)
	assert.Equal(t, clk.Now(), got[0].LastDropAt)

	assert.Equal(t, "redis", got[1].Source)
	assert.Equal(t, int64(1), got[1].Logs)
}

// The same origin dropped for two reasons or at two destinations needs two
// remediation branches, so it must stay two keys.
func TestDroppedLogsKeepsReasonAndDestinationApart(t *testing.T) {
	tr, _ := newTestDroppedLogsTracker()
	origin := map[DropOrigin]DropCount{{Source: "nginx", Service: "web"}: {Logs: 1, Bytes: 10}}

	recordDroppedLogs(tr, DropReasonAuthRejected, primaryIntake, origin)
	recordDroppedLogs(tr, DropReasonPayloadTooLarge, primaryIntake, origin)
	recordDroppedLogs(tr, DropReasonAuthRejected, DropTarget{Host: "intake.eu", Kind: DropTargetAdditional}, origin)

	assert.Len(t, droppedLogsSnapshot(tr), 3)
}

func TestDroppedLogsAgesOut(t *testing.T) {
	tr, clk := newTestDroppedLogsTracker()
	recordDroppedLogs(tr, DropReasonSendFailed, primaryIntake, map[DropOrigin]DropCount{
		{Source: "nginx", Service: "web"}: {Logs: 1, Bytes: 10},
	})
	require.Len(t, droppedLogsSnapshot(tr), 1)

	clk.Add(lossWindow + lossBucketSize)
	assert.Empty(t, droppedLogsSnapshot(tr))
	assert.Empty(t, tr.entries, "an aged-out key must be dropped from the map, not retained")
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
	extra := DropTarget{Host: "intake.eu", Kind: DropTargetAdditional}

	for i := 0; i < lossMaxKeys; i++ {
		recordDroppedLogs(tr, DropReasonAuthRejected, primaryIntake, map[DropOrigin]DropCount{
			{Source: fmt.Sprintf("source-%03d", i), Service: "svc"}: {Logs: 1, Bytes: 1},
		})
	}
	recordDroppedLogs(tr, DropReasonAuthRejected, primaryIntake, map[DropOrigin]DropCount{
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
		case DropReasonAuthRejected:
			auth = &s
		case DropReasonBufferFull:
			buffer = &s
		}
	}
	require.NotNil(t, auth)
	require.NotNil(t, buffer)
	assert.Equal(t, int64(5), auth.Logs)
	assert.Equal(t, primaryIntake.Host, auth.Destination)
	assert.Equal(t, int64(4), buffer.Logs)
	assert.Equal(t, DropTargetAdditional, buffer.Kind)
}

// Sustained churn with no reader: what runs when health_platform.enabled is false.
func TestDroppedLogsBoundedWithoutCollect(t *testing.T) {
	tr, clk := newTestDroppedLogsTracker()
	reasons := []DropReason{DropReasonAuthRejected, DropReasonPayloadTooLarge, DropReasonSendFailed}

	for hour := 0; hour < 3*lossMaxBuckets; hour++ {
		for i := 0; i < 50; i++ {
			recordDroppedLogs(tr, reasons[i%len(reasons)], primaryIntake, map[DropOrigin]DropCount{
				{Source: fmt.Sprintf("churn-%d-%d", hour, i), Service: "svc"}: {Logs: 1, Bytes: 1},
			})
		}
		clk.Add(lossBucketSize)

		require.LessOrEqual(t, len(tr.entries), lossMaxKeys+len(reasons), "hour %d", hour)
		for _, entry := range tr.entries {
			require.LessOrEqual(t, len(entry.buckets), lossMaxBuckets, "hour %d", hour)
		}
	}
}

func TestDroppedLogsBoundsNames(t *testing.T) {
	tr, _ := newTestDroppedLogsTracker()
	long := strings.Repeat("é", 4096)

	recordDroppedLogs(tr, DropReasonSendFailed, DropTarget{Host: strings.Repeat("h", 4096), Kind: DropTargetPrimary},
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
	assert.Len(t, got[0].Destination, droppedLogsMaxHostLen)
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
		recordDroppedLogs(tr, DropReasonAuthRejected, primaryIntake, drops)
	}
}
