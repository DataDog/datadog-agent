// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/compression/impl-noop"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

func TestPayloadsBuilderV3(t *testing.T) {
	r := assert.New(t)
	const ts = 1756737057.1
	tags := tagset.NewCompositeTags([]string{"foo", "bar"}, []string{"ook", "eek"})
	series := metrics.Series{
		&metrics.Serie{
			Name:     "serie1",
			NoIndex:  true,
			Points:   []metrics.Point{{Ts: ts, Value: 0}},
			Interval: 10,
		},
		&metrics.Serie{
			Name:     "serie2",
			NoIndex:  false,
			Tags:     tags,
			Points:   []metrics.Point{{Ts: ts, Value: 2}},
			Interval: 10,
		},
		&metrics.Serie{
			Name:           "serie3",
			NoIndex:        false,
			Tags:           tags,
			Host:           "test.example",
			SourceTypeName: "stn",
			Source:         metrics.MetricSourceDogstatsd,
			Points:         []metrics.Point{{Ts: ts, Value: 3}},
			Interval:       0,
		},
		&metrics.Serie{
			Name:           "serie4",
			NoIndex:        true,
			Host:           "test.example",
			Device:         "switch1",
			SourceTypeName: "stn",
			Source:         metrics.MetricSourceCassandra,
			Points:         []metrics.Point{{Ts: ts, Value: 3.14}},
			Interval:       0,
		},
	}

	pipelineConfig := PipelineConfig{
		Filter: AllowAllFilter{},
		V3:     true,
	}
	pipelineContext := &PipelineContext{}

	pb, err := newPayloadsBuilderV3(1000, 10000, 1000_0000, noopimpl.New(), pipelineConfig, pipelineContext)
	require.NoError(t, err)

	for _, s := range series {
		err := pb.writeSerie(s)
		r.NoError(err)
	}

	r.Equal(uint64(1), pb.stats.valuesZero)
	r.Equal(uint64(2), pb.stats.valuesSint64)
	r.Equal(uint64(0), pb.stats.valuesFloat32)
	r.Equal(uint64(1), pb.stats.valuesFloat64)

	pb.finishPayload()
	ps := pipelineContext.payloads
	r.Len(ps, 1)
	r.Equal(210, len(ps[0].GetContent()))
	r.Equal([]byte{
		// metricData
		3<<3 | 2, 0xcf, 0x1,

		// names
		1<<3 | 2, 28,
		/* 1 */ 6, 0x73, 0x65, 0x72, 0x69, 0x65, 0x31, // "serie1"
		/* 2 */ 6, 0x73, 0x65, 0x72, 0x69, 0x65, 0x32, // "serie2"
		/* 3 */ 6, 0x73, 0x65, 0x72, 0x69, 0x65, 0x33, // "serie3"
		/* 4 */ 6, 0x73, 0x65, 0x72, 0x69, 0x65, 0x34, // "serie4"

		// tags strings
		2<<3 | 2, 16,
		/* 2 */ 3, 0x62, 0x61, 0x72, // "bar"
		/* 1 */ 3, 0x66, 0x6f, 0x6f, // "foo"
		/* 4 */ 3, 0x65, 0x65, 0x6b, // "eek"
		/* 3 */ 3, 0x6f, 0x6f, 0x6b, // "ook"

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

		9<<3 | 2, 16,
		/* 1 */ 10, 0, 0, 9,
		/* 2 */ 10, 0, 0, 0,
		/* 3 */ 10, 10, 0, 0,
		/* 4 */ 10, 11, 28, 9,

		10<<3 | 2, 4,
		/* 1 */ 0x03,
		/* 2 */ 0x13,
		/* 3 */ 0x13,
		/* 4 */ 0x33,

		11<<3 | 2, 4, 2, 2, 2, 2,
		12<<3 | 2, 4, 0, 4, 0, 3,
		13<<3 | 2, 4, 0, 0, 2, 2,
		14<<3 | 2, 4, 10, 10, 0, 0,
		15<<3 | 2, 4, 1, 1, 1, 1,
		16<<3 | 2, 1, 8, 0xc2, 0xb8, 0xad, 0x8b, 0xd, 0, 0, 0,
		17<<3 | 2, 1, 2, 4, 6,
		19<<3 | 2, 1, 8, 0x1f, 0x85, 0xeb, 0x51, 0xb8, 0x1e, 0x9, 0x40,
		23<<3 | 2, 1, 4, 0, 0, 2, 0,
		24<<3 | 2, 1, 4, 2, 2, 2, 2,
	}, ps[0].GetContent())
}

func BenchmarkPayloadsBuilderV3(b *testing.B) {
	const ts = 1756737057.1
	serie := &metrics.Serie{
		Name:   "serie1",
		Tags:   tagset.NewCompositeTags([]string{"foo", "bar"}, []string{"ook", "eek"}),
		Points: []metrics.Point{{Ts: ts, Value: 3.14}}}

	pipelineConfig := PipelineConfig{
		Filter: AllowAllFilter{},
		V3:     true,
	}
	pipelineContext := &PipelineContext{}

	pb, err := newPayloadsBuilderV3(500_000, 2_000_000, 10_000, noopimpl.New(), pipelineConfig, pipelineContext)
	if err != nil {
		b.Fatalf("new: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err = pb.writeSerie(serie)
		pipelineContext.payloads = pipelineContext.payloads[:]
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
	pipelineConfig := PipelineConfig{
		Filter: AllowAllFilter{},
		V3:     true,
	}
	pipelineContext := &PipelineContext{}
	pb, err := newPayloadsBuilderV3(180, 10000, 1000_0000, noopimpl.New(), pipelineConfig, pipelineContext)
	require.NoError(t, err)

	r.NoError(pb.writeSerie(series[0]))
	r.NoError(pb.writeSerie(series[1]))
	r.NoError(pb.writeSerie(series[2]))
	r.NoError(pb.writeSerie(series[3]))
	pb.finishPayload()
	payloads := pipelineContext.payloads
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
	pipelineConfig := PipelineConfig{
		Filter: AllowAllFilter{},
		V3:     true,
	}
	pipelineContext := &PipelineContext{}
	pb, err := newPayloadsBuilderV3(180, 10000, 1000_0000, noopimpl.New(), pipelineConfig, pipelineContext)
	require.NoError(t, err)

	r.NoError(pb.writeSerie(series[0]))
	r.NoError(pb.writeSerie(series[1]))
	pb.finishPayload()
	payloads := pipelineContext.payloads
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

	pipelineConfig := PipelineConfig{
		Filter: AllowAllFilter{},
		V3:     true,
	}
	pipelineContext := &PipelineContext{}
	pb, err := newPayloadsBuilderV3(1000_0000, 1000_000, 10, noopimpl.New(), pipelineConfig, pipelineContext)
	require.NoError(t, err)

	r.NoError(pb.writeSerie(series[0]))
	r.NoError(pb.writeSerie(series[1]))
	r.NoError(pb.writeSerie(series[2]))
	r.NoError(pb.writeSerie(series[3]))
	pb.finishPayload()
	payloads := pipelineContext.payloads
	r.Len(payloads, 2)
	r.Equal(3, payloads[0].GetPointCount())
	r.Equal(9, payloads[1].GetPointCount())
}

func TestPayloadsBuilderV3_ReservedSpace(t *testing.T) {
	const ts = 1756737057.1
	serie := &metrics.Serie{
		Name:   "serie1",
		Tags:   tagset.NewCompositeTags([]string{"foo", "bar"}, []string{"ook", "eek"}),
		Points: []metrics.Point{{Ts: ts, Value: 3.14}}}

	pipelineConfig := PipelineConfig{
		Filter: AllowAllFilter{},
		V3:     true,
	}
	pipelineContext := &PipelineContext{}
	pb, err := newPayloadsBuilderV3(500, 2_000, 10_000, noopimpl.New(), pipelineConfig, pipelineContext)
	require.NoError(t, err)
	for len(pipelineContext.payloads) == 0 {
		require.NoError(t, pb.writeSerie(serie))
	}
	require.NoError(t, pb.finishPayload())
}

func TestPayloadsBuilderV3_Tags(t *testing.T) {
	pipelineConfig := PipelineConfig{
		Filter: AllowAllFilter{},
		V3:     true,
	}
	pipelineContext := &PipelineContext{}
	pb, err := newPayloadsBuilderV3(1000, 1000, 1000, noopimpl.New(), pipelineConfig, pipelineContext)
	require.NoError(t, err)
	for _, tags := range [][2][]string{{nil, nil}, {{"t1"}, nil}, {nil, {"t2"}}, {{"t3"}, {"t4"}}} {
		ct := tagset.NewCompositeTags(tags[0], tags[1])
		serie := &metrics.Serie{
			Name: "a",
			Tags: ct,
			Points: []metrics.Point{
				{
					Ts:    1,
					Value: 1,
				},
			},
		}
		require.NoError(t, pb.writeSerie(serie))
	}
	require.NoError(t, pb.finishPayload())
	require.Len(t, pipelineContext.payloads, 1)
	payload := string(pipelineContext.payloads[0].GetContent())
	require.Contains(t, payload, "t1")
	require.Contains(t, payload, "t2")
	require.Contains(t, payload, "t3")
	require.Contains(t, payload, "t4")
}

func TestPayloadsBuilderV3_Sketch(t *testing.T) {
	r := assert.New(t)
	const ts = 1756737057

	tags := tagset.NewCompositeTags([]string{"foo", "bar"}, []string{"ook", "eek"})
	sketches := metrics.SketchSeriesList{
		{
			Name:    "serie1",
			NoIndex: true,
			Points:  pointsOf(ts, 0, 0),
		}, {
			Name:    "serie2",
			NoIndex: false,
			Tags:    tags,
			Points:  pointsOf(ts, -1, 0, 1),
		}, {
			Name:    "serie3",
			NoIndex: false,
			Tags:    tags,
			Host:    "test.example",
			Source:  metrics.MetricSourceDogstatsd,
			Points:  pointsOf(ts, 0.5, -0.5),
		}, {
			Name:    "serie4",
			NoIndex: true,
			Host:    "test.example",
			Source:  metrics.MetricSourceCassandra,
			Points:  pointsOf(ts, 3.14159, 2.71),
		},
	}

	pipelineConfig := PipelineConfig{
		Filter: AllowAllFilter{},
		V3:     true,
	}
	pipelineContext := &PipelineContext{}

	pb, err := newPayloadsBuilderV3(1000, 10000, 1000_0000, noopimpl.New(), pipelineConfig, pipelineContext)
	require.NoError(t, err)

	for _, sk := range sketches {
		r.NoError(pb.writeSketch(sk))
	}
	r.Equal(uint64(3), pb.stats.valuesZero)
	r.Equal(uint64(7), pb.stats.valuesSint64)
	r.Equal(uint64(3), pb.stats.valuesFloat32)
	r.Equal(uint64(3), pb.stats.valuesFloat64)

	r.NoError(pb.finishPayload())
	r.NotEmpty(pipelineContext.payloads)

	r.Equal([]byte{
		// metricData
		3<<3 | 2, 252, 1,

		// dictNameStr
		1<<3 | 2, 28,
		/* 1 */ 6, 0x73, 0x65, 0x72, 0x69, 0x65, 0x31, // "serie1"
		/* 2 */ 6, 0x73, 0x65, 0x72, 0x69, 0x65, 0x32, // "serie2"
		/* 3 */ 6, 0x73, 0x65, 0x72, 0x69, 0x65, 0x33, // "serie3"
		/* 4 */ 6, 0x73, 0x65, 0x72, 0x69, 0x65, 0x34, // "serie4"

		// dictTagsStr
		2<<3 | 2, 16,
		/* 2 */ 3, 0x62, 0x61, 0x72, // "bar"
		/* 1 */ 3, 0x66, 0x6f, 0x6f, // "foo"
		/* 4 */ 3, 0x65, 0x65, 0x6b, // "eek"
		/* 3 */ 3, 0x6f, 0x6f, 0x6b, // "ook"

		// dictTagsets
		3<<3 | 2, 7,
		/* 1 */ 4, 0x2, 0x2,
		/* 2 */ 6, 0x1, 0x8, 0x02,

		// dictResourceStr
		4<<3 | 2, 18,
		/* 1 */ 4, 0x68, 0x6f, 0x73, 0x74,
		/* 2 */ 12, 0x74, 0x65, 0x73, 0x74, 0x2e, 0x65, 0x78, 0x61, 0x6d, 0x70, 0x6c, 0x65,
		// dictResourcesLen
		5<<3 | 2, 1, 0x1,
		// dictResourceType
		6<<3 | 2, 1, 0x2,
		// dictResourceName
		7<<3 | 2, 1, 0x4,

		// dictOrigin
		9<<3 | 2, 16,
		/* 1 */ 10, 0, 0, 9,
		/* 2 */ 10, 0, 0, 0,
		/* 3 */ 10, 10, 0, 0,
		/* 4 */ 10, 11, 28, 9,

		// type
		10<<3 | 2, 4, 0x04, 0x14, 0x24, 0x34,
		// nameRef
		11<<3 | 2, 4, 2, 2, 2, 2,
		// tagsRef
		12<<3 | 2, 4, 0, 4, 0, 3,
		// resourcesRef
		13<<3 | 2, 4, 0, 0, 2, 0,
		// interval
		14<<3 | 2, 4, 0, 0, 0, 0,
		// numPoints
		15<<3 | 2, 4, 1, 1, 1, 1,
		// timestamp
		16<<3 | 2, 1, 8, 0xc2, 0xb8, 0xad, 0x8b, 0xd, 0, 0, 0,
		// valueSint64
		17<<3 | 2, 1, 7,
		4,          // sketch 0 cnt
		0, 1, 2, 6, // sketch 1 sum, min, max, cnt
		4, // sketch 2 cnt
		4, // sketch 3 cnt

		// valueFloat32
		18<<3 | 2, 1, 12, 0, 0, 0, 0, 0, 0, 0, 191, 0, 0, 0, 63,
		// valueFloat64
		19<<3 | 2, 1, 24,
		14, 103, 126, 53, 7, 104, 23, 64, // list(pack('<ddd', 3.14159 + 2.71, 2.71, 3.14159))
		174, 71, 225, 122, 20, 174, 5, 64,
		110, 134, 27, 240, 249, 33, 9, 64,

		// sketchNumBins
		20<<3 | 2, 1, 4, 1, 3, 2, 2,
		// sketchBinKeys
		21<<3 | 2, 1, 14,
		0,
		243, 20, 244, 20, 244, 20,
		153, 20, 180, 40,
		244, 21, 20,

		// sketchBinCnts
		22<<3 | 2, 1, 8, 2, 1, 1, 1, 1, 1, 1, 1,
		// sourceTypeNameRef
		23<<3 | 2, 1, 4, 0, 0, 0, 0,
		// originRef
		24<<3 | 2, 1, 4, 2, 2, 2, 2,
	}, pipelineContext.payloads[0].GetContent())
}

func pointsOf(ts int64, v ...float64) []metrics.SketchPoint {
	s := &quantile.Sketch{}
	s.InsertMany(quantile.Default(), v)
	return []metrics.SketchPoint{{Ts: ts, Sketch: s}}
}

func TestValueEncoding(t *testing.T) {
	values := []float64{
		// cases for zero
		0,
		-0,
		// cases for int24
		-1,
		1,
		float64(-1 << 24),
		float64(1 << 24),
		// cases for int48
		float64(-1<<24 - 1),
		float64(1<<24 + 1),
		float64(-1 << 48),
		float64(1<<48 - 1),
		// cases for float32
		-0.5,
		0.5,
		// cases for float64
		float64(-1<<48 - 1),
		float64(1 << 48),
		-3.14,
		3.14,
	}

	for _, value1 := range values {
		for _, value2 := range values {
			ty := pointKindZero.unionOf(value1).unionOf(value2).toValueType()
			fmt.Printf("v1=%v, v2=%v, type=%x\n", value1, value2, ty)
			switch ty {
			case valueZero:
				require.Equal(t, value1, 0.0)
				require.Equal(t, value2, 0.0)
			case valueSint64:
				require.Equal(t, value1, float64(int64(value1)))
				require.Equal(t, value2, float64(int64(value2)))
			case valueFloat32:
				require.Equal(t, value1, float64(float32(value1)))
				require.Equal(t, value2, float64(float32(value2)))
			case valueFloat64:
				// no conversion
			}
		}
	}
}
