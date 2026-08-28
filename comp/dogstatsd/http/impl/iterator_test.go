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
			DictTagStr:       []byte("\x03ook\x03eek"),
			DictTagsets:      []int64{1, 1, 2, 1, 1, 1, 2},
			DictResourceStr:  []byte("\x04host"),
			DictResourceLen:  []int64{2},
			DictResourceType: []int64{1, 0},
			DictResourceName: []int64{0, 1},

			Types:              []uint64{0x1011, 0x4012, 0x13},
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
		Tags:     tagset.NewCompositeTags([]string{"low", "orch", "high"}, []string{"ook", "eek"}),
		Host:     "",
		MType:    metrics.APIRateType,
		Interval: 10,
		Source:   metrics.MetricSourceDogstatsd,
		Points:   []metrics.Point{{Ts: 1000, Value: 6}, {Ts: 1010, Value: 7}},
	}, it.Current())

	require.True(t, it.MoveNext())
	require.Equal(t, &metrics.Serie{
		Name:     "baz",
		Tags:     tagset.NewCompositeTags([]string{"low"}, []string{"eek"}),
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
			Tags:     tagset.NewCompositeTags([]string{"low"}, []string{"eek"}),
			Host:     "default",
			MType:    metrics.APIGaugeType,
			Interval: 10,
			Source:   metrics.MetricSourceDogstatsd,
			Points:   []metrics.Point{{Ts: 1000, Value: 8}, {Ts: 1010, Value: 9}},
		}, it.Current())

		require.False(t, it.MoveNext())
		require.NoError(t, it.err)
		require.Equal(t, payloadStats{metrics: 2, points: 3, filteredMetrics: 1, filteredPoints: 2}, it.stats)
	})

	t.Run("prefix match skips every matching metric", func(t *testing.T) {
		it, err := newSeriesIterator(seriesTestPayload(), seriesTestOrigin(t), "default",
			utilstrings.NewMatcher([]string{"ba"}, true))
		require.NoError(t, err)

		require.True(t, it.MoveNext())
		require.Equal(t, "foo", it.Current().Name)

		require.False(t, it.MoveNext())
		require.NoError(t, it.err)
		require.Equal(t, payloadStats{metrics: 1, points: 1, filteredMetrics: 2, filteredPoints: 4}, it.stats)
	})

	t.Run("everything filtered", func(t *testing.T) {
		it, err := newSeriesIterator(seriesTestPayload(), seriesTestOrigin(t), "default",
			utilstrings.NewMatcher([]string{"foo", "bar", "baz"}, false))
		require.NoError(t, err)

		require.False(t, it.MoveNext())
		require.NoError(t, it.err)
		require.Equal(t, payloadStats{filteredMetrics: 3, filteredPoints: 5}, it.stats)
	})
}

func TestIteratorTagCardinality(t *testing.T) {
	// Each entry is a gauge named foo carrying the single client tag `ook`,
	// they differ only in the cardinality nibble of the type column.
	payload := func(types []uint64) *pb.Payload {
		zeros := make([]int64, len(types))
		return &pb.Payload{
			MetricData: &pb.MetricData{
				DictNameStr:        []byte("\x03foo"),
				DictTagStr:         []byte("\x03ook"),
				DictTagsets:        []int64{1, 1},
				Types:              types,
				NameRefs:           append([]int64{1}, zeros[1:]...),
				TagsetRefs:         append([]int64{1}, zeros[1:]...),
				ResourcesRefs:      zeros,
				Intervals:          make([]uint64, len(types)),
				NumPoints:          make([]uint64, len(types)),
				SourceTypeNameRefs: zeros,
				OriginInfoRefs:     zeros,
			},
		}
	}

	tests := []struct {
		name       string
		packedType uint64
		originTags []string
	}{
		{"unset falls back to the configured cardinality", 0x13, []string{"low"}},
		{"none", 0x13 | uint64(pb.TagCardinality_None), []string{}},
		{"low", 0x13 | uint64(pb.TagCardinality_Low), []string{"low"}},
		{"orchestrator", 0x13 | uint64(pb.TagCardinality_Orch), []string{"low", "orch"}},
		{"high", 0x13 | uint64(pb.TagCardinality_High), []string{"low", "orch", "high"}},
		// The flags nibble sits below the cardinality one, setting flagNoIndex
		// must not read back as `none`.
		{"noIndex flag is not a cardinality", 0x113, []string{"low"}},
		// Reserved values are treated as unset rather than rejected.
		{"unknown cardinality falls back", 0x13 | 0xF000, []string{"low"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			it, err := newSeriesIterator(payload([]uint64{tc.packedType}), seriesTestOrigin(t), "default",
				utilstrings.Matcher{})
			require.NoError(t, err)

			require.True(t, it.MoveNext())
			require.Equal(t, tagset.NewCompositeTags(tc.originTags, []string{"ook"}), it.Current().Tags)
			require.False(t, it.MoveNext())
			require.NoError(t, it.err)
		})
	}

	// The legacy dd.internal.card tag no longer selects the cardinality, but it
	// must not reach the intake as a regular tag either.
	t.Run("legacy cardinality tag is dropped", func(t *testing.T) {
		p := payload([]uint64{0x13})
		p.MetricData.DictTagStr = []byte("\x03ook\x15dd.internal.card:high")
		p.MetricData.DictTagsets = []int64{2, 1, 1}

		it, err := newSeriesIterator(p, seriesTestOrigin(t), "default", utilstrings.Matcher{})
		require.NoError(t, err)

		require.True(t, it.MoveNext())
		// Origin tags resolve at the configured default, the `high` in the tag
		// is discarded rather than honoured.
		require.Equal(t, tagset.NewCompositeTags([]string{"low"}, []string{"ook"}), it.Current().Tags)
		require.False(t, it.MoveNext())
		require.NoError(t, it.err)
	})

	t.Run("legacy cardinality tag is dropped from a shared tagset", func(t *testing.T) {
		p := payload([]uint64{0x13, 0x13})
		p.MetricData.DictTagStr = []byte("\x03ook\x15dd.internal.card:high")
		p.MetricData.DictTagsets = []int64{2, 1, 1}

		it, err := newSeriesIterator(p, seriesTestOrigin(t), "default", utilstrings.Matcher{})
		require.NoError(t, err)

		// Both metrics reference the same filtered dictionary entry.
		for range 2 {
			require.True(t, it.MoveNext())
			require.Equal(t, tagset.NewCompositeTags([]string{"low"}, []string{"ook"}), it.Current().Tags)
		}
		require.False(t, it.MoveNext())
		require.NoError(t, it.err)
	})

	t.Run("a tagset of only cardinality tags is emptied", func(t *testing.T) {
		p := payload([]uint64{0x13})
		p.MetricData.DictTagStr = []byte("\x15dd.internal.card:high\x14dd.internal.card:low")
		p.MetricData.DictTagsets = []int64{2, 1, 1}

		it, err := newSeriesIterator(p, seriesTestOrigin(t), "default", utilstrings.Matcher{})
		require.NoError(t, err)

		require.True(t, it.MoveNext())
		require.Equal(t, tagset.NewCompositeTags([]string{"low"}, []string{}), it.Current().Tags)
		require.False(t, it.MoveNext())
		require.NoError(t, it.err)
	})

	t.Run("origin tags are resolved once per cardinality", func(t *testing.T) {
		it, err := newSeriesIterator(
			payload([]uint64{0x13, 0x13 | uint64(pb.TagCardinality_High), 0x13}),
			seriesTestOrigin(t), "default", utilstrings.Matcher{})
		require.NoError(t, err)

		for range 3 {
			require.True(t, it.MoveNext())
		}
		require.False(t, it.MoveNext())
		require.NoError(t, it.err)
		require.Len(t, it.origin.tags, 2)
	})
}
