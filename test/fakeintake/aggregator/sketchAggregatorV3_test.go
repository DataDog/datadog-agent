// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/DataDog/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	metricspb "github.com/DataDog/agent-payload/v5/gogen"
	intake_v3 "github.com/DataDog/agent-payload/v5/metrics/intake_v3"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

const (
	hasSum = uint32(intake_v3.PointFlags_pointHasSum)
	hasMin = uint32(intake_v3.PointFlags_pointHasMin)
	hasMax = uint32(intake_v3.PointFlags_pointHasMax)
)

// strDict encodes a dictionary column: each entry is a uvarint length followed by the string.
func strDict(strs ...string) []byte {
	var b []byte
	for _, s := range strs {
		b = binary.AppendUvarint(b, uint64(len(s)))
		b = append(b, s...)
	}
	return b
}

// v3Type packs a metric type and a value type into an entry of the types column.
func v3Type(m intake_v3.MetricType, v intake_v3.ValueType) uint64 {
	return uint64(m) | uint64(v)
}

// singleSeries returns the columns of one series "h" with one point at timestamp 1000,
// without tags or resources.
func singleSeries(types uint64) *intake_v3.MetricData {
	return &intake_v3.MetricData{
		DictNameStr:        strDict("h"),
		Types:              []uint64{types},
		NameRefs:           []int64{1},
		TagsetRefs:         []int64{0},
		ResourcesRefs:      []int64{0},
		SourceTypeNameRefs: []int64{0},
		OriginInfoRefs:     []int64{0},
		Intervals:          []uint64{0},
		NumPoints:          []uint64{1},
		Timestamps:         []int64{1000},
	}
}

// payloadV3 wraps metric data into an encoded Payload.
func payloadV3(t *testing.T, md *intake_v3.MetricData) api.Payload {
	t.Helper()
	data, err := proto.Marshal(&intake_v3.Payload{MetricData: md})
	require.NoError(t, err)
	return api.Payload{
		Data:      data,
		Encoding:  encodingEmpty,
		Timestamp: time.Unix(1700000000, 0),
	}
}

func ptr(v float64) *float64 { return &v }

// Example from the ExplicitHistogram documentation in intake_v3.proto.
func explicitSpecExample() *intake_v3.MetricData {
	md := singleSeries(v3Type(intake_v3.MetricType_ExplicitHistogram, intake_v3.ValueType_Sint64))
	md.PointFlags = []uint32{hasSum}
	md.NumBins = []uint64{2}
	md.BinCnts = []uint64{2, 3}
	md.ValsUint64 = []uint64{5}     // count
	md.ValsSint64 = []int64{30, 10} // sum, bounds
	return md
}

// Example from the ExponentialHistogram documentation in intake_v3.proto.
func exponentialSpecExample() *intake_v3.MetricData {
	md := singleSeries(v3Type(intake_v3.MetricType_ExponentialHistogram, intake_v3.ValueType_Sint64))
	md.PointFlags = []uint32{hasSum}
	md.NumBins = []uint64{3, 1}              // positive, negative
	md.BinCnts = []uint64{2, 0, 3, 4}        // positive, then negative
	md.ValsUint64 = []uint64{10, 1}          // count, zeroCount
	md.ValsSint64 = []int64{0, 0, -1, 18, 0} // scale, posOffset, negOffset, sum, zeroThreshold
	return md
}

func TestParseHistograms_ExplicitSpecExample(t *testing.T) {
	histograms, err := ParseHistograms(payloadV3(t, explicitSpecExample()))
	require.NoError(t, err)
	require.Len(t, histograms, 1)
	h := histograms[0]
	assert.Equal(t, "h", h.Metric)
	assert.Empty(t, h.Tags)
	assert.Empty(t, h.Exponential)
	assert.Equal(t, []*ExplicitHistogramPoint{{
		Timestamp: 1000,
		Count:     5,
		Sum:       ptr(30),
		Bounds:    []float64{10},
		Counts:    []uint64{2, 3},
	}}, h.Explicit)
}

func TestParseHistograms_ExponentialSpecExample(t *testing.T) {
	histograms, err := ParseHistograms(payloadV3(t, exponentialSpecExample()))
	require.NoError(t, err)
	require.Len(t, histograms, 1)
	h := histograms[0]
	assert.Equal(t, "h", h.Metric)
	assert.Empty(t, h.Explicit)
	assert.Equal(t, []*ExponentialHistogramPoint{{
		Timestamp:      1000,
		Count:          10,
		Sum:            ptr(18),
		Scale:          0,
		ZeroCount:      1,
		ZeroThreshold:  0,
		PositiveOffset: 0,
		Positive:       []uint64{2, 0, 3},
		NegativeOffset: -1,
		Negative:       []uint64{4},
	}}, h.Exponential)
}

func TestParseHistograms_ValueEncodings(t *testing.T) {
	t.Run("float64 with min and max", func(t *testing.T) {
		md := singleSeries(v3Type(intake_v3.MetricType_ExplicitHistogram, intake_v3.ValueType_Float64))
		md.DictTagStr = strDict("env:test")
		md.DictTagsets = []int64{1, 1} // one tagset with tag 1
		md.TagsetRefs = []int64{1}
		md.PointFlags = []uint32{hasSum | hasMin | hasMax}
		md.NumBins = []uint64{3}
		md.BinCnts = []uint64{1, 1, 1}
		md.ValsUint64 = []uint64{3}
		md.ValsFloat64 = []float64{2.5, -1.25, 7.75, -1, 0.5} // sum, min, max, bounds
		histograms, err := ParseHistograms(payloadV3(t, md))
		require.NoError(t, err)
		require.Len(t, histograms, 1)
		assert.Equal(t, []string{"env:test"}, histograms[0].Tags)
		assert.Equal(t, []*ExplicitHistogramPoint{{
			Timestamp: 1000, Count: 3,
			Sum: ptr(2.5), Min: ptr(-1.25), Max: ptr(7.75),
			Bounds: []float64{-1, 0.5}, Counts: []uint64{1, 1, 1},
		}}, histograms[0].Explicit)
	})

	t.Run("float32 exponential", func(t *testing.T) {
		md := singleSeries(v3Type(intake_v3.MetricType_ExponentialHistogram, intake_v3.ValueType_Float32))
		md.PointFlags = []uint32{hasMin | hasMax}
		md.NumBins = []uint64{1, 0}
		md.BinCnts = []uint64{2}
		md.ValsUint64 = []uint64{2, 0}
		md.ValsSint64 = []int64{-2, 5, 0}            // scale, posOffset, negOffset
		md.ValsFloat32 = []float32{0.5, 0.25, 0.125} // min, max, zeroThreshold
		histograms, err := ParseHistograms(payloadV3(t, md))
		require.NoError(t, err)
		require.Len(t, histograms, 1)
		assert.Equal(t, []*ExponentialHistogramPoint{{
			Timestamp: 1000, Count: 2,
			Min: ptr(0.5), Max: ptr(0.25),
			Scale: -2, ZeroThreshold: 0.125,
			PositiveOffset: 5, Positive: []uint64{2},
			Negative: []uint64{},
		}}, histograms[0].Exponential)
	})

	// The Zero value type takes no slot: absent summary and a single bound at 0.
	t.Run("zero", func(t *testing.T) {
		md := singleSeries(v3Type(intake_v3.MetricType_ExplicitHistogram, intake_v3.ValueType_Zero))
		md.PointFlags = []uint32{0}
		md.NumBins = []uint64{2}
		md.BinCnts = []uint64{4, 0}
		md.ValsUint64 = []uint64{4}
		histograms, err := ParseHistograms(payloadV3(t, md))
		require.NoError(t, err)
		require.Len(t, histograms, 1)
		assert.Equal(t, []*ExplicitHistogramPoint{{Timestamp: 1000, Count: 4, Bounds: []float64{0}, Counts: []uint64{4, 0}}}, histograms[0].Explicit)
	})
}

// Bin counts and the count keep the full uint64 range.
func TestParseHistograms_CountsAboveUint32(t *testing.T) {
	const big = uint64(1) << 40
	md := singleSeries(v3Type(intake_v3.MetricType_ExplicitHistogram, intake_v3.ValueType_Zero))
	md.PointFlags = []uint32{0}
	md.NumBins = []uint64{1}
	md.BinCnts = []uint64{big}
	md.ValsUint64 = []uint64{big}
	histograms, err := ParseHistograms(payloadV3(t, md))
	require.NoError(t, err)
	require.Len(t, histograms, 1)
	assert.Equal(t, []*ExplicitHistogramPoint{{Timestamp: 1000, Count: big, Bounds: []float64{}, Counts: []uint64{big}}}, histograms[0].Explicit)
}

// DDSketch series use both binKeys and binCnts, histograms only binCnts: the two
// columns must be indexed separately when the kinds are mixed.
func TestParseSketchPayloadV3_MixedKinds(t *testing.T) {
	sketch := v3Type(intake_v3.MetricType_DDSketch, intake_v3.ValueType_Sint64)
	md := &intake_v3.MetricData{
		DictNameStr:        strDict("sketch.a", "hist", "sketch.b"),
		DictResourceStr:    strDict("host", "node-a"),
		DictResourceLen:    []int64{1},
		DictResourceType:   []int64{1},
		DictResourceName:   []int64{2},
		Types:              []uint64{sketch, v3Type(intake_v3.MetricType_ExplicitHistogram, intake_v3.ValueType_Zero), sketch},
		NameRefs:           []int64{1, 1, 1},
		TagsetRefs:         []int64{0, 0, 0},
		ResourcesRefs:      []int64{1, -1, 1},
		SourceTypeNameRefs: []int64{0, 0, 0},
		OriginInfoRefs:     []int64{0, 0, 0},
		Intervals:          []uint64{10, 0, 10},
		NumPoints:          []uint64{1, 1, 1},
		Timestamps:         []int64{1000, 0, 10},
		// DDSketch: sum, min, max, cnt. The histogram takes no value slot.
		ValsSint64: []int64{6, 1, 3, 3, 9, 4, 5, 2},
		NumBins:    []uint64{2, 1, 1},
		BinKeys:    []int32{5, 2, -3}, // delta encoded per sketch: [5, 7], [-3]
		BinCnts:    []uint64{1, 2, 7, 2},
		PointFlags: []uint32{0},
		ValsUint64: []uint64{7},
	}
	p, err := ParseSketchPayloadV3(payloadV3(t, md))
	require.NoError(t, err)

	require.Len(t, p.Sketches, 2)
	assert.Equal(t, "sketch.a", p.Sketches[0].Metric)
	assert.Equal(t, "node-a", p.Sketches[0].Host)
	assert.Equal(t, []metricspb.SketchPayload_Sketch_Dogsketch{{Ts: 1000, Cnt: 3, Min: 1, Max: 3, Sum: 6, Avg: 2, K: []int32{5, 7}, N: []uint32{1, 2}}}, p.Sketches[0].Dogsketches)
	assert.Equal(t, "sketch.b", p.Sketches[1].Metric)
	assert.Equal(t, []metricspb.SketchPayload_Sketch_Dogsketch{{Ts: 1010, Cnt: 2, Min: 4, Max: 5, Sum: 9, Avg: 4.5, K: []int32{-3}, N: []uint32{2}}}, p.Sketches[1].Dogsketches)

	require.Len(t, p.Histograms, 1)
	assert.Equal(t, "hist", p.Histograms[0].Metric)
	assert.Empty(t, p.Histograms[0].Resources)
	assert.Equal(t, []*ExplicitHistogramPoint{{Timestamp: 1000, Count: 7, Bounds: []float64{}, Counts: []uint64{7}}}, p.Histograms[0].Explicit)

	sketches, err := ParseSketchesV3(payloadV3(t, md))
	require.NoError(t, err)
	assert.Len(t, sketches, 2)
}

func TestParseSketchPayloadV3_Compressed(t *testing.T) {
	payload := payloadV3(t, exponentialSpecExample())
	compressed, err := zstd.Compress(nil, payload.Data)
	require.NoError(t, err)
	payload.Data = compressed
	payload.Encoding = encodingZstd

	histograms, err := ParseHistograms(payload)
	require.NoError(t, err)
	require.Len(t, histograms, 1)
	assert.Equal(t, payload.Timestamp, histograms[0].GetCollectedTime())
}

func TestParseSketchPayloadV3_Empty(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("{}")} {
		p, err := ParseSketchPayloadV3(api.Payload{Data: data, Encoding: encodingEmpty})
		require.NoError(t, err)
		assert.Empty(t, p.Sketches)
		assert.Empty(t, p.Histograms)
	}
}

func TestParseSketchPayloadV3_Malformed(t *testing.T) {
	// Each case breaks one column of a valid payload. A nil error means any error.
	with := func(md *intake_v3.MetricData, f func(*intake_v3.MetricData)) *intake_v3.MetricData {
		f(md)
		return md
	}
	tests := map[string]struct {
		md  *intake_v3.MetricData
		err error
	}{
		"unknown point flag": {with(explicitSpecExample(), func(md *intake_v3.MetricData) { md.PointFlags = []uint32{8} }), errV3BadPointFlags},
		"bins past binCnts":  {with(explicitSpecExample(), func(md *intake_v3.MetricData) { md.NumBins = []uint64{3} }), errV3UnexpectedEOF},
		"missing valsUint64": {with(explicitSpecExample(), func(md *intake_v3.MetricData) { md.ValsUint64 = nil }), errV3UnexpectedEOF},
		"values past the end": {with(explicitSpecExample(), func(md *intake_v3.MetricData) {
			md.PointFlags = []uint32{hasSum | hasMin | hasMax}
		}), errV3UnexpectedEOF},
		"bounds not increasing": {with(explicitSpecExample(), func(md *intake_v3.MetricData) {
			md.NumBins, md.BinCnts, md.ValsSint64 = []uint64{3}, []uint64{1, 1, 1}, []int64{30, 10, 10}
		}), errV3BadBounds},
		"infinite bound": {with(explicitSpecExample(), func(md *intake_v3.MetricData) {
			md.Types = []uint64{v3Type(intake_v3.MetricType_ExplicitHistogram, intake_v3.ValueType_Float64)}
			md.ValsSint64, md.ValsFloat64 = nil, []float64{30, math.Inf(1)}
		}), errV3BadBounds},
		"negative zero threshold": {with(exponentialSpecExample(), func(md *intake_v3.MetricData) { md.ValsSint64[4] = -1 }), errV3BadZeroThreshold},
		"NaN zero threshold": {with(exponentialSpecExample(), func(md *intake_v3.MetricData) {
			md.Types = []uint64{v3Type(intake_v3.MetricType_ExponentialHistogram, intake_v3.ValueType_Float64)}
			md.ValsSint64, md.ValsFloat64 = []int64{0, 0, -1}, []float64{18, math.NaN()}
		}), errV3BadZeroThreshold},
		"last bucket index outside int32": {with(exponentialSpecExample(), func(md *intake_v3.MetricData) {
			md.ValsSint64[1] = math.MaxInt32 // positive offset of a 3 bucket range
		}), errV3BadBucketIndex},
		"DDSketch bin count above uint32": {with(singleSeries(v3Type(intake_v3.MetricType_DDSketch, intake_v3.ValueType_Sint64)), func(md *intake_v3.MetricData) {
			md.ValsSint64, md.NumBins, md.BinKeys, md.BinCnts = []int64{0, 0, 0, 1}, []uint64{1}, []int32{0}, []uint64{1 << 40}
		}), nil},
		"series type in route": {with(singleSeries(v3Type(intake_v3.MetricType_Gauge, intake_v3.ValueType_Float64)), func(md *intake_v3.MetricData) {
			md.ValsFloat64 = []float64{1}
		}), nil},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ParseSketchPayloadV3(payloadV3(t, tc.md))
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestParseMetricSeriesV3_HistogramInSeriesPayloadErrors(t *testing.T) {
	_, err := ParseMetricSeriesV3(payloadV3(t, explicitSpecExample()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected sketch metric")
}

func TestHistogramAggregator(t *testing.T) {
	agg := NewHistogramAggregator()
	require.NoError(t, agg.UnmarshallPayloads([]api.Payload{payloadV3(t, explicitSpecExample()), payloadV3(t, exponentialSpecExample())}))
	assert.Equal(t, []string{"h"}, agg.GetNames())
	assert.Len(t, agg.GetPayloadsByName("h"), 2)
}
