// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package stats

import (
	"bytes"

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
}

type sublayerStats struct {
	topLevel float64

	value int64
}

func newGroupedStats() *groupedStats {
	return &groupedStats{
		durationDistribution:    quantile.NewSliceSummary(),
		errDurationDistribution: quantile.NewSliceSummary(),
	}
}

type statsKey struct {
	name string
	aggr aggregation
}

type statsSubKey struct {
	name string
	sub  SublayerValue
	aggr aggregation
}

// RawBucket is used to compute span data and aggregate it
// within a time-framed bucket. This should not be used outside
// the agent, use Bucket for this.
type RawBucket struct {
	// This should really have no public fields. At all.

	start    int64 // timestamp of start in our format
	duration int64 // duration of a bucket in nanoseconds

	// this should really remain private as it's subject to refactoring
	data         map[statsKey]*groupedStats
	sublayerData map[statsSubKey]*sublayerStats

	// internal buffer for aggregate strings - not threadsafe
	keyBuf bytes.Buffer
}

// NewRawBucket opens a new calculation bucket for time ts and initializes it properly
func NewRawBucket(ts, d int64) *RawBucket {
	// The only non-initialized value is the Duration which should be set by whoever closes that bucket
	return &RawBucket{
		start:        ts,
		duration:     d,
		data:         make(map[statsKey]*groupedStats),
		sublayerData: make(map[statsSubKey]*sublayerStats),
	}
}

// Export transforms a RawBucket into a Bucket, typically used
// before communicating data to the API, as RawBucket is the internal
// type while Bucket is the public, shared one.
func (sb *RawBucket) Export() Bucket {
	ret := NewBucket(sb.start, sb.duration)
	for k, v := range sb.data {
		hitsKey := GrainKey(k.name, HITS, k.aggr)
		tagSet := k.aggr.toTags()
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
	for k, v := range sb.sublayerData {
		key := GrainKey(k.name, k.sub.Metric, k.aggr) + "," + k.sub.Tag.Name + ":" + k.sub.Tag.Value
		tagSet := append(k.aggr.toTags(), k.sub.Tag)
		ret.Counts[key] = Count{
			Key:      key,
			Name:     k.name,
			Measure:  k.sub.Metric,
			TagSet:   tagSet,
			TopLevel: v.topLevel,
			Value:    float64(v.value),
		}
	}
	return ret
}

// HandleSpan adds the span to this bucket stats, aggregated with the finest grain matching given aggregators
func (sb *RawBucket) HandleSpan(s *WeightedSpan, env string, sublayers []SublayerValue) {
	if env == "" {
		panic("env should never be empty")
	}

	aggr := newAggregationFromSpan(s.Span, env)
	sb.add(s, aggr)

	for _, sub := range sublayers {
		sb.addSublayer(s, aggr, sub)
	}
}

func (sb *RawBucket) add(s *WeightedSpan, aggr aggregation) {
	var gs *groupedStats
	var ok bool

	key := statsKey{name: s.Name, aggr: aggr}
	if gs, ok = sb.data[key]; !ok {
		gs = newGroupedStats()
		sb.data[key] = gs
	}

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

func (sb *RawBucket) addSublayer(s *WeightedSpan, aggr aggregation, sub SublayerValue) {
	// This is not as efficient as a "regular" add as we don't update
	// all sublayers at once (one call for HITS, and another one for ERRORS, DURATION...)
	// when logically, if we have a sublayer for HITS, we also have one for DURATION,
	// they should indeed come together. Still room for improvement here.

	var ss *sublayerStats
	var ok bool

	key := statsSubKey{name: s.Name, sub: sub, aggr: aggr}
	if ss, ok = sb.sublayerData[key]; !ok {
		ss = &sublayerStats{}
		sb.sublayerData[key] = ss
	}

	if s.TopLevel {
		ss.topLevel += s.Weight
	}

	ss.value += int64(s.Weight * sub.Value)

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
