// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpimpl

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/dogstatsdhttp"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

// seriesTestPayload builds a payload holding the count `foo` (1 point), the
// rate `bar` (2 points) and the gauge `baz` (2 points). Timestamps and values
// are shared across all three in delta-encoded columns, so skipping a metric
// must not shift what the following one decodes.
func seriesTestPayload() *pb.Payload {
	return &pb.Payload{
		MetricData: &pb.MetricData{
			DictNameStr:      []byte("\x03foo\x03bar\x03baz"),
			DictTagStr:       []byte("\x03ook\x15dd.internal.card:none\x15dd.internal.card:high"),
			DictTagsets:      []int64{2, 1, 1, 2, 1, 2, 1, 1},
			DictResourceStr:  []byte("\x04host"),
			DictResourceLen:  []int64{2},
			DictResourceType: []int64{1, 0},
			DictResourceName: []int64{0, 1},

			Types:              []uint64{0x11, 0x12, 0x13},
			NameRefs:           []int64{1, 1, 1},
			TagsetRefs:         []int64{1, 1, 1},
			ResourcesRefs:      []int64{0, 1, -1},
			Intervals:          []uint64{10, 10, 10},
			NumPoints:          []uint64{1, 2, 2},
			SourceTypeNameRefs: []int64{0, 0, 0},
			OriginInfoRefs:     []int64{0, 0, 0},
			Timestamps:         []int64{1000, 0, 10, -10, 10},
			ValsSint64:         []int64{5, 6, 7, 8, 9},
		},
	}
}

func seriesTestOrigin(t *testing.T) origin {
	t.Helper()

	entityID := taggertypes.NewEntityID(taggertypes.ContainerID, "123456789")

	tagger := tagger.SetupFakeTagger(t)
	tagger.SetTags(entityID, "test",
		[]string{"low"},
		[]string{"orch"},
		[]string{"high"},
		[]string{"std"})

	header := http.Header{"X-Dsd-Ld": {"123456789"}}
	origin, err := originFromHeader(header, tagger)
	require.NoError(t, err)

	return origin
}

func TestIterator(t *testing.T) {
	it, err := newSeriesIterator(seriesTestPayload(), seriesTestOrigin(t), "default", utilstrings.Matcher{})
	require.NoError(t, err)
	require.NotNil(t, it)

	require.True(t, it.MoveNext())
	require.Equal(t, &metrics.Serie{
		Name:     "foo",
		Tags:     tagset.NewCompositeTags([]string{}, []string{"ook"}),
		Host:     "default",
		MType:    metrics.APICountType,
		Interval: 10,
		Source:   metrics.MetricSourceDogstatsd,
		Points:   []metrics.Point{{Ts: 1000, Value: 5}},
	}, it.Current())

	require.True(t, it.MoveNext())
	require.Equal(t, &metrics.Serie{
		Name:     "bar",
		Tags:     tagset.NewCompositeTags([]string{"low", "orch", "high"}, []string{"ook"}),
		Host:     "",
		MType:    metrics.APIRateType,
		Interval: 10,
		Source:   metrics.MetricSourceDogstatsd,
		Points:   []metrics.Point{{Ts: 1000, Value: 6}, {Ts: 1010, Value: 7}},
	}, it.Current())

	require.True(t, it.MoveNext())
	require.Equal(t, &metrics.Serie{
		Name:     "baz",
		Tags:     tagset.NewCompositeTags([]string{"low"}, []string{"ook"}),
		Host:     "default",
		MType:    metrics.APIGaugeType,
		Interval: 10,
		Source:   metrics.MetricSourceDogstatsd,
		Points:   []metrics.Point{{Ts: 1000, Value: 8}, {Ts: 1010, Value: 9}},
	}, it.Current())
}

func TestIteratorFilterList(t *testing.T) {
	t.Run("exact match skips the metric", func(t *testing.T) {
		it, err := newSeriesIterator(seriesTestPayload(), seriesTestOrigin(t), "default",
			utilstrings.NewMatcher([]string{"bar"}, false))
		require.NoError(t, err)

		require.True(t, it.MoveNext())
		require.Equal(t, "foo", it.Current().Name)

		// baz must still decode the points that follow bar's in the payload.
		require.True(t, it.MoveNext())
		require.Equal(t, &metrics.Serie{
			Name:     "baz",
			Tags:     tagset.NewCompositeTags([]string{"low"}, []string{"ook"}),
			Host:     "default",
			MType:    metrics.APIGaugeType,
			Interval: 10,
			Source:   metrics.MetricSourceDogstatsd,
			Points:   []metrics.Point{{Ts: 1000, Value: 8}, {Ts: 1010, Value: 9}},
		}, it.Current())

		require.False(t, it.MoveNext())
		require.NoError(t, it.err)
	})

	t.Run("prefix match skips every matching metric", func(t *testing.T) {
		it, err := newSeriesIterator(seriesTestPayload(), seriesTestOrigin(t), "default",
			utilstrings.NewMatcher([]string{"ba"}, true))
		require.NoError(t, err)

		require.True(t, it.MoveNext())
		require.Equal(t, "foo", it.Current().Name)

		require.False(t, it.MoveNext())
		require.NoError(t, it.err)
	})

	t.Run("everything filtered", func(t *testing.T) {
		it, err := newSeriesIterator(seriesTestPayload(), seriesTestOrigin(t), "default",
			utilstrings.NewMatcher([]string{"foo", "bar", "baz"}, false))
		require.NoError(t, err)

		require.False(t, it.MoveNext())
		require.NoError(t, it.err)
	})
}
