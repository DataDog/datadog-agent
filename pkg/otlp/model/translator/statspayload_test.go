// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package translator

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/store"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

// The sketch's relative accuracy and maximum number of bins is identical
// to the one used in the trace-agent for consistency:
// https://github.com/DataDog/datadog-agent/blob/cbac965/pkg/trace/stats/statsraw.go#L18-L26
const (
	sketchRelativeAccuracy = 0.01
	sketchMaxBins          = 2048
)

func TestHelpers(t *testing.T) {
	t.Run("putStr", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			m := pcommon.NewMap()
			putStr(m, "key", "value")
			v, ok := m.Get("key")
			require.True(t, ok)
			require.Equal(t, v.Str(), "value")
		})

		t.Run("empty", func(t *testing.T) {
			m := pcommon.NewMap()
			putStr(m, "key", "")
			_, ok := m.Get("key")
			require.False(t, ok)
		})
	})

	t.Run("putInt", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			m := pcommon.NewMap()
			putInt(m, "key", 1)
			v, ok := m.Get("key")
			require.True(t, ok)
			require.EqualValues(t, v.Int(), 1)
		})

		t.Run("empty", func(t *testing.T) {
			m := pcommon.NewMap()
			putInt(m, "key", 0)
			_, ok := m.Get("key")
			require.False(t, ok)
		})
	})

	t.Run("getInt", func(t *testing.T) {
		m := pcommon.NewMap()
		m.PutInt("key", 1)
		require.Equal(t, getInt(m, "key"), uint64(1))
		require.Equal(t, getInt(m, "invalid"), uint64(0))
	})

	t.Run("getStr", func(t *testing.T) {
		m := pcommon.NewMap()
		m.PutStr("key", "value")
		require.Equal(t, getStr(m, "key"), "value")
		require.Equal(t, getStr(m, "invalid"), "")
	})

	t.Run("putGroupedStatsAttr", func(t *testing.T) {
		for _, tt := range []struct {
			attr map[string]interface{}
			cgs  *pb.ClientGroupedStats
		}{
			{
				cgs: &pb.ClientGroupedStats{
					Service:        "my-service",
					Name:           "my-name",
					Resource:       "my-resource",
					HTTPStatusCode: 220,
					Type:           "my-type",
					DBType:         "my-db-type",
					Synthetics:     true,
				},
				attr: map[string]interface{}{
					statsKeyService:        "my-service",
					statsKeySpanName:       "my-name",
					statsKeySpanResource:   "my-resource",
					statsKeyHTTPStatusCode: int64(220),
					statsKeySpanType:       "my-type",
					statsKeySpanDBType:     "my-db-type",
					statsKeySynthetics:     true,
				},
			},
			{
				cgs: &pb.ClientGroupedStats{
					Service:    "my-service",
					Synthetics: false,
				},
				attr: map[string]interface{}{
					statsKeyService: "my-service",
				},
			},
			{
				cgs: &pb.ClientGroupedStats{
					Service: "my-service",
				},
				attr: map[string]interface{}{
					statsKeyService: "my-service",
				},
			},
		} {
			m := pcommon.NewMap()
			putGroupedStatsAttr(m, tt.cgs)
			require.EqualValues(t, m.AsRaw(), tt.attr)
		}
	})
}

func TestAggregations(t *testing.T) {
	m1 := pcommon.NewMap()
	m1.FromRaw(map[string]interface{}{
		statsKeyService:        "my-service",
		statsKeySpanName:       "my-name",
		statsKeySpanResource:   "my-resource",
		statsKeyHTTPStatusCode: int64(220),
		statsKeySpanType:       "my-type",
		statsKeySpanDBType:     "my-db-type",
		statsKeySynthetics:     true,
	})
	var agg aggregations
	val1 := agg.Value(m1)
	val1.TopLevelHits = 5
	require.True(t, val1 == agg.Value(m1)) // same key should return same pointer
	require.Equal(t, val1, agg.Value(m1))  // ...and same contents

	m2 := pcommon.NewMap()
	m2.FromRaw(map[string]interface{}{
		statsKeyService:        "my-service",
		statsKeySpanName:       "my-name",
		statsKeySpanResource:   "my-resource",
		statsKeyHTTPStatusCode: int64(220),
		statsKeySpanType:       "my-type",
		statsKeySpanDBType:     "my-db-type",
	})
	val2 := agg.Value(m2)
	val2.Hits = 10
	val2.Errors++
	require.True(t, val2 == agg.Value(m2))
	require.Equal(t, val2, agg.Value(m2))
	require.True(t, val1 != agg.Value(m2))
	require.NotEqual(t, val1, agg.Value(m2))

	require.ElementsMatch(t, agg.Stats(), []pb.ClientGroupedStats{
		{
			Service:        "my-service",
			Name:           "my-name",
			Resource:       "my-resource",
			HTTPStatusCode: 0xdc,
			Type:           "my-type",
			DBType:         "my-db-type",
			Hits:           0x0,
			OkSummary:      []uint8(nil),
			ErrorSummary:   []uint8(nil),
			Synthetics:     true,
			TopLevelHits:   5,
		},
		{
			Service:        "my-service",
			Name:           "my-name",
			Resource:       "my-resource",
			HTTPStatusCode: 0xdc,
			Type:           "my-type",
			DBType:         "my-db-type",
			OkSummary:      []uint8(nil),
			ErrorSummary:   []uint8(nil),
			Synthetics:     false,
			Hits:           10,
			Errors:         1,
		},
	})
}

func TestSum(t *testing.T) {
	ms := pmetric.NewMetricSlice()
	appendSum(ms, "hits", 23, 100, 150, &pb.ClientGroupedStats{
		Service:  "mysql",
		Resource: "SELECT * FROM users",
	})

	t.Run("appendSum", func(t *testing.T) {
		require.Equal(t, ms.Len(), 1)
		require.Equal(t, ms.At(0).Type(), pmetric.MetricTypeSum)
		sum := ms.At(0).Sum()
		require.Equal(t, sum.AggregationTemporality(), pmetric.AggregationTemporalityDelta)
		require.Equal(t, sum.IsMonotonic(), true)
		require.Equal(t, sum.DataPoints().Len(), 1)
		dp := sum.DataPoints().At(0)
		require.Equal(t, dp.StartTimestamp(), pcommon.Timestamp(100))
		require.Equal(t, dp.Timestamp(), pcommon.Timestamp(150))
		require.Equal(t, dp.IntValue(), int64(23))
		require.EqualValues(t, dp.Attributes().AsRaw(), map[string]interface{}{
			statsKeyService:      "mysql",
			statsKeySpanResource: "SELECT * FROM users",
		})
	})

	t.Run("extractSum", func(t *testing.T) {
		buck := &pb.ClientStatsBucket{}
		m, out := (&Translator{logger: zap.NewNop()}).extractSum(ms.At(0).Sum(), buck)
		require.Equal(t, out, uint64(23))
		require.Equal(t, m.AsRaw(), map[string]interface{}{statsKeySpanResource: "SELECT * FROM users", statsKeyService: "mysql"})
		require.EqualValues(t, buck.Start, 100)
		require.EqualValues(t, buck.Duration, 50)
	})
}

func TestStoreToBuckets(t *testing.T) {
	s := store.NewDenseStore()
	s.AddWithCount(-3, 12)
	s.AddWithCount(5, 10)
	s.AddWithCount(6, 11)
	s.AddWithCount(10, 15)
	b := pmetric.NewExponentialHistogramDataPointBuckets()
	storeToBuckets(s, b)
	require.EqualValues(t, b.Offset(), -3)
	require.EqualValues(t, b.BucketCounts().Len(), 14)
	require.EqualValues(t, b.BucketCounts().AsRaw(), []uint64{12, 0, 0, 0, 0, 0, 0, 0, 10, 11, 0, 0, 0, 15})
}

func TestSketchPoint(t *testing.T) {
	ms := pmetric.NewMetricSlice()
	sketch, err := ddsketch.LogCollapsingLowestDenseDDSketch(sketchRelativeAccuracy, sketchMaxBins)
	if err != nil {
		t.Fatal(err)
	}
	sketch.AddWithCount(-1, 4)
	sketch.AddWithCount(1, 1)
	sketch.AddWithCount(2, 5)
	sketch.AddWithCount(5, 10)
	buf, err := proto.Marshal(sketch.ToProto())
	if err != nil {
		t.Fatal(err)
	}
	require.NotEmpty(t, buf)
	if err := appendSketch(ms, "hits", buf, 200, 250, &pb.ClientGroupedStats{
		Service:        "my-service",
		Name:           "my-name",
		Resource:       "my-resource",
		HTTPStatusCode: 0xdc,
		Type:           "my-type",
		DBType:         "my-db-type",
		Hits:           0x0,
		OkSummary:      []uint8(nil),
		ErrorSummary:   []uint8(nil),
		Synthetics:     true,
		TopLevelHits:   5,
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("appendSketch", func(t *testing.T) {
		require.Equal(t, ms.Len(), 1)
		require.Equal(t, ms.At(0).Type(), pmetric.MetricTypeExponentialHistogram)
		sum := ms.At(0).ExponentialHistogram()
		require.Equal(t, sum.AggregationTemporality(), pmetric.AggregationTemporalityDelta)
		require.Equal(t, sum.DataPoints().Len(), 1)
		dp := sum.DataPoints().At(0)
		require.Equal(t, dp.StartTimestamp(), pcommon.Timestamp(200))
		require.Equal(t, dp.Timestamp(), pcommon.Timestamp(250))
		require.EqualValues(t, dp.Attributes().AsRaw(), map[string]interface{}{
			statsKeySpanDBType:     "my-db-type",
			statsKeyHTTPStatusCode: int64(220),
			statsKeySpanName:       "my-name",
			statsKeySpanResource:   "my-resource",
			statsKeyService:        "my-service",
			statsKeySynthetics:     true,
			statsKeySpanType:       "my-type",
		})
		require.EqualValues(t, dp.Count(), sketch.GetCount())
		if max, err := sketch.GetMaxValue(); err == nil {
			require.EqualValues(t, dp.Max(), max)
		}
		if min, err := sketch.GetMinValue(); err == nil {
			require.EqualValues(t, dp.Min(), min)
		}
		require.EqualValues(t, dp.Sum(), sketch.GetSum())
		require.EqualValues(t, dp.ZeroCount(), sketch.GetZeroCount())
		require.EqualValues(t, dp.Scale(), 5)

		b := dp.Positive()
		require.EqualValues(t, b.Offset(), 0)
		require.EqualValues(t, b.BucketCounts().Len(), 81)
		require.EqualValues(t, b.BucketCounts().AsRaw(), []uint64{0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa})

		b = dp.Negative()
		require.EqualValues(t, b.Offset(), 0)
		require.EqualValues(t, b.BucketCounts().Len(), 1)
		require.EqualValues(t, b.BucketCounts().AsRaw(), []uint64{0x4})
	})

	t.Run("extractSketch", func(t *testing.T) {
		buck := &pb.ClientStatsBucket{}
		m, out := (&Translator{logger: zap.NewNop()}).extractSketch(ms.At(0).ExponentialHistogram(), buck)
		require.NotEmpty(t, out)
		require.Equal(t, out, buf)
		require.EqualValues(t, map[string]interface{}{
			statsKeySpanDBType:     "my-db-type",
			statsKeyHTTPStatusCode: int64(220),
			statsKeySpanName:       "my-name",
			statsKeySpanResource:   "my-resource",
			statsKeyService:        "my-service",
			statsKeySynthetics:     true,
			statsKeySpanType:       "my-type",
		}, m.AsRaw())
		require.EqualValues(t, buck.Start, 200)
		require.EqualValues(t, buck.Duration, 50)
	})
}

func testSketchBytes(nums ...float64) []byte {
	sketch, err := ddsketch.LogCollapsingLowestDenseDDSketch(sketchRelativeAccuracy, sketchMaxBins)
	if err != nil {
		// the only possible error is if the relative accuracy is < 0 or > 1;
		// we know that's not the case because it's a constant defined as 0.01
		panic(err)
	}
	for _, num := range nums {
		sketch.Add(num)
	}
	buf, err := proto.Marshal(sketch.ToProto())
	if err != nil {
		// there should be no error under any circumstances here
		panic(err)
	}
	return buf
}

func TestConversion(t *testing.T) {
	want := pb.StatsPayload{
		Stats: []pb.ClientStatsPayload{
			{
				Hostname:         "host",
				Env:              "prod",
				Version:          "v1.2",
				Lang:             "go",
				TracerVersion:    "v44",
				RuntimeID:        "123jkl",
				Sequence:         2,
				AgentAggregation: "blah",
				Service:          "mysql",
				ContainerID:      "abcdef123456",
				Tags:             []string{"a:b", "c:d"},
				Stats: []pb.ClientStatsBucket{
					{
						Start:    10,
						Duration: 1,
						Stats: []pb.ClientGroupedStats{
							{
								Service:        "mysql",
								Name:           "db.query",
								Resource:       "UPDATE name",
								HTTPStatusCode: 100,
								Type:           "sql",
								DBType:         "postgresql",
								Synthetics:     true,
								Hits:           5,
								Errors:         2,
								Duration:       100,
								OkSummary:      testSketchBytes(1, 2, 3),
								ErrorSummary:   testSketchBytes(4, 5, 6),
								TopLevelHits:   3,
							},
							{
								Service:        "kafka",
								Name:           "queue.add",
								Resource:       "append",
								HTTPStatusCode: 220,
								Type:           "queue",
								Hits:           15,
								Errors:         3,
								Duration:       143,
								OkSummary:      nil,
								ErrorSummary:   nil,
								TopLevelHits:   5,
							},
						},
					},
					{
						Start:    20,
						Duration: 3,
						Stats: []pb.ClientGroupedStats{
							{
								Service:        "php-go",
								Name:           "http.post",
								Resource:       "user_profile",
								HTTPStatusCode: 440,
								Type:           "web",
								Hits:           11,
								Errors:         3,
								Duration:       987,
								OkSummary:      testSketchBytes(7, 8),
								ErrorSummary:   testSketchBytes(9, 10, 11),
								TopLevelHits:   1,
							},
						},
					},
				},
			},
			{
				Hostname:         "host2",
				Env:              "staging",
				Version:          "v1.3",
				Lang:             "java",
				TracerVersion:    "v12",
				RuntimeID:        "12#12@",
				Sequence:         2,
				AgentAggregation: "blur",
				Service:          "sprint",
				ContainerID:      "kljdsfalk32",
				Tags:             []string{"x:y", "z:w"},
				Stats: []pb.ClientStatsBucket{
					{
						Start:    30,
						Duration: 5,
						Stats: []pb.ClientGroupedStats{
							{
								Service:        "spring-web",
								Name:           "http.get",
								Resource:       "login",
								HTTPStatusCode: 200,
								Type:           "web",
								Hits:           12,
								Errors:         2,
								Duration:       13,
								OkSummary:      testSketchBytes(9, 7, 5),
								ErrorSummary:   testSketchBytes(9, 5, 2),
								TopLevelHits:   9,
							},
						},
					},
				},
			},
		},
	}

	t.Run("same", func(t *testing.T) {
		trans := &Translator{logger: zap.NewNop()}
		var got pb.StatsPayload
		mx := trans.StatsPayloadToMetrics(want)
		for i := 0; i < mx.ResourceMetrics().Len(); i++ {
			rm := mx.ResourceMetrics().At(i)
			out, err := trans.statsPayloadFromMetrics(rm)
			if err != nil {
				t.Fatal(err)
			}
			got.Stats = append(got.Stats, out)
		}
		var found int
	outer:
		for _, wants := range want.Stats {
			for _, gots := range got.Stats {
				if equalStats(wants, gots) {
					found++
					continue outer
				}
			}
		}
		if found != len(want.Stats) {
			t.Fatalf("Found %d/%d", found, len(want.Stats))
		}
	})
}

func equalStats(want, got pb.ClientStatsPayload) bool {
	cpwant, cpgot := want, got
	cpwant.Stats = nil
	cpgot.Stats = nil
	if !assert.ObjectsAreEqual(cpwant, cpgot) {
		return false
	}
	var found int
outer:
	for _, wantb := range want.Stats {
		for _, gotb := range got.Stats {
			props := wantb.Start == gotb.Start && wantb.Duration == gotb.Duration && wantb.AgentTimeShift == gotb.AgentTimeShift
			if props && assert.ElementsMatch(&fakeT{}, wantb.Stats, gotb.Stats) {
				found++
				continue outer
			}
		}
	}
	return found == len(want.Stats)
}

// fakeT implements testing.T
type fakeT struct{}

func (*fakeT) Errorf(_ string, _ ...interface{}) {}
