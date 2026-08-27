// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// missedBytesBase is 2024-01-01T00:00:00Z, a whole number of missedBytesBucketSize
// since the epoch, so records land on predictable bucket boundaries.
var missedBytesBase = time.Unix(1704067200, 0)

// newTestMissedBytesTracker returns a tracker driven by a mock clock parked at missedBytesBase.
func newTestMissedBytesTracker() (*missedBytesTracker, *clock.Mock) {
	clk := clock.NewMock()
	clk.Set(missedBytesBase)
	return newMissedBytesTracker(clk), clk
}

// findMissedBytes returns the summary for source:service from a slice, or fails the test.
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

// TestMissedBytes_SingleRecord checks one recorded loss surfaces with its bytes, rotation count and timestamp.
func TestMissedBytes_SingleRecord(t *testing.T) {
	tr, clk := newTestMissedBytesTracker()

	tr.record("nginx", "web", 4096)

	summaries := tr.snapshot()
	require.Len(t, summaries, 1)
	assert.Equal(t, "nginx", summaries[0].Source)
	assert.Equal(t, "web", summaries[0].Service)
	assert.Equal(t, int64(4096), summaries[0].Bytes)
	assert.Equal(t, int64(1), summaries[0].Rotations, "one record must count as one rotation")
	assert.Equal(t, clk.Now().UnixNano(), summaries[0].LastLossAt.UnixNano())
}

// TestMissedBytes_SameTupleAccumulates checks repeated losses for one tuple sum and count up.
func TestMissedBytes_SameTupleAccumulates(t *testing.T) {
	tr, clk := newTestMissedBytesTracker()

	tr.record("nginx", "web", 100)
	clk.Add(time.Minute)
	tr.record("nginx", "web", 250)
	clk.Add(time.Minute)
	tr.record("nginx", "web", 25)

	summaries := tr.snapshot()
	require.Len(t, summaries, 1, "one tuple must produce one summary regardless of rotation count")
	assert.Equal(t, int64(375), summaries[0].Bytes)
	assert.Equal(t, int64(3), summaries[0].Rotations)
	assert.Equal(t, clk.Now().UnixNano(), summaries[0].LastLossAt.UnixNano(),
		"LastLossAt must track the most recent loss")
}

// TestMissedBytes_DistinctTuplesSorted checks tuples stay separate and come back sorted by source then service.
func TestMissedBytes_DistinctTuplesSorted(t *testing.T) {
	tr, _ := newTestMissedBytesTracker()

	tr.record("nginx", "web", 10)
	tr.record("apache", "web", 20)
	tr.record("nginx", "api", 30)
	tr.record("apache", "api", 40)

	summaries := tr.snapshot()
	require.Len(t, summaries, 4)

	order := make([]string, 0, len(summaries))
	for _, s := range summaries {
		order = append(order, s.Source+":"+s.Service)
	}
	assert.Equal(t, []string{"apache:api", "apache:web", "nginx:api", "nginx:web"}, order,
		"summaries must be sorted by source then service")

	assert.Equal(t, int64(10), findMissedBytes(t, summaries, "nginx", "web").Bytes)
	assert.Equal(t, int64(20), findMissedBytes(t, summaries, "apache", "web").Bytes)
	assert.Equal(t, int64(30), findMissedBytes(t, summaries, "nginx", "api").Bytes)
	assert.Equal(t, int64(40), findMissedBytes(t, summaries, "apache", "api").Bytes)
}

// TestMissedBytes_WindowExpiry checks a loss stays visible inside the window and is dropped once it ages out.
func TestMissedBytes_WindowExpiry(t *testing.T) {
	tr, clk := newTestMissedBytesTracker()

	tr.record("nginx", "web", 512)

	clk.Add(missedBytesWindow - missedBytesBucketSize)
	summaries := tr.snapshot()
	require.Len(t, summaries, 1, "a loss still inside the trailing window must be reported")
	assert.Equal(t, int64(512), summaries[0].Bytes)

	// A bucket counts while any part of its slice overlaps the window, so it takes
	// one further bucket beyond missedBytesWindow to disappear entirely.
	clk.Add(2 * missedBytesBucketSize)
	assert.Empty(t, tr.snapshot(), "a loss older than the window must age out of the snapshot")
	assert.Empty(t, tr.entries, "an aged-out tuple must be dropped from the map, not retained")
}

// TestMissedBytes_SumsAcrossBuckets checks losses spread over several hours all count within one window.
func TestMissedBytes_SumsAcrossBuckets(t *testing.T) {
	tr, clk := newTestMissedBytesTracker()

	// Six losses 3h apart span 15h, comfortably inside the 24h window.
	for i := int64(1); i <= 6; i++ {
		tr.record("nginx", "web", i*100)
		if i < 6 {
			clk.Add(3 * missedBytesBucketSize)
		}
	}

	summaries := tr.snapshot()
	require.Len(t, summaries, 1)
	assert.Equal(t, int64(2100), summaries[0].Bytes, "every in-window bucket must contribute")
	assert.Equal(t, int64(6), summaries[0].Rotations)
	assert.Equal(t, clk.Now().UnixNano(), summaries[0].LastLossAt.UnixNano())
}

// TestMissedBytes_AgedOutBucketsPruned checks an expired bucket stops counting and
// is removed, while a fresh loss for the same tuple survives.
func TestMissedBytes_AgedOutBucketsPruned(t *testing.T) {
	tr, clk := newTestMissedBytesTracker()

	tr.record("nginx", "web", 900)

	clk.Add(missedBytesWindow + missedBytesBucketSize)
	tr.record("nginx", "web", 50)

	summaries := tr.snapshot()
	require.Len(t, summaries, 1)
	assert.Equal(t, int64(50), summaries[0].Bytes, "the aged-out bucket must not contribute")
	assert.Equal(t, int64(1), summaries[0].Rotations)
	assert.Len(t, tr.entries[missedBytesKey{source: "nginx", service: "web"}].buckets, 1,
		"the aged-out bucket must be deleted, not just skipped")
}

// TestMissedBytes_OverflowFoldsIntoSharedKey checks tuples past the cap merge into one summary and the map stops growing.
func TestMissedBytes_OverflowFoldsIntoSharedKey(t *testing.T) {
	tr, _ := newTestMissedBytesTracker()

	const overflow = 5
	for i := 0; i < missedBytesMaxKeys+overflow; i++ {
		tr.record(fmt.Sprintf("source-%03d", i), "svc", 10)
	}

	require.Len(t, tr.entries, missedBytesMaxKeys+1,
		"the map must hold at most the cap plus the shared overflow key")

	summaries := tr.snapshot()
	require.Len(t, summaries, missedBytesMaxKeys+1)

	other := findMissedBytes(t, summaries, missedBytesOverflowLabel, missedBytesOverflowLabel)
	assert.Equal(t, int64(overflow*10), other.Bytes, "every tuple past the cap must land in the overflow summary")
	assert.Equal(t, int64(overflow), other.Rotations)

	// The tuples recorded before the cap keep their own identity.
	assert.Equal(t, int64(10), findMissedBytes(t, summaries, "source-000", "svc").Bytes)

	for i := 0; i < 50; i++ {
		tr.record(fmt.Sprintf("late-%03d", i), "svc", 1)
	}
	assert.Len(t, tr.entries, missedBytesMaxKeys+1, "the map must not grow once the cap is reached")
	assert.Equal(t, int64(overflow*10+50), findMissedBytes(t, tr.snapshot(),
		missedBytesOverflowLabel, missedBytesOverflowLabel).Bytes)
}

// TestMissedBytes_NonPositiveBytesIgnored checks a rotation that lost nothing is not tracked.
func TestMissedBytes_NonPositiveBytesIgnored(t *testing.T) {
	tr, _ := newTestMissedBytesTracker()

	tr.record("nginx", "web", 0)
	tr.record("nginx", "web", -100)

	assert.Empty(t, tr.snapshot(), "a rotation that lost no bytes must not produce a summary")
	assert.Empty(t, tr.entries, "a rotation that lost no bytes must not allocate an entry")

	tr.record("nginx", "web", 200)
	tr.record("nginx", "web", 0)

	summaries := tr.snapshot()
	require.Len(t, summaries, 1)
	assert.Equal(t, int64(200), summaries[0].Bytes)
	assert.Equal(t, int64(1), summaries[0].Rotations, "a zero-byte rotation must not count as a rotation")
}

// TestMissedBytes_Reset checks reset clears tracked state and leaves the tracker usable.
func TestMissedBytes_Reset(t *testing.T) {
	tr, _ := newTestMissedBytesTracker()

	tr.record("nginx", "web", 100)
	tr.record("apache", "web", 100)
	require.Len(t, tr.snapshot(), 2)

	tr.reset()

	assert.Empty(t, tr.snapshot(), "reset must clear every tracked tuple")
	assert.Empty(t, tr.entries)

	tr.record("nginx", "web", 42)
	summaries := tr.snapshot()
	require.Len(t, summaries, 1, "the tracker must stay usable after reset")
	assert.Equal(t, int64(42), summaries[0].Bytes)
}

// TestMissedBytes_ConcurrentRecord checks concurrent writers on overlapping tuples lose no bytes.
func TestMissedBytes_ConcurrentRecord(t *testing.T) {
	// The mock clock never advances here, so nothing can expire mid-test and the
	// totals are exact.
	tr, _ := newTestMissedBytesTracker()

	const (
		goroutines     = 16
		perGoroutine   = 500
		tuples         = 4
		bytesPerRecord = 3
	)

	start := make(chan struct{})
	readersDone := make(chan struct{})

	var writers sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		writers.Add(1)
		go func(g int) {
			defer writers.Done()
			<-start
			for i := 0; i < perGoroutine; i++ {
				tr.record(fmt.Sprintf("source-%d", (g+i)%tuples), "svc", bytesPerRecord)
			}
		}(g)
	}

	// A reader racing the writers, so -race exercises snapshot against record.
	var reader sync.WaitGroup
	reader.Add(1)
	go func() {
		defer reader.Done()
		<-start
		for {
			select {
			case <-readersDone:
				return
			default:
				tr.snapshot()
			}
		}
	}()

	close(start)
	writers.Wait()
	close(readersDone)
	reader.Wait()

	var totalBytes, totalRotations int64
	summaries := tr.snapshot()
	for _, s := range summaries {
		totalBytes += s.Bytes
		totalRotations += s.Rotations
	}

	assert.Len(t, summaries, tuples, "overlapping writers must not create extra tuples")
	assert.Equal(t, int64(goroutines*perGoroutine*bytesPerRecord), totalBytes,
		"concurrent records must not lose bytes")
	assert.Equal(t, int64(goroutines*perGoroutine), totalRotations,
		"concurrent records must not lose rotations")
}

// TestMissedBytes_PackageLevelAPI checks the exported wrappers reach the process-wide tracker.
func TestMissedBytes_PackageLevelAPI(t *testing.T) {
	ResetMissedBytesForTest()
	defer ResetMissedBytesForTest()

	require.Empty(t, MissedBytesSnapshot())
	require.False(t, FileTailingActive())

	RecordMissedBytes("nginx", "web", 128)
	MarkFileTailingActive()

	summaries := MissedBytesSnapshot()
	require.Len(t, summaries, 1)
	assert.Equal(t, "nginx", summaries[0].Source)
	assert.Equal(t, "web", summaries[0].Service)
	assert.Equal(t, int64(128), summaries[0].Bytes)
	assert.True(t, FileTailingActive())
}
