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
)

func TestIterator(t *testing.T) {
	payload := &pb.Payload{
		MetricData: &pb.MetricData{
			DictNameStr:      []byte("\x03foo\x03bar\x03baz"),
			DictTagsStr:      []byte("\x03ook\x15dd.internal.card:none\x15dd.internal.card:high"),
			DictTagsets:      []int64{2, 1, 1, 2, 1, 2, 1, 1},
			DictResourceStr:  []byte("\x04host"),
			DictResourceLen:  []int64{2},
			DictResourceType: []int64{1, 0},
			DictResourceName: []int64{0, 1},

			Types:           []uint64{0x11, 0x12, 0x13},
			Names:           []int64{1, 1, 1},
			Tags:            []int64{1, 1, 1},
			Resources:       []int64{0, 1, -1},
			Intervals:       []uint64{10, 10, 10},
			NumPoints:       []uint64{1, 2, 2},
			SourceTypeNames: []int64{0, 0, 0},
			OriginInfos:     []int64{0, 0, 0},
			Timestamps:      []int64{1000, 0, 10, -10, 10},
			ValsSint64:      []int64{5, 6, 7, 8, 9},
		},
	}

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

	it, err := newSeriesIterator(payload, origin, "default")
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
