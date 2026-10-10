// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/protocolbuffers/protoscope"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/compression/impl-noop"
)

type testExplicitPoint struct {
	ts               int64
	haveMin, haveMax bool
	min, max, sum    float64
	cnt              uint64
	bounds           []float64
	counts           []uint64
}

type testExplicitPoints []testExplicitPoint

func (p testExplicitPoints) GetOtelExplicitHistogramPoint(i int) (int64, bool, bool, float64, float64, float64, uint64, []float64, []uint64) {
	x := p[i]
	return x.ts, x.haveMin, x.haveMax, x.min, x.max, x.sum, x.cnt, x.bounds, x.counts
}

type testExponentialPoint struct {
	ts                      int64
	haveMin, haveMax        bool
	min, max, sum, zeroThr  float64
	cnt, zeroCnt            uint64
	scale, posOffs, negOffs int32
	posCnt, negCnt          []uint64
}

type testExponentialPoints []testExponentialPoint

func (p testExponentialPoints) GetOtelExponentialHistogramPoint(i int) (int64, bool, bool, float64, float64, float64, float64, uint64, uint64, int32, int32, int32, []uint64, []uint64) {
	x := p[i]
	return x.ts, x.haveMin, x.haveMax, x.min, x.max, x.sum, x.zeroThr, x.cnt, x.zeroCnt, x.scale, x.posOffs, x.negOffs, x.posCnt, x.negCnt
}

func newTestHistogramBuilder(t *testing.T, maxPoints int) (*payloadsBuilderV3, *PipelineContext) {
	pipelineConfig := PipelineConfig{
		Filter: AllowAllFilter{},
		V3:     true,
	}
	pipelineContext := &PipelineContext{}
	pb, err := newPayloadsBuilderV3(100_000, 1_000_000, maxPoints, noopimpl.New(), pipelineConfig, pipelineContext)
	require.NoError(t, err)
	return pb, pipelineContext
}

// requireProtoscope finishes the payload and compares it with the protoscope
// definition. On mismatch, both sides are disassembled for a readable diff.
func requireProtoscope(t *testing.T, pb *payloadsBuilderV3, pipelineContext *PipelineContext, expected string) {
	t.Helper()
	require.NoError(t, pb.finishPayload())
	require.Len(t, pipelineContext.payloads, 1)
	actual := pipelineContext.payloads[0].GetContent()

	want, err := protoscope.NewScanner(expected).Exec()
	require.NoError(t, err)
	require.Equal(t, protoscope.Write(want, protoscope.WriterOptions{}), protoscope.Write(actual, protoscope.WriterOptions{}))
	require.Equal(t, want, actual)
}

// Example from the ExplicitHistogram documentation in the API v3 payload.proto.
func TestPayloadsBuilderV3_ExplicitHistogramProtoExample(t *testing.T) {
	pb, pipelineContext := newTestHistogramBuilder(t, 1000)

	require.NoError(t, pb.WriteOtelExplicitHistogram(
		metrics.DistributionMetadata{Name: "h"}, 1,
		testExplicitPoints{{ts: 1000, sum: 30, cnt: 5, bounds: []float64{10}, counts: []uint64{2, 3}}},
	))

	requireProtoscope(t, pb, pipelineContext, `3: {
		1: {1 "h"}     # dictNameStr
		9: {10 0 0}    # dictOriginInfo
		10: {0x16}     # types: ExplicitHistogram | Sint64
		11: {1z}       # nameRefs
		12: {0z}       # tagsetRefs
		13: {0z}       # resourcesRefs
		14: {0}        # intervals
		15: {1}        # numPoints
		16: {1000z}    # timestamps
		17: {30z 10z}  # valsSint64: sum, bounds
		20: {2}        # numBins
		22: {2 3}      # binCnts
		23: {0z}       # sourceTypeNameRefs
		24: {1z}       # originInfoRefs
		30: {0}        # pointFlags
		31: {5}        # valsUint64: count
	}`)
}

// Example from the ExponentialHistogram documentation in the API v3 payload.proto.
func TestPayloadsBuilderV3_ExponentialHistogramProtoExample(t *testing.T) {
	pb, pipelineContext := newTestHistogramBuilder(t, 1000)

	require.NoError(t, pb.WriteOtelExponentialHistogram(
		metrics.DistributionMetadata{Name: "h"}, 1,
		testExponentialPoints{{
			ts:      1000,
			sum:     18,
			cnt:     10,
			zeroCnt: 1,
			scale:   0,
			posOffs: 0,
			posCnt:  []uint64{2, 0, 3},
			negOffs: -1,
			negCnt:  []uint64{4},
		}},
	))

	requireProtoscope(t, pb, pipelineContext, `3: {
		1: {1 "h"}
		9: {10 0 0}
		10: {0x17}               # types: ExponentialHistogram | Sint64
		11: {1z}
		12: {0z}
		13: {0z}
		14: {0}
		15: {1}
		16: {1000z}
		17: {0z 0z -1z 18z 0z}   # valsSint64: scale, posOffset, negOffset, sum, zeroThreshold
		20: {3 1}                # numBins: positive, negative
		22: {2 0 3 4}            # binCnts: positive, then negative
		23: {0z}
		24: {1z}
		30: {0}                  # pointFlags
		31: {10 1}               # valsUint64: count, zeroCount
	}`)
}

func TestPayloadsBuilderV3_ExplicitHistogramMinMaxFloat(t *testing.T) {
	pb, pipelineContext := newTestHistogramBuilder(t, 1000)

	require.NoError(t, pb.WriteOtelExplicitHistogram(
		metrics.DistributionMetadata{Name: "h", NoIndex: true}, 3,
		testExplicitPoints{
			{ts: 1000, haveMin: true, min: 0.5, sum: 1.5, cnt: 3, bounds: []float64{1, 2}, counts: []uint64{1, 2, 0}},
			// summary-only point
			{ts: 1010, haveMax: true, max: 3, sum: 2.5, cnt: 1},
			{ts: 1020, haveMin: true, haveMax: true, min: -1, max: 1, sum: 0, cnt: math.MaxUint64, counts: []uint64{math.MaxUint64}},
		},
	))
	require.Equal(t, uint64(9), pb.stats.valuesFloat32)

	requireProtoscope(t, pb, pipelineContext, `3: {
		1: {1 "h"}
		9: {10 0 0}
		10: {0x126}              # types: flagNoIndex | Float32 | ExplicitHistogram
		11: {1z}
		12: {0z}
		13: {0z}
		14: {0}
		15: {3}
		16: {1000z 10z 10z}
		18: {                    # valsFloat32
			1.5i32 0.5i32 1.0i32 2.0i32   # sum, min, bounds
			2.5i32 3.0i32                 # sum, max
			0.0i32 -1.0i32 1.0i32         # sum, min, max
		}
		20: {3 0 1}
		22: {1 2 0 18446744073709551615}
		23: {0z}
		24: {1z}
		30: {1 2 3}              # pointFlags: min, max, min|max
		31: {3 1 18446744073709551615}
	}`)
}

func TestPayloadsBuilderV3_ExponentialHistogramFloat64(t *testing.T) {
	pb, pipelineContext := newTestHistogramBuilder(t, 1000)

	require.NoError(t, pb.WriteOtelExponentialHistogram(
		metrics.DistributionMetadata{Name: "h"}, 2,
		testExponentialPoints{
			{ts: 1000, haveMin: true, haveMax: true, min: 0.1, max: 7, sum: 12.3, zeroThr: 1e-9, cnt: 4, scale: -3, posOffs: math.MaxInt32, posCnt: []uint64{4}},
			{ts: 1010, sum: 0, cnt: 2, zeroCnt: 2, scale: 20, posOffs: math.MinInt32, negOffs: 5},
		},
	))
	require.Equal(t, uint64(6), pb.stats.valuesFloat64)

	requireProtoscope(t, pb, pipelineContext, `3: {
		1: {1 "h"}
		9: {10 0 0}
		10: {0x37}               # types: Float64 | ExponentialHistogram
		11: {1z}
		12: {0z}
		13: {0z}
		14: {0}
		15: {2}
		16: {1000z 10z}
		17: {                    # valsSint64: integer headers only
			-3z 2147483647z 0z
			20z -2147483648z 5z
		}
		19: {                    # valsFloat64: sum, min?, max?, zeroThreshold
			12.3 0.1 7.0 1.0e-9
			0.0 0.0
		}
		20: {1 0 0 0}
		22: {4}
		23: {0z}
		24: {1z}
		30: {3 0}
		31: {4 0 2 2}
	}`)
}

func TestPayloadsBuilderV3_HistogramZeroValues(t *testing.T) {
	pb, pipelineContext := newTestHistogramBuilder(t, 1000)

	require.NoError(t, pb.WriteOtelExplicitHistogram(
		metrics.DistributionMetadata{Name: "e"}, 1,
		testExplicitPoints{{ts: 1000, haveMin: true, cnt: 1, counts: []uint64{1}}},
	))
	require.NoError(t, pb.WriteOtelExponentialHistogram(
		metrics.DistributionMetadata{Name: "x"}, 1,
		testExponentialPoints{{ts: 1000, haveMax: true, cnt: 1, zeroCnt: 1, scale: 1, posOffs: 2, negOffs: 3}},
	))
	// explicit: sum, min; exponential: sum, max, zero threshold
	require.Equal(t, uint64(5), pb.stats.valuesZero)

	requireProtoscope(t, pb, pipelineContext, `3: {
		1: {1 "e" 1 "x"}
		9: {10 0 0}
		10: {0x06 0x07}          # types: Zero value type
		11: {1z 1z}
		12: {0z 0z}
		13: {0z 0z}
		14: {0 0}
		15: {1 1}
		16: {1000z 0z}
		17: {1z 2z 3z}           # numeric values omitted, exponential header kept
		20: {1 0 0}
		22: {1}
		23: {0z 0z}
		24: {1z 0z}
		30: {1 2}
		31: {1 1 1}
	}`)
}

func TestPayloadsBuilderV3_HistogramPointsLimit(t *testing.T) {
	pb, pipelineContext := newTestHistogramBuilder(t, 2)

	r := require.New(t)
	// Empty and oversized series are skipped without writing anything.
	r.NoError(pb.WriteOtelExplicitHistogram(metrics.DistributionMetadata{Name: "empty"}, 0, testExplicitPoints{}))
	r.NoError(pb.WriteOtelExponentialHistogram(metrics.DistributionMetadata{Name: "big"}, 3, make(testExponentialPoints, 3)))
	r.NoError(pb.WriteOtelExplicitHistogram(metrics.DistributionMetadata{Name: "ok"}, 1,
		testExplicitPoints{{ts: 1000, sum: 1, cnt: 1, counts: []uint64{1}}}))

	requireProtoscope(t, pb, pipelineContext, `3: {
		1: {2 "ok"}
		9: {10 0 0}
		10: {0x16}
		11: {1z}
		12: {0z}
		13: {0z}
		14: {0}
		15: {1}
		16: {1000z}
		17: {1z}
		20: {1}
		22: {1}
		23: {0z}
		24: {1z}
		30: {0}
		31: {1}
	}`)
}

// TestPayloadsBuilderV3_HistogramPermutations covers every combination of
// histogram type, value type and min/max presence for a single point.
func TestPayloadsBuilderV3_HistogramPermutations(t *testing.T) {
	// Values chosen so that each set compacts to exactly one value type.
	// bound is the explicit bound, or the exponential zero threshold.
	type valueCase struct {
		name                   string
		valueType              int64
		sum, min, max, bound   float64
		sumLit, minLit, maxLit string
		boundLit               string
	}
	valueCases := []valueCase{
		{"zero", valueZero, 0, 0, 0, 0, "", "", "", ""},
		{"sint64", valueSint64, 30, -2, 7, 10, "30z", "-2z", "7z", "10z"},
		{"float32", valueFloat32, 1.5, -0.25, 2.75, 0.5, "1.5i32", "-0.25i32", "2.75i32", "0.5i32"},
		{"float64", valueFloat64, 0.1, -0.2, 0.3, 0.7, "0.1", "-0.2", "0.3", "0.7"},
	}
	valueField := map[int64]int{valueSint64: 17, valueFloat32: 18, valueFloat64: 19}

	for _, exponential := range []bool{false, true} {
		for _, vc := range valueCases {
			for _, haveMin := range []bool{false, true} {
				for _, haveMax := range []bool{false, true} {
					kind := "explicit"
					if exponential {
						kind = "exponential"
					}
					name := fmt.Sprintf("%s/%s/min=%v/max=%v", kind, vc.name, haveMin, haveMax)
					t.Run(name, func(t *testing.T) {
						// Absent min/max carry a value that would otherwise force Float64.
						min, max := math.Pi, math.Pi
						var flags uint64
						vals := []string{vc.sumLit}
						if haveMin {
							min = vc.min
							flags |= pointFlagsHaveMin
							vals = append(vals, vc.minLit)
						}
						if haveMax {
							max = vc.max
							flags |= pointFlagsHaveMax
							vals = append(vals, vc.maxLit)
						}
						vals = append(vals, vc.boundLit)

						pb, pipelineContext := newTestHistogramBuilder(t, 1000)
						var metricType int64
						var header, numBins, binCnts, uints string
						if exponential {
							metricType = metricOtelExponentialHistogram
							header, numBins, binCnts, uints = "1z -2z 3z", "2 1", "1 2 3", "10 4"
							require.NoError(t, pb.WriteOtelExponentialHistogram(
								metrics.DistributionMetadata{Name: "h"}, 1,
								testExponentialPoints{{
									ts: 1000, haveMin: haveMin, haveMax: haveMax, min: min, max: max, sum: vc.sum, zeroThr: vc.bound,
									cnt: 10, zeroCnt: 4, scale: 1, posOffs: -2, negOffs: 3, posCnt: []uint64{1, 2}, negCnt: []uint64{3},
								}},
							))
						} else {
							metricType = metricOtelExplicitHistogram
							numBins, binCnts, uints = "2", "1 2", "3"
							require.NoError(t, pb.WriteOtelExplicitHistogram(
								metrics.DistributionMetadata{Name: "h"}, 1,
								testExplicitPoints{{
									ts: 1000, haveMin: haveMin, haveMax: haveMax, min: min, max: max, sum: vc.sum,
									cnt: 3, bounds: []float64{vc.bound}, counts: []uint64{1, 2},
								}},
							))
						}

						numValues := uint64(len(vals))
						stats := map[int64]uint64{
							valueZero:    pb.stats.valuesZero,
							valueSint64:  pb.stats.valuesSint64,
							valueFloat32: pb.stats.valuesFloat32,
							valueFloat64: pb.stats.valuesFloat64,
						}
						require.Equal(t, map[int64]uint64{valueZero: 0, valueSint64: 0, valueFloat32: 0, valueFloat64: 0, vc.valueType: numValues}, stats)

						// Value columns, in field order. Integer headers always precede
						// the point's values when both are in valsSint64.
						sint64Col := header
						floatCols := ""
						switch vc.valueType {
						case valueZero:
						case valueSint64:
							sint64Col = strings.TrimSpace(header + " " + strings.Join(vals, " "))
						default:
							floatCols = fmt.Sprintf("%d: {%s}", valueField[vc.valueType], strings.Join(vals, " "))
						}
						if sint64Col != "" {
							sint64Col = fmt.Sprintf("17: {%s}", sint64Col)
						}

						requireProtoscope(t, pb, pipelineContext, fmt.Sprintf(`3: {
							1: {1 "h"}
							9: {10 0 0}
							10: {%d}
							11: {1z}
							12: {0z}
							13: {0z}
							14: {0}
							15: {1}
							16: {1000z}
							%s
							%s
							20: {%s}
							22: {%s}
							23: {0z}
							24: {1z}
							30: {%d}
							31: {%s}
						}`, metricType|vc.valueType, sint64Col, floatCols, numBins, binCnts, flags, uints))
					})
				}
			}
		}
	}
}
