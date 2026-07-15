// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"
	"time"
)

const (
	// SaturationThreshold is the EWMA value above which a component is considered saturated.
	SaturationThreshold = 0.90

	fineTierCapacity     = 300 // 1s samples, 5min
	mediumBucketDuration = time.Minute
	mediumTierCapacity   = 25 // 1min buckets, 5m–30m
	coarseBucketDuration = time.Hour
	coarseTierCapacity   = 10 // 1h buckets, 30m–10h

	// currentSaturationWindow bounds how recent a saturated sample must be to count as saturated "now".
	currentSaturationWindow = 15 * time.Second
)

type fineSample struct {
	tsNano    int64
	ewmaValue float64
}

// aggregateBucket holds pre-aggregated stats for a time-aligned period (medium and coarse tiers).
type aggregateBucket struct {
	tsNano            int64 // period-aligned start
	ewmaSum           float64
	ewmaMax           float64
	count             int32
	saturatedCount    int32
	lastSaturatedNano int64
}

// WindowStats holds pre-computed statistics over several time windows. Ratios are fractions in [0,1].
type WindowStats struct {
	Avg5m            float64
	Max5m            float64
	Avg30m           float64
	Max30m           float64
	Max2h            float64
	Max5h            float64
	Max10h           float64
	Saturated1m      time.Duration
	Saturated30m     time.Duration
	LastSaturatedAt  time.Time
	HasLastSaturated bool
	// CurrentlySaturated is true if any sample within currentSaturationWindow was saturated (debounces flapping).
	CurrentlySaturated bool
}

// rollingHistory is a three-tier (1s/1m/1h) EWMA time-series; samples roll up tier to tier, bounding memory to ~6.2 KB.
type rollingHistory struct {
	mu sync.Mutex

	fine     [fineTierCapacity]fineSample
	fineHead int // next write slot; also the oldest slot once full
	fineSize int

	mediumPending    aggregateBucket
	hasMediumPending bool

	medium     [mediumTierCapacity]aggregateBucket
	mediumHead int
	mediumSize int

	coarse     [coarseTierCapacity]aggregateBucket
	coarseHead int
	coarseSize int
}

func newRollingHistory() *rollingHistory {
	return &rollingHistory{}
}

// add records a 1-second EWMA sample, rolling the oldest fine sample up when the ring is full.
func (h *rollingHistory) add(ts time.Time, ewmaValue float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.fineSize == fineTierCapacity {
		h.rollupFineToMedium(h.fine[h.fineHead], ts)
	}

	h.fine[h.fineHead] = fineSample{tsNano: ts.UnixNano(), ewmaValue: ewmaValue}
	h.fineHead = (h.fineHead + 1) % fineTierCapacity
	if h.fineSize < fineTierCapacity {
		h.fineSize++
	}
}

// rollupFineToMedium accumulates a fine sample into the current minute bucket, flushing on a minute boundary. Caller holds h.mu.
func (h *rollingHistory) rollupFineToMedium(s fineSample, now time.Time) {
	bucketStart := (s.tsNano / int64(mediumBucketDuration)) * int64(mediumBucketDuration)

	if h.hasMediumPending && h.mediumPending.tsNano == bucketStart {
		accumulateInto(&h.mediumPending, s)
		return
	}

	if h.hasMediumPending {
		h.pushMediumBucket(h.mediumPending, now)
	}
	h.mediumPending = aggregateBucket{tsNano: bucketStart}
	accumulateInto(&h.mediumPending, s)
	h.hasMediumPending = true
}

// pushMediumBucket appends a completed bucket to the medium ring, overflowing into the coarse tier. Caller holds h.mu.
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

// rollupMediumToCoarse merges a medium bucket into its hourly coarse bucket. Caller holds h.mu.
func (h *rollingHistory) rollupMediumToCoarse(b aggregateBucket, _ time.Time) {
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

	cCurrent := now.Add(-currentSaturationWindow).UnixNano()
	c1m := now.Add(-1 * time.Minute).UnixNano()
	c5m := now.Add(-5 * time.Minute).UnixNano()
	c30m := now.Add(-30 * time.Minute).UnixNano()
	c2h := now.Add(-2 * time.Hour).UnixNano()
	c5h := now.Add(-5 * time.Hour).UnixNano()
	c10h := now.Add(-10 * time.Hour).UnixNano()

	var (
		sum5m, sum30m      float64
		cnt5m, cnt30m      int
		max5m, max30m      float64
		sat1m, satFine     int
		lastSat            time.Time
		hasLastSat         bool
		currentlySaturated bool
	)

	// Fine tier. The ring evicts by count, not time, so break at c30m to drop stale idle samples.
	for i := 0; i < h.fineSize; i++ {
		idx := (h.fineHead - 1 - i + fineTierCapacity) % fineTierCapacity
		s := h.fine[idx]
		v := s.ewmaValue

		if s.tsNano < c30m {
			break
		}
		if s.tsNano >= cCurrent && v >= SaturationThreshold {
			currentlySaturated = true
		}
		if !hasLastSat && v >= SaturationThreshold {
			lastSat = time.Unix(0, s.tsNano)
			hasLastSat = true
		}
		sum30m += v
		cnt30m++
		if v > max30m {
			max30m = v
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

	// Medium pending: aged out of fine but not yet in the ring; include it so stats don't jump back ~1 minute.
	var satMedium int
	if h.hasMediumPending && h.mediumPending.count > 0 && h.mediumPending.tsNano >= c30m {
		satMedium += int(h.mediumPending.saturatedCount)
		sum30m += h.mediumPending.ewmaSum
		cnt30m += int(h.mediumPending.count)
		if h.mediumPending.ewmaMax > max30m {
			max30m = h.mediumPending.ewmaMax
		}
		if !hasLastSat && h.mediumPending.lastSaturatedNano > 0 {
			lastSat = time.Unix(0, h.mediumPending.lastSaturatedNano)
			hasLastSat = true
		}
	}

	// Medium tier (5m–30m, 1-minute buckets).
	for i := 0; i < h.mediumSize; i++ {
		idx := (h.mediumHead - 1 - i + mediumTierCapacity) % mediumTierCapacity
		b := h.medium[idx]

		// Compare against the bucket end so a bucket straddling c30m is still included.
		bucketEnd := b.tsNano + int64(mediumBucketDuration)
		if bucketEnd <= c30m {
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

	sat30m := satFine + satMedium

	var avg5m, avg30m float64
	if cnt5m > 0 {
		avg5m = sum5m / float64(cnt5m)
	}
	if cnt30m > 0 {
		avg30m = sum30m / float64(cnt30m)
	}

	// Coarse tier (30m–10h, 1-hour buckets). Seed long-window maxes from fine+medium.
	max2h, max5h, max10h := max30m, max30m, max30m

	for i := 0; i < h.coarseSize; i++ {
		idx := (h.coarseHead - 1 - i + coarseTierCapacity) % coarseTierCapacity
		b := h.coarse[idx]

		// Compare against the bucket end so a bucket straddling a cutoff is included.
		bucketEnd := b.tsNano + int64(coarseBucketDuration)

		if bucketEnd <= c10h {
			break
		}
		if b.ewmaMax > max10h {
			max10h = b.ewmaMax
		}
		if bucketEnd > c5h && b.ewmaMax > max5h {
			max5h = b.ewmaMax
		}
		if bucketEnd > c2h && b.ewmaMax > max2h {
			max2h = b.ewmaMax
		}
		if !hasLastSat && b.lastSaturatedNano > 0 {
			lastSat = time.Unix(0, b.lastSaturatedNano)
			hasLastSat = true
		}
	}

	return WindowStats{
		Avg5m:              avg5m,
		Max5m:              max5m,
		Avg30m:             avg30m,
		Max30m:             max30m,
		Max2h:              max2h,
		Max5h:              max5h,
		Max10h:             max10h,
		Saturated1m:        time.Duration(sat1m) * time.Second,
		Saturated30m:       time.Duration(sat30m) * time.Second,
		LastSaturatedAt:    lastSat,
		HasLastSaturated:   hasLastSat,
		CurrentlySaturated: currentlySaturated,
	}
}
