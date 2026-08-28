// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package reader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/dogstatsdhttp"
)

func TestBasic(t *testing.T) {
	md := &pb.MetricData{
		DictNameStr:        []byte("\x03foo\x03bar"),
		DictTagStr:         []byte("\x03baz"),
		DictTagsets:        []int64{1, 1},
		DictResourceStr:    []byte("\x01a\x01b\x01c\x01d\x01e"),
		DictResourceLen:    []int64{1, 2, 2},
		DictResourceType:   []int64{1, 1, 2, 1, 2},
		DictResourceName:   []int64{2, 2, 2, 2, 3},
		DictOriginInfo:     []int32{10, 10, 0},
		Types:              []uint64{0x11, 0x22, 0x33, 0x4},
		NameRefs:           []int64{1, 0, 1, 0},
		TagsetRefs:         []int64{1, -1, 1, 0},
		ResourcesRefs:      []int64{1, 1, 1, -1},
		Intervals:          []uint64{10, 10, 10, 0},
		OriginInfoRefs:     []int64{1, 0, 0, -1},
		SourceTypeNameRefs: []int64{0, 0, 0, 0},
		NumPoints:          []uint64{1, 1, 1, 1},
		Timestamps:         []int64{10000, 0, 0, 0},
		ValsSint64:         []int64{42, 4},
		ValsFloat32:        []float32{0.5},
		ValsFloat64:        []float64{3.14},
		SketchNumBins:      []uint64{1},
		SketchBinKeys:      []int32{0},
		SketchBinCnts:      []uint32{4},
	}

	r := NewMetricDataReader(md)
	require.NoError(t, r.Initialize())

	require.True(t, r.HaveMoreMetrics())
	require.NoError(t, r.NextMetric())
	require.Equal(t, "foo", r.Name())
	require.Equal(t, pb.MetricType_Count, r.Type())
	require.EqualValues(t, []string{"baz"}, r.Tags())
	require.Equal(t, uint64(10), r.Interval())
	require.Equal(t, "", r.SourceTypeName())
	require.Equal(t, &originInfo{
		OriginProduct:  10,
		OriginCategory: 10,
		OriginService:  0,
	}, r.Origin())
	require.True(t, r.HaveMorePoints())
	require.NoError(t, r.NextPoint())
	require.Equal(t, float64(42), r.Value())
	require.False(t, r.HaveMorePoints())

	require.True(t, r.HaveMoreMetrics())
	require.NoError(t, r.NextMetric())
	require.Equal(t, pb.MetricType_Rate, r.Type())
	require.Equal(t, "foo", r.Name())
	require.Equal(t, []string(nil), r.Tags())
	require.Equal(t, uint64(10), r.Interval())
	require.Equal(t, "", r.SourceTypeName())
	require.Equal(t, &originInfo{
		OriginProduct:  10,
		OriginCategory: 10,
		OriginService:  0,
	}, r.Origin())
	require.True(t, r.HaveMorePoints())
	require.NoError(t, r.NextPoint())
	require.Equal(t, 0.5, r.Value())
	require.False(t, r.HaveMorePoints())

	require.True(t, r.HaveMoreMetrics())
	require.NoError(t, r.NextMetric())
	require.Equal(t, pb.MetricType_Gauge, r.Type())
	require.Equal(t, "bar", r.Name())
	require.EqualValues(t, []string{"baz"}, r.Tags())
	require.Equal(t, uint64(10), r.Interval())
	require.Equal(t, "", r.SourceTypeName())
	require.Equal(t, &originInfo{
		OriginProduct:  10,
		OriginCategory: 10,
		OriginService:  0,
	}, r.Origin())
	require.True(t, r.HaveMorePoints())
	require.NoError(t, r.NextPoint())
	require.Equal(t, 3.14, r.Value())
	require.False(t, r.HaveMorePoints())

	require.True(t, r.HaveMoreMetrics())
	require.NoError(t, r.NextMetric())
	require.Equal(t, "bar", r.Name())
	require.EqualValues(t, []string{"baz"}, r.Tags())
	require.Equal(t, uint64(0), r.Interval())
	require.Equal(t, "", r.SourceTypeName())
	require.Nil(t, r.Origin())
	require.True(t, r.HaveMorePoints())
	require.NoError(t, r.NextPoint())
	require.Equal(t, pb.MetricType_Sketch, r.Type())

	sum, min, max, cnt := r.SketchSummary()
	require.Equal(t, 0., sum)
	require.Equal(t, 0., min)
	require.Equal(t, 0., max)
	require.Equal(t, uint64(4), cnt)
	k, n := r.SketchCols()
	require.Equal(t, []int32{0}, k)
	require.Equal(t, []uint32{4}, n)

	require.False(t, r.HaveMorePoints())
	require.False(t, r.HaveMoreMetrics())
}

func TestDictResources(t *testing.T) {
	pb := &pb.MetricData{
		DictResourceStr:  []byte("\x01a\x01b\x01c\x01d\x01e"),
		DictResourceLen:  []int64{1, 2, 2},
		DictResourceType: []int64{1, 1, 2, 1, 2},
		DictResourceName: []int64{2, 2, 2, 2, 3},
	}

	r := NewMetricDataReader(pb)
	r.Initialize()

	assert.Equal(t, r.dictResources, [][]*resource{
		nil,
		{{Type: "a", Name: "b"}},
		{{Type: "a", Name: "b"}, {Type: "c", Name: "d"}},
		{{Type: "a", Name: "b"}, {Type: "c", Name: "e"}},
	})
}

func FuzzReader(f *testing.F) {
	f.Fuzz(func(_ *testing.T, bytes []byte) {
		var p pb.Payload
		if err := p.UnmarshalVT(bytes); err != nil {
			return
		}
		if p.MetricData == nil {
			return
		}
		mr := NewMetricDataReader(p.MetricData)
		if err := mr.Initialize(); err != nil {
			return
		}
		for mr.HaveMoreMetrics() {
			if err := mr.NextMetric(); err != nil {
				return
			}
			mr.Type()
			mr.ValueType()
			mr.Name()
			mr.Tags()
			mr.Resources()
			mr.SourceTypeName()
			mr.Origin()
			mr.Interval()
			mr.NumPoints()
			for mr.HaveMorePoints() {
				if err := mr.NextPoint(); err != nil {
					return
				}
				_ = mr.Timestamp()
				switch mr.Type() {
				case pb.MetricType_Sketch:
					_, _, _, _ = mr.SketchSummary()
					_ = mr.SketchNumBins()
					_, _ = mr.SketchCols()
				default:
					_ = mr.Value()
				}
			}
		}
	})
}

func TestSketchIndex(t *testing.T) {
	pb := &pb.MetricData{
		Types:              []uint64{0x4},
		NameRefs:           []int64{0},
		TagsetRefs:         []int64{0},
		ResourcesRefs:      []int64{0},
		Intervals:          []uint64{0},
		OriginInfoRefs:     []int64{0},
		SourceTypeNameRefs: []int64{0},
		NumPoints:          []uint64{1},
		Timestamps:         []int64{0},
		ValsSint64:         []int64{1},
		SketchNumBins:      []uint64{}, // malformed
	}

	r := NewMetricDataReader(pb)
	require.NoError(t, r.Initialize())

	require.True(t, r.HaveMoreMetrics())
	require.NoError(t, r.NextMetric())
	require.True(t, r.HaveMorePoints())
	require.Error(t, r.NextPoint())
}

func TestNil(t *testing.T) {
	r := NewMetricDataReader(nil)
	require.NoError(t, r.Initialize())
	require.False(t, r.HaveMoreMetrics())
	require.Error(t, r.NextMetric())
	require.False(t, r.HaveMorePoints())
	require.Error(t, r.NextPoint())
}

func TestEmpty(t *testing.T) {
	r := NewMetricDataReader(&pb.MetricData{})
	require.NoError(t, r.Initialize())
	require.False(t, r.HaveMoreMetrics())
	require.Error(t, r.NextMetric())
	require.False(t, r.HaveMorePoints())
	require.Error(t, r.NextPoint())
}

func TestTagCardinality(t *testing.T) {
	// Every entry is a gauge with flagNoIndex set, so the cardinality nibble
	// must not pick up the flag bits sitting below it.
	md := &pb.MetricData{
		DictNameStr:        []byte("\x03foo"),
		Types:              []uint64{0x103, 0x1103, 0x2103, 0x3103, 0x4103},
		NameRefs:           []int64{1, 0, 0, 0, 0},
		TagsetRefs:         []int64{0, 0, 0, 0, 0},
		ResourcesRefs:      []int64{0, 0, 0, 0, 0},
		Intervals:          []uint64{0, 0, 0, 0, 0},
		NumPoints:          []uint64{0, 0, 0, 0, 0},
		SourceTypeNameRefs: []int64{0, 0, 0, 0, 0},
		OriginInfoRefs:     []int64{0, 0, 0, 0, 0},
	}

	r := NewMetricDataReader(md)
	require.NoError(t, r.Initialize())

	for _, want := range []pb.TagCardinality{
		pb.TagCardinality_Unset,
		pb.TagCardinality_None,
		pb.TagCardinality_Low,
		pb.TagCardinality_Orch,
		pb.TagCardinality_High,
	} {
		require.True(t, r.HaveMoreMetrics())
		require.NoError(t, r.NextMetric())
		require.Equal(t, want, r.TagCardinality())
		require.Equal(t, pb.MetricType_Gauge, r.Type())
		require.Equal(t, pb.ValueType_Zero, r.ValueType())
	}

	require.False(t, r.HaveMoreMetrics())
}

func TestCardinalityTagsAreFiltered(t *testing.T) {
	// Tagset 1 is {ook, dd.internal.card:high}, tagset 2 inherits tagset 1 and
	// adds eek, tagset 3 holds nothing but cardinality tags.
	md := &pb.MetricData{
		DictNameStr:        []byte("\x03foo"),
		DictTagStr:         []byte("\x03ook\x15dd.internal.card:high\x03eek\x14dd.internal.card:low"),
		DictTagsets:        []int64{2, 1, 1, 2, -1, 4, 2, 2, 2},
		Types:              []uint64{0x3, 0x3, 0x3},
		NameRefs:           []int64{1, 0, 0},
		TagsetRefs:         []int64{1, 1, 1},
		ResourcesRefs:      []int64{0, 0, 0},
		Intervals:          []uint64{0, 0, 0},
		NumPoints:          []uint64{0, 0, 0},
		SourceTypeNameRefs: []int64{0, 0, 0},
		OriginInfoRefs:     []int64{0, 0, 0},
	}

	r := NewMetricDataReader(md)
	require.NoError(t, r.Initialize())

	require.NoError(t, r.NextMetric())
	require.Equal(t, []string{"ook"}, r.Tags())

	// The inherited tagset must pick up the already filtered version.
	require.NoError(t, r.NextMetric())
	require.Equal(t, []string{"ook", "eek"}, r.Tags())

	require.NoError(t, r.NextMetric())
	require.Empty(t, r.Tags())

	require.False(t, r.HaveMoreMetrics())
}
