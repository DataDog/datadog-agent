// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpimpl

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	taggerfake "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/dogstatsdhttp"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestSketchIterator(t *testing.T) {
	payload := &pb.Payload{
		MetricData: &pb.MetricData{
			DictNameStr:        []byte("\x03foo\x03bar"),
			Types:              []uint64{0x14, 0x04},
			NameRefs:           []int64{1, 1},
			TagsetRefs:         []int64{0, 0},
			ResourcesRefs:      []int64{0, 0},
			Intervals:          []uint64{0, 0},
			NumPoints:          []uint64{2, 1},
			SourceTypeNameRefs: []int64{0, 0},
			OriginInfoRefs:     []int64{0, 0},
			Timestamps:         []int64{1000, 1000, 1000},
			ValsSint64:         []int64{4, 1, 3, 2, 6, 2, 4, 3, 5},
			SketchNumBins:      []uint64{2, 1, 0},
			SketchBinKeys:      []int32{0, 2, 1},
			SketchBinCnts:      []uint32{1, 1, 3},
		},
	}

	tagger := taggerfake.SetupFakeTagger(t)
	header := http.Header{}
	origin, err := originFromHeader(header, tagger)
	require.NoError(t, err)

	it, err := newSketchIterator(payload, origin, "default")
	require.NoError(t, err)
	require.NotNil(t, it)

	require.True(t, it.MoveNext())
	s := it.Current()
	require.Equal(t, "foo", s.Name)
	require.Equal(t, "default", s.Host)
	require.Equal(t, metrics.MetricSourceDogstatsd, s.Source)
	require.Len(t, s.Points, 2)

	pt := s.Points[0]
	require.Equal(t, int64(1000), pt.Ts)
	k, n := pt.Sketch.Cols()
	require.Equal(t, []int32{0, 2}, k)
	require.Equal(t, []uint32{1, 1}, n)
	cnt, min, max, sum, avg := pt.Sketch.BasicStats()
	require.Equal(t, int64(2), cnt)
	require.Equal(t, 1.0, min)
	require.Equal(t, 3.0, max)
	require.Equal(t, 4.0, sum)
	require.Equal(t, 2.0, avg)

	pt = s.Points[1]
	require.Equal(t, int64(2000), pt.Ts)
	k, n = pt.Sketch.Cols()
	require.Equal(t, []int32{1}, k)
	require.Equal(t, []uint32{3}, n)
	cnt, min, max, sum, avg = pt.Sketch.BasicStats()
	require.Equal(t, int64(3), cnt)
	require.Equal(t, 2.0, min)
	require.Equal(t, 4.0, max)
	require.Equal(t, 6.0, sum)
	require.Equal(t, 2.0, avg)

	require.True(t, it.MoveNext())
	s = it.Current()
	require.Equal(t, "bar", s.Name)
	require.Equal(t, "default", s.Host)
	require.Len(t, s.Points, 1)

	pt = s.Points[0]
	require.Equal(t, int64(3000), pt.Ts)
	k, n = pt.Sketch.Cols()
	require.Empty(t, k)
	require.Empty(t, n)
	cnt, min, max, sum, avg = pt.Sketch.BasicStats()
	require.Equal(t, int64(5), cnt)
	require.Equal(t, 0.0, min)
	require.Equal(t, 0.0, max)
	require.Equal(t, 0.0, sum)
	require.Equal(t, 0.0, avg) // sum=0/cnt=5=0

	require.False(t, it.MoveNext())
	require.NoError(t, it.err)
}

func TestSketchIteratorTagMerging(t *testing.T) {
	payload := &pb.Payload{
		MetricData: &pb.MetricData{
			DictNameStr:        []byte("\x03foo\x03bar"),
			DictTagStr:         []byte("\x03ook\x15dd.internal.card:high"),
			DictTagsets:        []int64{2, 1, 1, 1, 1},
			Types:              []uint64{0x04, 0x04},
			NameRefs:           []int64{1, 1},
			TagsetRefs:         []int64{1, 1},
			ResourcesRefs:      []int64{0, 0},
			Intervals:          []uint64{0, 0},
			NumPoints:          []uint64{1, 1},
			SourceTypeNameRefs: []int64{0, 0},
			OriginInfoRefs:     []int64{0, 0},
			Timestamps:         []int64{1000, 0},
			ValsSint64:         []int64{1, 1},
			SketchNumBins:      []uint64{0, 0},
		},
	}

	entityID := taggertypes.NewEntityID(taggertypes.ContainerID, "abc123")
	tagger := taggerfake.SetupFakeTagger(t)
	tagger.SetTags(entityID, "test",
		[]string{"low"},
		[]string{"orch"},
		[]string{"high"},
		[]string{"std"})

	header := http.Header{"X-Dsd-Ld": {"abc123"}}
	origin, err := originFromHeader(header, tagger)
	require.NoError(t, err)

	it, err := newSketchIterator(payload, origin, "default")
	require.NoError(t, err)

	require.True(t, it.MoveNext())
	s := it.Current()
	require.Equal(t, "foo", s.Name)
	require.Equal(t, tagset.NewCompositeTags([]string{"low", "orch", "high"}, []string{"ook"}), s.Tags)

	require.True(t, it.MoveNext())
	s = it.Current()
	require.Equal(t, "bar", s.Name)
	require.Equal(t, tagset.NewCompositeTags([]string{"low"}, []string{"ook"}), s.Tags)

	require.False(t, it.MoveNext())
}

func TestSketchIteratorHostOverride(t *testing.T) {
	payload := &pb.Payload{
		MetricData: &pb.MetricData{
			DictNameStr:        []byte("\x03foo"),
			DictResourceStr:    []byte("\x04host"),
			DictResourceLen:    []int64{1},
			DictResourceType:   []int64{1},
			DictResourceName:   []int64{0},
			Types:              []uint64{0x04},
			NameRefs:           []int64{1},
			TagsetRefs:         []int64{0},
			ResourcesRefs:      []int64{1},
			Intervals:          []uint64{0},
			NumPoints:          []uint64{1},
			SourceTypeNameRefs: []int64{0},
			OriginInfoRefs:     []int64{0},
			Timestamps:         []int64{1000},
			ValsSint64:         []int64{2},
			SketchNumBins:      []uint64{0},
		},
	}

	tagger := taggerfake.SetupFakeTagger(t)
	header := http.Header{}
	origin, err := originFromHeader(header, tagger)
	require.NoError(t, err)
	it, err := newSketchIterator(payload, origin, "default")
	require.NoError(t, err)
	require.True(t, it.MoveNext())
	s := it.Current()
	require.Equal(t, "foo", s.Name)
	require.Equal(t, "", s.Host)
	require.False(t, it.MoveNext())
}

func TestSketchIteratorWrongType(t *testing.T) {
	payload := &pb.Payload{
		MetricData: &pb.MetricData{
			DictNameStr:        []byte("\x03foo"),
			Types:              []uint64{0x11},
			NameRefs:           []int64{1},
			TagsetRefs:         []int64{0},
			ResourcesRefs:      []int64{0},
			Intervals:          []uint64{0},
			NumPoints:          []uint64{1},
			SourceTypeNameRefs: []int64{0},
			OriginInfoRefs:     []int64{0},
			Timestamps:         []int64{1000},
			ValsSint64:         []int64{42},
		},
	}

	tagger := taggerfake.SetupFakeTagger(t)
	header := http.Header{}
	origin, err := originFromHeader(header, tagger)
	require.NoError(t, err)
	it, err := newSketchIterator(payload, origin, "default")
	require.NoError(t, err)
	require.False(t, it.MoveNext())
	require.Error(t, it.err)
}
