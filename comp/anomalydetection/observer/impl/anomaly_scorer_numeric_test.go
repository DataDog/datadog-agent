// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/stretchr/testify/require"
)

// Frozen pre-numeric-key scorer core, used only for differential replay and
// before/after full-tick benchmarks. Keep its string identity and loops independent.
type stringScorerReference struct {
	config       AnomalyScorerConfig
	pending      map[int64][]observerdef.Anomaly
	windowMap    map[string]windowEntry
	topAnomalies *topAnomalyBuffer
	ewma         float64
	buckets      []observerdef.AnomalyScoreBucket
}

func newStringScorerReference(cfg AnomalyScorerConfig) *stringScorerReference {
	return &stringScorerReference{config: cfg, pending: make(map[int64][]observerdef.Anomaly), windowMap: make(map[string]windowEntry)}
}

func TestScorerNumericIdentity(t *testing.T) {
	// Same ref ignores descriptors; distinct refs and supported aggregations
	// remain distinct. Unknown aggregation values previously all meant "unknown".
	cases := []struct {
		name    string
		handles []observerdef.QueryHandle
		want    int
	}{
		{"same", []observerdef.QueryHandle{{Ref: 1}, {Ref: 1}}, 1},
		{"different refs", []observerdef.QueryHandle{{Ref: 1}, {Ref: 2}}, 2},
		{"aggregations", []observerdef.QueryHandle{{Ref: 1}, {Ref: 1, Aggregate: 1}, {Ref: 1, Aggregate: 2}, {Ref: 1, Aggregate: 3}}, 4},
		{"unknown", []observerdef.QueryHandle{{Ref: 1, Aggregate: -1}, {Ref: 1, Aggregate: 99}}, 1},
		{"zero and negative refs", []observerdef.QueryHandle{{Ref: 0}, {Ref: -1}}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultAnomalyScorerConfig()
			s := newAnomalyScorerBase(cfg)
			for i := range tc.handles {
				a := makeAnomaly("bocpd", 100, nil)
				a.SourceRef = &tc.handles[i]
				a.Source.Name = strconv.Itoa(i)
				s.pending[100] = append(s.pending[100], a)
			}
			s.advanceSecond(100)
			require.Equal(t, tc.want, s.buckets[0].Count)
			require.Nil(t, s.fallbackWindowMap)
			// A descriptor-only series is a separate identity, even with the same
			// human-readable source as a storage-backed anomaly.
			s.pending[101] = []observerdef.Anomaly{makeAnomaly("bocpd", 101, nil)}
			s.advanceSecond(101)
			require.Equal(t, tc.want+1, s.buckets[1].Count)
			s.advanceSecond(101 + cfg.WindowSecs)
			require.Empty(t, s.windowMap)
			require.Empty(t, s.fallbackWindowMap)
			// Reset must also release populated numeric and fallback identities.
			a := makeAnomaly("bocpd", 200, nil)
			a.SourceRef = &tc.handles[0]
			s.pending[200] = []observerdef.Anomaly{a, makeAnomaly("bocpd", 200, nil)}
			s.advanceSecond(200)
			require.NotEmpty(t, s.windowMap)
			require.NotEmpty(t, s.fallbackWindowMap)
			s.Reset()
			require.Empty(t, s.windowMap)
			require.Nil(t, s.fallbackWindowMap)
			require.Empty(t, s.buckets)
		})
	}
}

func TestScorerNumericLevelExpiry(t *testing.T) {
	cfg := DefaultAnomalyScorerConfig()
	cfg.WindowSecs = 3
	cfg.DetectorThresholds = map[string][4]float64{"test": {1, 2, 3, 4}}
	scorer := newAnomalyScorerBase(cfg)
	handle := observerdef.QueryHandle{Ref: 42, Aggregate: observerdef.AggregateAverage}
	for sec := int64(100); sec <= 104; sec++ {
		if sec <= 101 {
			a := makeAnomaly("test", sec, scorePtr(float64(4*(101-sec))))
			// Exercise both key paths, with independent but equivalent series.
			scorer.pending[sec] = []observerdef.Anomaly{a}
			a.SourceRef = &handle
			scorer.pending[sec] = append(scorer.pending[sec], a)
		}
		scorer.advanceSecond(sec)
	}
	// At 103 the old peak is gone but the low-level recurrence is still live.
	require.Equal(t, [5]int{2, 0, 0, 0, 0}, scorer.buckets[1].Bins)
	require.Equal(t, 0, scorer.buckets[2].Count)
}

func TestScorerNumericReplayParity(t *testing.T) {
	cfg := DefaultAnomalyScorerConfig()
	cfg.WindowSecs = 7
	cfg.DetectorThresholds = map[string][4]float64{"test": {1, 2, 3, 4}}
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprintf("mixed=%t", mixed), func(t *testing.T) {
			current, previous := newAnomalyScorerBase(cfg), newStringScorerReference(cfg)
			rng := rand.New(rand.NewSource(173))
			for sec := int64(100); sec < 400; sec++ {
				var anomalies []observerdef.Anomaly
				// Finish with silence to exercise complete expiration.
				if sec < 380 {
					for j := 0; j < rng.Intn(30); j++ {
						a := makeAnomaly("test", sec, scorePtr(float64(rng.Intn(5))))
						a.Source.Name = strconv.Itoa(rng.Intn(12))
						tags := []string{"env:test", "host:test"}
						if rng.Intn(2) == 0 {
							tags[0], tags[1] = tags[1], tags[0]
						}
						a.Source.Tags = testCompositeTags(tags)
						if !mixed || rng.Intn(3) != 0 {
							a.SourceRef = &observerdef.QueryHandle{Ref: observerdef.SeriesRef(rng.Intn(12) - 1), Aggregate: observerdef.Aggregate(rng.Intn(7) - 2)}
						}
						anomalies = append(anomalies, a)
					}
				}
				current.pending[sec], previous.pending[sec] = anomalies, anomalies
				current.advanceSecond(sec)
				previous.advanceSecond(sec)
				got, want := current.buckets[len(current.buckets)-1], previous.buckets[len(previous.buckets)-1]
				require.Equal(t, want.Bins, got.Bins, "second %d", sec)
				require.Equal(t, want.Count, got.Count, "second %d", sec)
				// Map iteration order was already unspecified. Floating sums may
				// differ in their last bits when identities change representation.
				require.InDelta(t, want.WeightSum, got.WeightSum, 1e-12)
				require.InDelta(t, want.Ewma, got.Ewma, 1e-12)
			}
			require.Empty(t, current.windowMap)
			require.Empty(t, current.fallbackWindowMap)
		})
	}
}

func BenchmarkScorerNumericKeys(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		anomalies := make([]observerdef.Anomaly, n)
		for i := range anomalies {
			anomalies[i] = observerdef.Anomaly{SourceRef: &observerdef.QueryHandle{Ref: observerdef.SeriesRef(100000 + i), Aggregate: observerdef.AggregateAverage}}
		}
		for _, numeric := range []bool{false, true} {
			b.Run(fmt.Sprintf("series=%d/numeric=%t", n, numeric), func(b *testing.B) {
				cfg := DefaultAnomalyScorerConfig()
				current, previous := newAnomalyScorerBase(cfg), newStringScorerReference(cfg)
				// Prewarm equally; each operation then advances all active series
				// one second, including merge, expiry, bins, EWMA and bucket retention.
				if numeric {
					current.pending[100] = anomalies
					current.advanceSecond(100)
				} else {
					previous.pending[100] = anomalies
					previous.advanceSecond(100)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					sec := int64(i + 101)
					if numeric {
						current.pending[sec] = anomalies
						current.advanceSecond(sec)
					} else {
						previous.pending[sec] = anomalies
						previous.advanceSecond(sec)
					}
				}
			})
		}
	}
}

func (s *stringScorerReference) advanceSecond(sec int64) float64 {
	anomalies := s.pending[sec]
	delete(s.pending, sec)

	if s.topAnomalies != nil {
		s.topAnomalies.update(sec, anomalies, s.config.AnomalyScorerConfig)
	}

	// Step 1: merge new anomalies into the window.
	for _, a := range anomalies {
		var sid string
		if a.SourceRef != nil {
			sid = a.SourceRef.CompactID()
		} else {
			sid = a.Source.Key()
		}
		level := anomalyLevel(a, s.config.AnomalyScorerConfig)
		entry := s.windowMap[sid]
		if sec > entry[level] {
			entry[level] = sec
		}
		s.windowMap[sid] = entry
	}

	// Step 2: evict per-level timestamps that have fallen out of the window,
	// and remove the series entirely when no level remains active.
	windowStart := sec - s.config.WindowSecs + 1
	for sid, entry := range s.windowMap {
		alive := false
		for level := 0; level < 5; level++ {
			if entry[level] > 0 && entry[level] < windowStart {
				entry[level] = 0
			}
			if entry[level] > 0 {
				alive = true
			}
		}
		if !alive {
			delete(s.windowMap, sid)
			continue
		}
		s.windowMap[sid] = entry
	}

	// Step 3: bucket from the live window.
	// Each series contributes at the highest level that still has an active timestamp.
	var bins [5]int
	var count int
	var weightSum float64
	for _, entry := range s.windowMap {
		for level := 4; level >= 0; level-- {
			if entry[level] == 0 {
				continue
			}
			bins[level]++
			count++
			weightSum += levelWeights[level]
			break
		}
	}

	// Step 4: saturated input → EWMA.
	var input float64
	if count > 0 {
		meanWeight := weightSum / float64(count)
		input = meanWeight * (1 - math.Exp(-float64(count)/s.config.SaturationK))
	}

	s.ewma = s.config.Alpha*input + (1-s.config.Alpha)*s.ewma

	s.buckets = append(s.buckets, observerdef.AnomalyScoreBucket{
		Second:    sec,
		Bins:      bins,
		Count:     count,
		WeightSum: weightSum,
		Ewma:      s.ewma,
	})
	// Default cap is WindowSecs; MaxBuckets overrides this when set to a positive value.
	bucketCap := s.config.MaxBuckets
	if bucketCap <= 0 {
		bucketCap = s.config.WindowSecs
	}
	if int64(len(s.buckets)) > bucketCap {
		trimmed := make([]observerdef.AnomalyScoreBucket, bucketCap)
		copy(trimmed, s.buckets[int64(len(s.buckets))-bucketCap:])
		s.buckets = trimmed
	}

	return s.ewma
}
