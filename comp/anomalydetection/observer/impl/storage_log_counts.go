// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"

// logCountHistory describes completed log-count buckets independently of their
// values. Only nonzero values occupy seriesStats.buckets. Keeping the eligible
// intervals preserves discovery, idle-TTL gaps, and re-anchoring on reactivation;
// an arbitrary missing timestamp must never be interpreted as a zero.
//
// It is allocated only for bucketized logs. Retention and the logical point cap
// trim both intervals and values, including when no nonzero value is appended.
type logCountHistory struct {
	width     int64
	intervals []logCountBucketInterval
	count     int
}

func (h *logCountHistory) append(timestamp int64) {
	n := len(h.intervals)
	if n > 0 && h.intervals[n-1].lastEnd+h.width == timestamp {
		h.intervals[n-1].lastEnd = timestamp
	} else {
		h.intervals = append(h.intervals, logCountBucketInterval{firstEnd: timestamp, lastEnd: timestamp})
	}
	h.count++
}

func (h *logCountHistory) pointsUpTo(timestamp int64) int {
	count := 0
	for _, interval := range h.intervals {
		if timestamp < interval.firstEnd {
			break
		}
		count += int((min(timestamp, interval.lastEnd)-interval.firstEnd)/h.width + 1)
		if timestamp < interval.lastEnd {
			break
		}
	}
	return count
}

func (h *logCountHistory) timestampAt(index int) int64 {
	for _, interval := range h.intervals {
		n := int((interval.lastEnd-interval.firstEnd)/h.width + 1)
		if index < n {
			return interval.firstEnd + int64(index)*h.width
		}
		index -= n
	}
	panic("log-count point index out of range")
}

// trimThrough drops logical buckets at or before timestamp. The interval slice
// reuses its bounded backing array, just like the ordinary point storage.
func (h *logCountHistory) trimThrough(timestamp int64) {
	removed := h.pointsUpTo(timestamp)
	remaining := removed
	i := 0
	for i < len(h.intervals) {
		n := int((h.intervals[i].lastEnd-h.intervals[i].firstEnd)/h.width + 1)
		if remaining < n {
			h.intervals[i].firstEnd += int64(remaining) * h.width
			break
		}
		remaining -= n
		i++
	}
	h.intervals = trimFront(h.intervals, i)
	h.count -= removed
}

// pointsUpTo and timestampAt address the logical series. The ordinary metrics
// path retains its binary-search/direct-index behavior.
func (s *seriesStats) pointsUpTo(timestamp int64) int {
	if s.logCounts != nil {
		return s.logCounts.pointsUpTo(timestamp)
	}
	return searchAfter(s.buckets, timestamp)
}

func (s *seriesStats) timestampAt(index int) int64 {
	if s.logCounts != nil {
		return s.logCounts.timestampAt(index)
	}
	return s.buckets[index].timestamp
}

func (s *seriesStats) appendLogCount(value float64, timestamp, width, retention int64, maxPoints int) {
	if s.logCounts == nil {
		s.logCounts = &logCountHistory{width: width}
		// A series with no empty intervals uses ordinary storage until its
		// first zero. Its retained nonzero timestamps supply the initial spans.
		for _, bucket := range s.buckets {
			s.logCounts.append(bucket.timestamp)
		}
	}
	s.logCounts.append(timestamp)
	if value != 0 {
		s.buckets = append(s.buckets, pointBucket{timestamp: timestamp, sum: value})
	}
	// These are the same inclusive retention boundary and pending-point allowance
	// as materialized storage. Apply them to logical points, not nonzero entries.
	cutoff := int64(-1 << 63)
	if retention > 0 {
		cutoff = timestamp - retention - 1
	}
	if maxPoints > 0 && s.logCounts.count > maxPoints+1 {
		cutoff = max(cutoff, s.logCounts.timestampAt(s.logCounts.count-maxPoints-2))
	}
	if s.logCounts.pointsUpTo(cutoff) > 0 {
		s.logCounts.trimThrough(cutoff)
		s.trimBuckets(searchAfter(s.buckets, cutoff))
		if len(s.buckets) == 0 {
			// Release a formerly busy series' numeric backing array once only
			// implicit zeros remain in its retained window.
			s.buckets = nil
		}
	}
	if len(s.buckets) == s.logCounts.count {
		// Once every retained bucket is nonzero again, the values themselves
		// fully describe the series. Avoid interval overhead for busy logs.
		s.logCounts = nil
	}
}

// snapshotLogCountPoints fills a bounded logical range in order. One walk over
// the intervals and nonzero values generates zeros directly into the caller's
// read buffer; no dense history is retained by the series.
func (s *seriesStats) snapshotLogCountPoints(lo, hi int, agg Aggregate, dst []observer.Point) {
	if lo == hi {
		return
	}
	physical := searchAfter(s.buckets, s.logCounts.timestampAt(lo)-1)
	index, out := 0, 0
	for _, interval := range s.logCounts.intervals {
		n := int((interval.lastEnd-interval.firstEnd)/s.logCounts.width + 1)
		begin, end := max(lo-index, 0), min(hi-index, n)
		for i := begin; i < end; i++ {
			timestamp := interval.firstEnd + int64(i)*s.logCounts.width
			value := 0.0
			if physical < len(s.buckets) && s.buckets[physical].timestamp == timestamp {
				value = s.buckets[physical].sum
				physical++
			}
			switch agg {
			case AggregateCount:
				// A materialized bucket is one sample, even when its log count is
				// zero. Count remains unsupported for detection on these series.
				value = 1
			case AggregateAverage, AggregateSum:
			default:
				value = 0
			}
			dst[out] = observer.Point{Timestamp: timestamp, Value: value}
			out++
		}
		index += n
		if index >= hi {
			return
		}
	}
}
