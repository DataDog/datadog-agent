// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/golang/protobuf/proto"
)

const (
	relativeAccuracy = 0.01
	maxNumBins       = 2048
)

// Most "algorithm" stuff here is tested with stats_test.go as what is important
// is that the final data, the one with send after a call to Export(), is correct.

type groupedStats struct {
	hits            float64
	topLevelHits    float64
	errors          float64
	duration        float64
	okDistribution  *ddsketch.DDSketch
	errDistribution *ddsketch.DDSketch
}

func (s *groupedStats) export(k statsKey) (pb.ClientGroupedStats, error) {
	msg := s.okDistribution.ToProto()
	okSummary, err := proto.Marshal(msg)
	if err != nil {
		return pb.ClientGroupedStats{}, err
	}
	msg = s.errDistribution.ToProto()
	errSummary, err := proto.Marshal(msg)
	if err != nil {
		return pb.ClientGroupedStats{}, err
	}
	return pb.ClientGroupedStats{
		Service:        k.aggr.Service,
		Name:           k.name,
		Resource:       k.aggr.Resource,
		HTTPStatusCode: k.aggr.StatusCode,
		Type:           k.aggr.Type,
		Hits:           uint64(s.hits),
		Errors:         uint64(s.errors),
		Duration:       uint64(s.duration),
		OkSummary:      okSummary,
		ErrorSummary:   errSummary,
		Synthetics:     k.aggr.Synthetics,
		TopLevelHits:   uint64(s.topLevelHits),
	}, nil
}

func newGroupedStats() *groupedStats {
	ok, _ := ddsketch.LogCollapsingLowestDenseDDSketch(relativeAccuracy, maxNumBins)
	err, _ := ddsketch.LogCollapsingLowestDenseDDSketch(relativeAccuracy, maxNumBins)
	return &groupedStats{
		okDistribution:  ok,
		errDistribution: err,
	}
}

type statsKey struct {
	name string
	aggr Aggregation
}

// RawBucket is used to compute span data and aggregate it
// within a time-framed bucket. This should not be used outside
// the agent, use ClientStatsBucket for this.
type RawBucket struct {
	// This should really have no public fields. At all.

	start    uint64 // timestamp of start in our format
	duration uint64 // duration of a bucket in nanoseconds

	// this should really remain private as it's subject to refactoring
	data map[statsKey]*groupedStats

	// internal buffer for aggregate strings - not threadsafe
	keyBuf strings.Builder
}

// PayloadKey is the key of a stats payload.
type PayloadKey struct {
	hostname string
	version  string
	env      string
}

// NewRawBucket opens a new calculation bucket for time ts and initializes it properly
func NewRawBucket(ts, d uint64) *RawBucket {
	// The only non-initialized value is the Duration which should be set by whoever closes that bucket
	return &RawBucket{
		start:    ts,
		duration: d,
		data:     make(map[statsKey]*groupedStats),
	}
}

// Export transforms a RawBucket into a ClientStatsBucket, typically used
// before communicating data to the API, as RawBucket is the internal
// type while ClientStatsBucket is the public, shared one.
func (sb *RawBucket) Export() map[PayloadKey]pb.ClientStatsBucket {
	m := make(map[PayloadKey]pb.ClientStatsBucket)
	for k, v := range sb.data {
		b, err := v.export(k)
		if err != nil {
			log.Errorf("Dropping stats bucket due to encoding error: %v.", err)
			continue
		}
		key := PayloadKey{
			hostname: k.aggr.Hostname,
			version:  k.aggr.Version,
			env:      k.aggr.Env,
		}
		s, ok := m[key]
		if !ok {
			s = pb.ClientStatsBucket{
				Start:    sb.start,
				Duration: sb.duration,
			}
		}
		s.Stats = append(s.Stats, b)
		m[key] = s
	}
	return m
}

// HandleSpan adds the span to this bucket stats, aggregated with the finest grain matching given aggregators
func (sb *RawBucket) HandleSpan(s *WeightedSpan, env string, agentHostname string) {
	if env == "" {
		panic("env should never be empty")
	}
	aggr := NewAggregationFromSpan(s.Span, env, agentHostname)
	sb.add(s, aggr)
}

func (sb *RawBucket) add(s *WeightedSpan, aggr Aggregation) {
	var gs *groupedStats
	var ok bool

	key := statsKey{name: s.Name, aggr: aggr}
	if gs, ok = sb.data[key]; !ok {
		gs = newGroupedStats()
		sb.data[key] = gs
	}
	if s.TopLevel {
		gs.topLevelHits += s.Weight
	}
	gs.hits += s.Weight
	if s.Error != 0 {
		gs.errors += s.Weight
	}
	gs.duration += float64(s.Duration) * s.Weight
	// alter resolution of duration distro
	trundur := nsTimestampToFloat(s.Duration)
	if s.Error != 0 {
		gs.errDistribution.Add(trundur)
	} else {
		gs.okDistribution.Add(trundur)
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
