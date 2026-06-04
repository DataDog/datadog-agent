// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes business developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"
	"time"
)

const (
	// SaturationThreshold is the short-EWMA value above which a component is considered saturated.
	SaturationThreshold = 0.90

	// Fine tier: 1-second resolution, last 5 minutes.
	fineTierCapacity = 300 // 5min × 60s

	// Medium tier: 1-minute aggregations, covers 5m–30m (25 buckets).
	mediumBucketDuration = time.Minute
	mediumTierCapacity   = 25

	// Coarse tier: 1-hour aggregations, covers 30m–10h (10 buckets).
	coarseBucketDuration = time.Hour
	coarseTierCapacity   = 10
)

// fineSample is one 1-second entry in the fine-resolution ring buffer.
type fineSample struct {
	tsNano    int64
	ewmaValue float64
}

// aggregateBucket holds pre-aggregated statistics for a time-aligned period.
// Used for both the medium (1-minute) and coarse (1-hour) tiers.
type aggregateBucket struct {
	tsNano            int64   // period-aligned start time (UnixNano)
	ewmaSum           float64 // sum of ewmaValues in this period
	ewmaMax           float64 // peak ewmaValue in this period
	count             int32   // number of samples aggregated
	saturatedCount    int32   // samples where ewmaValue >= SaturationThreshold
	lastSaturatedNano int64   // UnixNano of most recent saturated sample in this period
}

// WindowStats contains pre-computed statistics over several time windows.
// All ratio values are fractions in [0,1]; multiply by 100 for percentages.
type WindowStats struct {
	Avg5m  float64
	Max5m  float64
	Avg30m float64
	Max30m float64
	Max2h  float64
	Max5h  float64
	Max10h float64
	// Saturated1m is total time within the last 1 minute where short-EWMA >= SaturationThreshold.
	Saturated1m time.Duration
	// Saturated30m is total time within the last 30 minutes where short-EWMA >= SaturationThreshold.
	Saturated30m     time.Duration
	LastSaturatedAt  time.Time
	HasLastSaturated bool
}

// rollingHistory is a three-tier time-series store for component short-EWMA values.
//
//   Fine tier   — 1-second samples, last 5 minutes        (~4.8 KB)
//   Medium tier — 1-minute aggregates, 5m–30m (25 buckets) (~1.0 KB)
//   Coarse tier — 1-hour aggregates, 30m–10h (10 buckets)  (~0.4 KB)
//
// Total memory per component: ~6.2 KB, down from ~864 KB with the previous
// 10-hour per-second raw-sample ring buffer.
//
// add() is called at the existing 1-second rate. When fine samples age out of
// the 5-minute fine window they are accumulated into the medium tier; when
// medium buckets age out of the 30-minute window they roll into the coarse tier.
type rollingHistory struct {
	mu sync.Mutex

	// Fine ring buffer: 1-second resolution, last 5 minutes.
	fine     [fineTierCapacity]fineSample
	fineHead int // next write slot; also oldest slot when fineSize == fineTierCapacity
	fineSize int

	// In-progress accumulator for the current 1-minute medium bucket.
	mediumPending    aggregateBucket
	hasMediumPending bool

	// Medium ring buffer: completed 1-minute buckets, 5m–30m.
	medium     [mediumTierCapacity]aggregateBucket
	mediumHead int
	mediumSize int

	// Coarse ring buffer: completed 1-hour buckets, 30m–10h.
	coarse     [coarseTierCapacity]aggregateBucket
	coarseHead int
	coarseSize int
}

func newRollingHistory() *rollingHistory {
	return &rollingHistory{}
}

// add records a new 1-second short-EWMA sample. Called at 1-second intervals.
// Samples that overflow the fine window are automatically rolled up into the
// medium tier; medium buckets that overflow 30 minutes roll into coarse.
func (h *rollingHistory) add(ts time.Time, ewmaValue float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// When fine buffer is full, roll the oldest sample into the medium tier
	// before overwriting it.
	if h.fineSize == fineTierCapacity {
		h.rollupFineToMedium(h.fine[h.fineHead], ts)
	}

	h.fine[h.fineHead] = fineSample{tsNano: ts.UnixNano(), ewmaValue: ewmaValue}
	h.fineHead = (h.fineHead + 1) % fineTierCapacity
	if h.fineSize < fineTierCapacity {
		h.fineSize++
	}
}

// rollupFineToMedium accumulates a fine sample into the current 1-minute medium
// bucket. When the sample crosses into a new minute the completed bucket is
// pushed into the medium ring buffer. Must be called with h.mu held.
func (h *rollingHistory) rollupFineToMedium(s fineSample, now time.Time) {
	bucketStart := (s.tsNano / int64(mediumBucketDuration)) * int64(mediumBucketDuration)

	if h.hasMediumPending && h.mediumPending.tsNano == bucketStart {
		accumulateInto(&h.mediumPending, s)
		return
	}

	// Minute boundary crossed: flush the completed pending bucket.
	if h.hasMediumPending {
		h.pushMediumBucket(h.mediumPending, now)
	}
	h.mediumPending = aggregateBucket{tsNano: bucketStart}
	accumulateInto(&h.mediumPending, s)
	h.hasMediumPending = true
}

// pushMediumBucket writes a completed medium bucket into the medium ring buffer.
// If the medium buffer is full the overflowing bucket rolls into the coarse tier.
// Must be called with h.mu held.
func (h *rollingHistory) pushMediumBucket(b aggregateBucket, now time.Time) {
	if h.mediumSize == mediumTierCapacity {
		h.rollupMediumToCoarse(h.medium[h.mediumHead], now)
	}
	h.medium[h.mediumHead] = b
	h.mediumHead = (h.mediumHead + 1) % mediumTierCapacity
	if h.mediumSize < mediumTierCapacity {
		h.mediumSize++
	}
}

// rollupMediumToCoarse accumulates a medium bucket into the appropriate hourly
// coarse bucket. Must be called with h.mu held.
func (h *rollingHistory) rollupMediumToCoarse(b aggregateBucket, now time.Time) {
	bucketStart := (b.tsNano / int64(coarseBucketDuration)) * int64(coarseBucketDuration)

	if h.coarseSize > 0 {
		last := &h.coarse[(h.coarseHead-1+coarseTierCapacity)%coarseTierCapacity]
		if last.tsNano == bucketStart {
			mergeInto(last, b)
			return
		}
	}

	h.coarse[h.coarseHead] = aggregateBucket{
		tsNano:            bucketStart,
		ewmaSum:           b.ewmaSum,
		ewmaMax:           b.ewmaMax,
		count:             b.count,
		saturatedCount:    b.saturatedCount,
		lastSaturatedNano: b.lastSaturatedNano,
	}
	h.coarseHead = (h.coarseHead + 1) % coarseTierCapacity
	if h.coarseSize < coarseTierCapacity {
		h.coarseSize++
	}
}

// accumulateInto adds a single fine sample into an aggregate bucket.
func accumulateInto(b *aggregateBucket, s fineSample) {
	b.ewmaSum += s.ewmaValue
	b.count++
	if s.ewmaValue > b.ewmaMax {
		b.ewmaMax = s.ewmaValue
	}
	if s.ewmaValue >= SaturationThreshold {
		b.saturatedCount++
		b.lastSaturatedNano = s.tsNano
	}
}

// mergeInto merges a completed medium bucket into an existing coarse bucket.
func mergeInto(dst *aggregateBucket, src aggregateBucket) {
	dst.ewmaSum += src.ewmaSum
	dst.count += src.count
	if src.ewmaMax > dst.ewmaMax {
		dst.ewmaMax = src.ewmaMax
	}
	dst.saturatedCount += src.saturatedCount
	if src.lastSaturatedNano > dst.lastSaturatedNano {
		dst.lastSaturatedNano = src.lastSaturatedNano
	}
}

// allStats computes all window statistics in a single pass over all three tiers.
func (h *rollingHistory) allStats(now time.Time) WindowStats {
	h.mu.Lock()
	defer h.mu.Unlock()

	c1m := now.Add(-1 * time.Minute).UnixNano()
	c5m := now.Add(-5 * time.Minute).UnixNano()
	c30m := now.Add(-30 * time.Minute).UnixNano()
	c2h := now.Add(-2 * time.Hour).UnixNano()
	c5h := now.Add(-5 * time.Hour).UnixNano()

	var (
		sum5m, sum30m        float64
		cnt5m, cnt30m        int
		max5m, max30m        float64
		sat1m, satFine       int
		lastSat              time.Time
		hasLastSat           bool
	)

	// --- Fine tier (last 5 minutes, 1-second resolution) ---
	for i := 0; i < h.fineSize; i++ {
		idx := (h.fineHead - 1 - i + fineTierCapacity) % fineTierCapacity
		s := h.fine[idx]
		v := s.ewmaValue

		if !hasLastSat && v >= SaturationThreshold {
			lastSat = time.Unix(0, s.tsNano)
			hasLastSat = true
		}
		if s.tsNano >= c5m {
			sum5m += v
			cnt5m++
			if v > max5m {
				max5m = v
			}
		}
		if v >= SaturationThreshold {
			satFine++
			if s.tsNano >= c1m {
				sat1m++
			}
		}
	}

	// --- Medium tier (5m–30m, 1-minute buckets) ---
	var satMedium int
	for i := 0; i < h.mediumSize; i++ {
		idx := (h.mediumHead - 1 - i + mediumTierCapacity) % mediumTierCapacity
		b := h.medium[idx]

		if b.tsNano < c30m {
			break
		}
		if b.count > 0 {
			sum30m += b.ewmaSum
			cnt30m += int(b.count)
			if b.ewmaMax > max30m {
				max30m = b.ewmaMax
			}
			satMedium += int(b.saturatedCount)
		}
		if !hasLastSat && b.lastSaturatedNano > 0 {
			lastSat = time.Unix(0, b.lastSaturatedNano)
			hasLastSat = true
		}
	}

	// Combine fine + medium for 30m totals.
	sum30m += sum5m
	cnt30m += cnt5m
	if max5m > max30m {
		max30m = max5m
	}
	sat30m := satFine + satMedium

	var avg5m, avg30m float64
	if cnt5m > 0 {
		avg5m = sum5m / float64(cnt5m)
	}
	if cnt30m > 0 {
		avg30m = sum30m / float64(cnt30m)
	}

	// --- Coarse tier (30m–10h, 1-hour buckets) ---
	// Seed long-window maxes from what we already know from fine+medium.
	max2h, max5h, max10h := max30m, max30m, max30m

	for i := 0; i < h.coarseSize; i++ {
		idx := (h.coarseHead - 1 - i + coarseTierCapacity) % coarseTierCapacity
		b := h.coarse[idx]

		if b.ewmaMax > max10h {
			max10h = b.ewmaMax
		}
		if b.tsNano >= c5h && b.ewmaMax > max5h {
			max5h = b.ewmaMax
		}
		if b.tsNano >= c2h && b.ewmaMax > max2h {
			max2h = b.ewmaMax
		}
		if !hasLastSat && b.lastSaturatedNano > 0 {
			lastSat = time.Unix(0, b.lastSaturatedNano)
			hasLastSat = true
		}
	}

	return WindowStats{
		Avg5m:            avg5m,
		Max5m:            max5m,
		Avg30m:           avg30m,
		Max30m:           max30m,
		Max2h:            max2h,
		Max5h:            max5h,
		Max10h:           max10h,
		Saturated1m:      time.Duration(sat1m) * time.Second,
		Saturated30m:     time.Duration(sat30m) * time.Second,
		LastSaturatedAt:  lastSat,
		HasLastSaturated: hasLastSat,
	}
}
