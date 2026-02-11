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
	pb := &pb.MetricData{
		DictNameStr:      []byte("\x03foo\x03bar"),
		DictTagsStr:      []byte("\x03baz"),
		DictTagsets:      []int64{1, 1},
		DictResourceStr:  []byte("\x01a\x01b\x01c\x01d\x01e"),
		DictResourceLen:  []int64{1, 2, 2},
		DictResourceType: []int64{1, 1, 2, 1, 2},
		DictResourceName: []int64{2, 2, 2, 2, 3},
		DictOriginInfo:   []int32{10, 10, 0},
		Types:            []uint64{0x11, 0x21, 0x31},
		Names:            []int64{1, 0, 1},
		Tags:             []int64{1, -1, 1},
		Resources:        []int64{1, 1, 1},
		Intervals:        []uint64{10, 10, 10},
		OriginInfos:      []int64{1, 0, 0},
		SourceTypeNames:  []int64{0, 0, 0},
		NumPoints:        []uint64{1, 1, 1},
		Timestamps:       []int64{10000, 0, 0},
		ValsSint64:       []int64{42},
		ValsFloat32:      []float32{0.5},
		ValsFloat64:      []float64{3.14},
	}

	r := NewMetricDataReader(pb)
	require.NoError(t, r.Initialize())

	require.True(t, r.HaveMoreMetrics())
	require.NoError(t, r.NextMetric())
	require.Equal(t, "foo", r.Name())
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
