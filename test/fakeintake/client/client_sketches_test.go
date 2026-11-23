// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	metricspb "github.com/DataDog/agent-payload/v5/gogen"
	intake_v3 "github.com/DataDog/agent-payload/v5/metrics/intake_v3"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

// newV3SketchesData returns an encoded API v3 payload with a DDSketch series
// "dist.v3" and an explicit bucket histogram "hist".
func newV3SketchesData(t *testing.T) []byte {
	t.Helper()
	var names []byte // dictionary: each entry is a uvarint length followed by the string
	for _, name := range []string{"dist.v3", "hist"} {
		names = binary.AppendUvarint(names, uint64(len(name)))
		names = append(names, name...)
	}

	md := &intake_v3.MetricData{
		DictNameStr: names,
		Types: []uint64{
			uint64(intake_v3.MetricType_DDSketch) | uint64(intake_v3.ValueType_Sint64),
			uint64(intake_v3.MetricType_ExplicitHistogram) | uint64(intake_v3.ValueType_Zero),
		},
		NameRefs:           []int64{1, 1},
		TagsetRefs:         []int64{0, 0},
		ResourcesRefs:      []int64{0, 0},
		SourceTypeNameRefs: []int64{0, 0},
		OriginInfoRefs:     []int64{0, 0},
		Intervals:          []uint64{0, 0},
		NumPoints:          []uint64{1, 1},
		Timestamps:         []int64{1000, 0},
		ValsSint64:         []int64{2, 1, 1, 2}, // DDSketch sum, min, max, cnt
		NumBins:            []uint64{1, 1},
		BinKeys:            []int32{3},
		BinCnts:            []uint64{2, 5},
		PointFlags:         []uint32{0}, // histogram without sum, min and max
		ValsUint64:         []uint64{5}, // histogram count
	}

	data, err := proto.Marshal(&intake_v3.Payload{MetricData: md})
	require.NoError(t, err)
	return data
}

func TestClientSketchesV3(t *testing.T) {
	v2, err := (&metricspb.SketchPayload{Sketches: []metricspb.SketchPayload_Sketch{{Metric: "dist.v2"}}}).Marshal()
	require.NoError(t, err)

	ts := newSeriesServer(map[string][]byte{
		sketchesEndpoint:   newRawPayloadsResponse(t, "application/x-protobuf", v2),
		sketchesV3Endpoint: newRawPayloadsResponse(t, "application/x-protobuf", newV3SketchesData(t)),
	})
	defer ts.Close()
	client := NewClient(ts.URL)

	t.Run("FilterSketches merges v2 and v3", func(t *testing.T) {
		v2Sketches, err := client.FilterSketches("dist.v2")
		require.NoError(t, err)
		assert.Len(t, v2Sketches, 1)

		v3Sketches, err := client.FilterSketches("dist.v3")
		require.NoError(t, err)
		require.Len(t, v3Sketches, 1)
		require.Len(t, v3Sketches[0].Dogsketches, 1)
		assert.Equal(t, []int32{3}, v3Sketches[0].Dogsketches[0].K)
		assert.Equal(t, []uint32{2}, v3Sketches[0].Dogsketches[0].N)

		names, err := client.GetSketchNames()
		require.NoError(t, err)
		assert.Equal(t, []string{"dist.v2", "dist.v3"}, names)
	})

	t.Run("FilterHistograms", func(t *testing.T) {
		histograms, err := client.FilterHistograms("hist")
		require.NoError(t, err)
		require.Len(t, histograms, 1)
		assert.Equal(t, []*aggregator.ExplicitHistogramPoint{{Timestamp: 1000, Count: 5, Bounds: []float64{}, Counts: []uint64{5}}}, histograms[0].Explicit)

		names, err := client.GetHistogramNames()
		require.NoError(t, err)
		assert.Equal(t, []string{"hist"}, names)
	})
}
