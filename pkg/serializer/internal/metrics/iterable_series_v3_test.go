// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/compression/impl-noop"
)

func TestPayloadsBuilderV3(t *testing.T) {
	r := assert.New(t)
	const ts = 1756737057.1
	tags := tagset.NewCompositeTags([]string{"foo", "bar"}, []string{"ook", "eek"})
	series := metrics.Series{
		&metrics.Serie{
			Name:    "serie1",
			NoIndex: true,
			Points:  []metrics.Point{{Ts: ts, Value: 0}}},

		&metrics.Serie{
			Name:    "serie2",
			NoIndex: false,
			Tags:    tags,
			Points:  []metrics.Point{{Ts: ts, Value: 2}}},
		&metrics.Serie{
			Name:           "serie3",
			NoIndex:        false,
			Tags:           tags,
			Host:           "test.example",
			SourceTypeName: "stn",
			Source:         metrics.MetricSourceDogstatsd,
			Points:         []metrics.Point{{Ts: ts, Value: 3}}},

		&metrics.Serie{
			Name:           "serie4",
			NoIndex:        true,
			Host:           "test.example",
			Device:         "switch1",
			SourceTypeName: "stn",
			Source:         metrics.MetricSourceCassandra,
			Points:         []metrics.Point{{Ts: ts, Value: 3.14}}},
	}

	pb, err := newPayloadsBuilderV3(1000, 10000, 1000_0000, noopimpl.New())
	require.NoError(t, err)

	for _, s := range series {
		err := pb.writeSerie(s)
		r.NoError(err)
	}
	pb.finishPayload()
	ps := pb.transactionPayloads()
	r.Len(ps, 1)
	r.Equal(205, len(ps[0].GetContent()))
	r.Equal([]byte{
		// metricData
		3<<3 | 2, 0xca, 0x1,

		// names
		1<<3 | 2, 28,
		/* 1 */ 6, 0x73, 0x65, 0x72, 0x69, 0x65, 0x31, // "serie1"
		/* 2 */ 6, 0x73, 0x65, 0x72, 0x69, 0x65, 0x32, // "serie2"
		/* 3 */ 6, 0x73, 0x65, 0x72, 0x69, 0x65, 0x33, // "serie3"
		/* 4 */ 6, 0x73, 0x65, 0x72, 0x69, 0x65, 0x34, // "serie4"

		// tags strings
		2<<3 | 2, 16,
		/* 1 */ 3, 0x66, 0x6f, 0x6f, // "foo"
		/* 2 */ 3, 0x62, 0x61, 0x72, // "bar"
		/* 3 */ 3, 0x6f, 0x6f, 0x6b, // "ook"
		/* 4 */ 3, 0x65, 0x65, 0x6b, // "eek"

		3<<3 | 2, 7,
		/* 1 */ 4, 0x2, 0x2,
		/* 2 */ 6, 0x1, 0x8, 0x02,

		4<<3 | 2, 33,
		/* 1 */ 4, 0x68, 0x6f, 0x73, 0x74,
		/* 2 */ 12, 0x74, 0x65, 0x73, 0x74, 0x2e, 0x65, 0x78, 0x61, 0x6d, 0x70, 0x6c, 0x65,
		/* 3 */ 6, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65,
		/* 4 */ 7, 0x73, 0x77, 0x69, 0x74, 0x63, 0x68, 0x31,

		5<<3 | 2, 2, 0x1, 0x2,
		6<<3 | 2, 3, 0x2, 0x2, 0x4,
		7<<3 | 2, 3, 0x4, 0x4, 0x4,

		8<<3 | 2, 4,
		/* 1 */ 3, 0x73, 0x74, 0x6e,

		9<<3 | 2, 9,
		/* 1 */ 10, 0, 0,
		/* 2 */ 10, 10, 0,
		/* 3 */ 10, 11, 28,

		10<<3 | 2, 6,
		/* 1 */ 0x83, 0x02,
		/* 2 */ 0x13,
		/* 3 */ 0x13,
		/* 4 */ 0xb3, 0x02,

		11<<3 | 2, 4, 2, 2, 2, 2,
		12<<3 | 2, 4, 0, 4, 0, 3,
		13<<3 | 2, 4, 0, 0, 2, 2,
		14<<3 | 2, 4, 0, 0, 0, 0,
		15<<3 | 2, 4, 1, 1, 1, 1,
		16<<3 | 2, 1, 8, 0xc2, 0xb8, 0xad, 0x8b, 0xd, 0, 0, 0,
		17<<3 | 2, 1, 2, 4, 6,
		19<<3 | 2, 1, 8, 0x1f, 0x85, 0xeb, 0x51, 0xb8, 0x1e, 0x9, 0x40,
		23<<3 | 2, 1, 4, 0, 0, 2, 0,
		24<<3 | 2, 1, 4, 2, 0, 2, 2,
	}, ps[0].GetContent())
}

func BenchmarkPaylodsBuilderV3(b *testing.B) {
	const ts = 1756737057.1
	serie := &metrics.Serie{
		Name:   "serie1",
		Tags:   tagset.NewCompositeTags([]string{"foo", "bar"}, []string{"ook", "eek"}),
		Points: []metrics.Point{{Ts: ts, Value: 3.14}}}

	pb, err := newPayloadsBuilderV3(500_000, 2_000_000, 10_000, noopimpl.New())
	if err != nil {
		b.Fatalf("new: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err = pb.writeSerie(serie)
		pb.payloads = pb.payloads[:]
	}

	if err != nil {
		b.Fatalf("writeSerie: %v", err)
	}
}

func TestPayloadBuildersV3_Split(t *testing.T) {
	r := assert.New(t)
	const ts = 1756737057.1
	tags := tagset.NewCompositeTags([]string{"foo", "bar"}, []string{"ook", "eek"})
	series := metrics.Series{
		&metrics.Serie{
			Name:    "serie1",
			NoIndex: true,
			Points:  []metrics.Point{{Ts: ts, Value: 0}}},

		&metrics.Serie{
			Name:    "serie2",
			NoIndex: false,
			Tags:    tags,
			Points:  []metrics.Point{{Ts: ts, Value: 2}}},
		&metrics.Serie{
			Name:           "serie3",
			NoIndex:        false,
			Tags:           tags,
			Host:           "test.example",
			SourceTypeName: "stn",
			Source:         metrics.MetricSourceDogstatsd,
			Points:         []metrics.Point{{Ts: ts, Value: 3}}},

		&metrics.Serie{
			Name:           "serie4",
			NoIndex:        true,
			Host:           "test.example",
			Device:         "switch1",
			SourceTypeName: "stn",
			Source:         metrics.MetricSourceCassandra,
			Points:         []metrics.Point{{Ts: ts, Value: 3.14}}},
	}

	pb, err := newPayloadsBuilderV3(180, 10000, 1000_0000, noopimpl.New())
	require.NoError(t, err)

	r.NoError(pb.writeSerie(series[0]))
	r.NoError(pb.writeSerie(series[1]))
	r.NoError(pb.writeSerie(series[2]))
	r.NoError(pb.writeSerie(series[3]))
	pb.finishPayload()
	payloads := pb.transactionPayloads()
	r.Len(payloads, 3)

	r.Equal(2, payloads[0].GetPointCount())
	r.Less(len(payloads[1].GetContent()), 180)
	r.Equal(1, payloads[1].GetPointCount())
	r.Equal(1, payloads[2].GetPointCount())
	r.NotContains("foo", payloads[1].GetContent())
	r.NotContains("foo", payloads[2].GetContent())
}

func TestPayloadsBuilderV3_SplitTooBig(t *testing.T) {
	// Test that payload contains all necessary data info after an item was dropped.

	r := assert.New(t)
	const ts = 1756737057.1
	tags := tagset.NewCompositeTags([]string{"foo", "bar"}, []string{"ook", "eek"})
	series := metrics.Series{
		&metrics.Serie{
			Name:           "serie1",
			Tags:           tags,
			SourceTypeName: "System",
			Points:         slices.Repeat([]metrics.Point{{Ts: ts, Value: 0}}, 10000),
		},
		&metrics.Serie{
			Name:           "serie1",
			Tags:           tags,
			SourceTypeName: "System",
			Points:         []metrics.Point{{Ts: ts, Value: 2}},
		},
	}

	pb, err := newPayloadsBuilderV3(180, 10000, 1000_0000, noopimpl.New())
	require.NoError(t, err)

	r.NoError(pb.writeSerie(series[0]))
	r.NoError(pb.writeSerie(series[1]))
	pb.finishPayload()
	payloads := pb.transactionPayloads()
	r.Len(payloads, 1)

	r.Equal(1, payloads[0].GetPointCount())
	r.Contains(string(payloads[0].GetContent()), "serie1")
	r.Contains(string(payloads[0].GetContent()), "foo")
}

func TestPayloadsBuilderV3_PointsLimit(t *testing.T) {
	r := assert.New(t)
	const ts = 1756737057.1

	series := metrics.Series{
		&metrics.Serie{
			Name:   "serie1",
			Points: slices.Repeat([]metrics.Point{{Ts: ts, Value: 0}}, 3),
		},
		&metrics.Serie{
			Name:   "serie2",
			Points: slices.Repeat([]metrics.Point{{Ts: ts, Value: 2}}, 8),
		},
		&metrics.Serie{
			Name:   "serie3",
			Points: slices.Repeat([]metrics.Point{{Ts: ts, Value: 3}}, 20),
		},
		&metrics.Serie{
			Name:   "serie4",
			Points: []metrics.Point{{Ts: ts, Value: 3.14}},
		},
	}

	pb, err := newPayloadsBuilderV3(1000_0000, 1000_000, 10, noopimpl.New())
	require.NoError(t, err)

	r.NoError(pb.writeSerie(series[0]))
	r.NoError(pb.writeSerie(series[1]))
	r.NoError(pb.writeSerie(series[2]))
	r.NoError(pb.writeSerie(series[3]))
	pb.finishPayload()
	payloads := pb.transactionPayloads()
	r.Len(payloads, 2)
	r.Equal(3, payloads[0].GetPointCount())
	r.Equal(9, payloads[1].GetPointCount())
}

func TestPaylodsBuilderV3_ReservedSpace(t *testing.T) {
	const ts = 1756737057.1
	serie := &metrics.Serie{
		Name:   "serie1",
		Tags:   tagset.NewCompositeTags([]string{"foo", "bar"}, []string{"ook", "eek"}),
		Points: []metrics.Point{{Ts: ts, Value: 3.14}}}

	pb, err := newPayloadsBuilderV3(500, 2_000, 10_000, noopimpl.New())
	require.NoError(t, err)
	for len(pb.payloads) == 0 {
		require.NoError(t, pb.writeSerie(serie))
	}
	require.NoError(t, pb.finishPayload())
}

func TestPaylodsBuilderV3_Tags(t *testing.T) {
	pb, err := newPayloadsBuilderV3(1000, 1000, 1000, noopimpl.New())
	require.NoError(t, err)
	for _, tags := range [][2][]string{{nil, nil}, {{"t1"}, nil}, {nil, {"t2"}}, {{"t3"}, {"t4"}}} {
		ct := tagset.NewCompositeTags(tags[0], tags[1])
		require.NoError(t, pb.writeSerie(&metrics.Serie{Name: "a", Tags: ct, Points: []metrics.Point{{1, 1}}}))
	}
	require.NoError(t, pb.finishPayload())
	require.Len(t, pb.payloads, 1)
	payload := string(pb.payloads[0].GetContent())
	require.Contains(t, payload, "t1")
	require.Contains(t, payload, "t2")
	require.Contains(t, payload, "t3")
	require.Contains(t, payload, "t4")
}
