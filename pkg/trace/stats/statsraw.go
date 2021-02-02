// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/stats/quantile"
)

// Most "algorithm" stuff here is tested with stats_test.go as what is important
// is that the final data, the one with send after a call to Export(), is correct.

type groupedStats struct {
	topLevel float64

	hits                    float64
	errors                  float64
	duration                float64
	durationDistribution    *quantile.SliceSummary
	errDurationDistribution *quantile.SliceSummary
	sublayerStats           map[sublayerKey]sublayerStat
}

func (g *groupedStats) IsSublayersOnly() bool {
	return g.hits == 0 && g.durationDistribution.N == 0
}

type sublayerStat struct {
	topLevel float64
	value    int64
}

type sublayerKey struct {
	Metric string
	Tag    Tag
}

func newGroupedStats() *groupedStats {
	return &groupedStats{
		durationDistribution:    quantile.NewSliceSummary(),
		errDurationDistribution: quantile.NewSliceSummary(),
	}
}

type statsKey struct {
	name string
	aggr Aggregation
}

// RawBucket is used to compute span data and aggregate it
// within a time-framed bucket. This should not be used outside
// the agent, use Bucket for this.
type RawBucket struct {
	// This should really have no public fields. At all.

	start    int64 // timestamp of start in our format
	duration int64 // duration of a bucket in nanoseconds

	// this should really remain private as it's subject to refactoring
	data map[statsKey]*groupedStats

	// internal buffer for aggregate strings - not threadsafe
	keyBuf strings.Builder
}

// NewRawBucket opens a new calculation bucket for time ts and initializes it properly
func NewRawBucket(ts, d int64) *RawBucket {
	// The only non-initialized value is the Duration which should be set by whoever closes that bucket
	return &RawBucket{
		start:    ts,
		duration: d,
		data:     make(map[statsKey]*groupedStats),
	}
}

// Export transforms a RawBucket into a Bucket, typically used
// before communicating data to the API, as RawBucket is the internal
// type while Bucket is the public, shared one.
func (sb *RawBucket) Export() Bucket {
	ret := NewBucket(sb.start, sb.duration)
	for k, v := range sb.data {
		hitsKey := GrainKey(k.name, HITS, k.aggr)
		tagSet := k.aggr.ToTagSet()
		if !v.IsSublayersOnly() {
			ret.Counts[hitsKey] = Count{
				Key:      hitsKey,
				Name:     k.name,
				Measure:  HITS,
				TagSet:   tagSet,
				TopLevel: v.topLevel,
				Value:    float64(v.hits),
			}
			errorsKey := GrainKey(k.name, ERRORS, k.aggr)
			ret.Counts[errorsKey] = Count{
				Key:      errorsKey,
				Name:     k.name,
				Measure:  ERRORS,
				TagSet:   tagSet,
				TopLevel: v.topLevel,
				Value:    float64(v.errors),
			}
			durationKey := GrainKey(k.name, DURATION, k.aggr)
			ret.Counts[durationKey] = Count{
				Key:      durationKey,
				Name:     k.name,
				Measure:  DURATION,
				TagSet:   tagSet,
				TopLevel: v.topLevel,
				Value:    float64(v.duration),
			}
			ret.Distributions[durationKey] = Distribution{
				Key:      durationKey,
				Name:     k.name,
				Measure:  DURATION,
				TagSet:   tagSet,
				TopLevel: v.topLevel,
				Summary:  v.durationDistribution,
			}
			ret.ErrDistributions[durationKey] = Distribution{
				Key:      durationKey,
				Name:     k.name,
				Measure:  DURATION,
				TagSet:   tagSet,
				TopLevel: v.topLevel,
				Summary:  v.errDurationDistribution,
			}
		}
		for sk, sv := range v.sublayerStats {
			key := GrainKey(k.name, sk.Metric, k.aggr) + "," + sk.Tag.Name + ":" + sk.Tag.Value
			tagSet := append(k.aggr.ToTagSet(), sk.Tag)
			ret.Counts[key] = Count{
				Key:      key,
				Name:     k.name,
				Measure:  sk.Metric,
				TagSet:   tagSet,
				TopLevel: sv.topLevel,
				Value:    float64(sv.value),
			}
		}
	}
	return ret
}

// HandleSpan adds the span to this bucket stats, aggregated with the finest grain matching given aggregators
func (sb *RawBucket) HandleSpan(s *WeightedSpan, env string, sublayers []SublayerValue, skipStats bool) {
	if env == "" {
		panic("env should never be empty")
	}

	aggr := NewAggregationFromSpan(s.Span, env)
	sb.add(s, aggr, sublayers, skipStats)
}

func (sb *RawBucket) add(s *WeightedSpan, aggr Aggregation, sublayers []SublayerValue, skipStats bool) {
	var gs *groupedStats
	var ok bool

	key := statsKey{name: s.Name, aggr: aggr}
	if gs, ok = sb.data[key]; !ok {
		gs = newGroupedStats()
		sb.data[key] = gs
	}

	if !skipStats {
		if s.TopLevel {
			gs.topLevel += s.Weight
		}

		gs.hits += s.Weight
		if s.Error != 0 {
			gs.errors += s.Weight
		}
		gs.duration += float64(s.Duration) * s.Weight

		// TODO add for s.Metrics ability to define arbitrary counts and distros, check some config?
		// alter resolution of duration distro
		trundur := nsTimestampToFloat(s.Duration)
		gs.durationDistribution.Insert(trundur)

		if s.Error != 0 {
			gs.errDurationDistribution.Insert(trundur)
		}
	}

	for _, sub := range sublayers {
		var (
			ss sublayerStat
			ok bool
		)

		sKey := sublayerKey{sub.Metric, sub.Tag}
		if ss, ok = gs.sublayerStats[sKey]; !ok {
			if gs.sublayerStats == nil {
				// there are 3 types of sublayers
				gs.sublayerStats = make(map[sublayerKey]sublayerStat, 3)
			}
			ss = sublayerStat{}
		}

		if s.TopLevel {
			ss.topLevel += s.Weight
		}

		ss.value += int64(s.Weight * sub.Value)

		gs.sublayerStats[sKey] = ss
	}
}

// 10 bits precision (any value will be +/- 1/1024)
const roundMask int64 = 1 << 10

// nsTimestampToFloat converts a nanosec timestamp into a float nanosecond timestamp truncated to a fixed precision
func nsTimestampToFloat(ns int64) float64 {
	var shift uint
	for ns > roundMask {
		ns = ns >> 1
		shift++
	}
	return float64(ns << shift)
}
