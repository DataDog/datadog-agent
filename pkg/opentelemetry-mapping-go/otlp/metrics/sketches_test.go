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

package metrics

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/lightstep/go-expohisto/structure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/util/quantile"
	"github.com/DataDog/datadog-agent/pkg/util/quantile/sketchtest"
)

var _ SketchConsumer = (*sketchConsumer)(nil)

type sketchConsumer struct {
	mockTimeSeriesConsumer
	sk *quantile.Sketch
}

// ConsumeSketch implements the translator.Consumer interface.
func (c *sketchConsumer) ConsumeSketch(
	_ context.Context,
	_ *Dimensions,
	_ uint64,
	_ int64,
	sketch *quantile.Sketch,
) {
	c.sk = sketch
}

func newHistogramMetric(p pmetric.HistogramDataPoint) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rms := md.ResourceMetrics()
	rm := rms.AppendEmpty()
	ilms := rm.ScopeMetrics()
	ilm := ilms.AppendEmpty()
	metricsArray := ilm.Metrics()
	m := metricsArray.AppendEmpty()
	m.SetEmptyHistogram()
	m.SetName("test")

	// Copy Histogram point
	m.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dps := m.Histogram().DataPoints()
	np := dps.AppendEmpty()
	np.SetCount(p.Count())
	np.SetSum(p.Sum())
	np.SetMin(p.Min())
	np.SetMax(p.Max())
	p.BucketCounts().CopyTo(np.BucketCounts())
	p.ExplicitBounds().CopyTo(np.ExplicitBounds())
	np.SetTimestamp(p.Timestamp())

	return md
}

func TestHistogramSketches(t *testing.T) {
	N := 1_000
	M := 50_000.0

	// Given a cumulative distribution function for a distribution
	// with support [0, N], generate an OTLP Histogram data point with N buckets,
	// (-inf, 0], (0, 1], ..., (N-1, N], (N, inf)
	// which contains N*M uniform samples of the distribution.
	fromCDF := func(cdf func(x float64) float64) pmetric.Metrics {
		p := pmetric.NewHistogramDataPoint()
		bounds := make([]float64, N+1)
		buckets := make([]uint64, N+2)
		buckets[0] = 0
		count := uint64(0)
		for i := 0; i < N; i++ {
			bounds[i] = float64(i)
			// the bucket with bounds (i, i+1) has the
			// cdf delta between the bounds as a value.
			buckets[i+1] = uint64((cdf(float64(i+1)) - cdf(float64(i))) * M)
			count += buckets[i+1]
		}
		bounds[N] = float64(N)
		buckets[N+1] = 0
		p.ExplicitBounds().FromRaw(bounds)
		p.BucketCounts().FromRaw(buckets)
		p.SetCount(count)
		p.SetMin(0)
		p.SetMax(cdf(float64(N-1)) - cdf(float64(N-2)))
		return newHistogramMetric(p)
	}

	tests := []struct {
		// distribution name
		name string
		// the cumulative distribution function (within [0,N])
		cdf func(x float64) float64
		// error tolerance for testing cdf(quantile(q)) ≈ q
		epsilon float64
	}{
		{
			// https://en.wikipedia.org/wiki/Continuous_uniform_distribution
			name:    "Uniform distribution (a=0,b=N)",
			cdf:     func(x float64) float64 { return x / float64(N) },
			epsilon: 0.01,
		},
		{
			// https://en.wikipedia.org/wiki/U-quadratic_distribution
			name: "U-quadratic distribution (a=0,b=N)",
			cdf: func(x float64) float64 {
				a := 0.0
				b := float64(N)
				alpha := 12.0 / math.Pow(b-a, 3)
				beta := (b + a) / 2.0
				return alpha / 3 * (math.Pow(x-beta, 3) + math.Pow(beta-alpha, 3))
			},
			epsilon: 0.025,
		},
	}

	defaultEps := 1.0 / 128.0
	tol := 1e-8
	cfg := quantile.Default()
	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			md := fromCDF(test.cdf)
			consumer := &sketchConsumer{}
			_, err := tr.MapMetrics(ctx, md, consumer, nil)
			assert.NoError(t, err)
			sk := consumer.sk

			// Check the minimum is 0.0
			assert.Equal(t, 0.0, sk.Quantile(cfg, 0))
			// Check the quantiles are approximately correct
			for i := 1; i <= 99; i++ {
				q := (float64(i)) / 100.0
				assert.InEpsilon(t,
					// test that the CDF is the (approximate) inverse of the quantile function
					test.cdf(sk.Quantile(cfg, q)),
					q,
					test.epsilon,
					fmt.Sprintf("error too high for p%d", i),
				)
			}

			cumulSum := uint64(0)
			p := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Histogram().DataPoints().At(0)
			assert.Equal(t, sk.Basic.Min, p.Min())
			assert.Equal(t, sk.Basic.Max, p.Max())
			assert.Equal(t, uint64(sk.Basic.Cnt), p.Count())
			assert.Equal(t, sk.Basic.Sum, p.Sum())
			for i := 0; i < p.BucketCounts().Len()-3; i++ {
				{
					q := float64(cumulSum) / float64(p.Count()) * (1 - tol)
					quantileValue := sk.Quantile(cfg, q)
					// quantileValue, if computed from the explicit buckets, would have to be <= bounds[i].
					// Because of remapping, it is <= bounds[i+1].
					// Because of DDSketch accuracy guarantees, it is <= bounds[i+1] * (1 + defaultEps)
					maxExpectedQuantileValue := p.ExplicitBounds().At(i+1) * (1 + defaultEps)
					assert.LessOrEqual(t, quantileValue, maxExpectedQuantileValue)
				}

				cumulSum += p.BucketCounts().At(i + 1)

				{
					q := float64(cumulSum) / float64(p.Count()) * (1 + tol)
					quantileValue := sk.Quantile(cfg, q)
					// quantileValue, if computed from the explicit buckets, would have to be >= bounds[i+1].
					// Because of remapping, it is >= bounds[i].
					// Because of DDSketch accuracy guarantees, it is >= bounds[i] * (1 - defaultEps)
					minExpectedQuantileValue := p.ExplicitBounds().At(i) * (1 - defaultEps)
					assert.GreaterOrEqual(t, quantileValue, minExpectedQuantileValue)
				}
			}
		})
	}
}

func TestExactHistogramStats(t *testing.T) {
	tests := []struct {
		name        string
		getHist     func() pmetric.Metrics
		sum         float64
		count       uint64
		testExtrema bool
		min         float64
		max         float64
	}{}

	// Add tests for issue 6129: https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/6129
	tests = append(tests,
		struct {
			name        string
			getHist     func() pmetric.Metrics
			sum         float64
			count       uint64
			testExtrema bool
			min         float64
			max         float64
		}{
			name: "Uniform distribution (delta)",
			getHist: func() pmetric.Metrics {
				md := pmetric.NewMetrics()
				rms := md.ResourceMetrics()
				rm := rms.AppendEmpty()
				ilms := rm.ScopeMetrics()
				ilm := ilms.AppendEmpty()
				metricsArray := ilm.Metrics()
				m := metricsArray.AppendEmpty()
				m.SetEmptyHistogram()
				m.SetName("test")
				m.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
				dp := m.Histogram().DataPoints()
				p := dp.AppendEmpty()
				p.ExplicitBounds().FromRaw([]float64{0, 5_000, 10_000, 15_000, 20_000})
				// Points from contrib issue 6129: 0, 5_000, 10_000, 15_000, 20_000
				p.BucketCounts().FromRaw([]uint64{0, 1, 1, 1, 1, 1})
				p.SetCount(5)
				p.SetSum(50_000)
				p.SetMin(0)
				p.SetMax(20_000)
				return md
			},
			sum:         50_000,
			count:       5,
			testExtrema: true,
			min:         0,
			max:         20_000,
		},

		struct {
			name        string
			getHist     func() pmetric.Metrics
			sum         float64
			count       uint64
			testExtrema bool
			min         float64
			max         float64
		}{
			name: "Uniform distribution (cumulative)",
			getHist: func() pmetric.Metrics {
				md := pmetric.NewMetrics()
				rms := md.ResourceMetrics()
				rm := rms.AppendEmpty()
				ilms := rm.ScopeMetrics()
				ilm := ilms.AppendEmpty()
				metricsArray := ilm.Metrics()
				m := metricsArray.AppendEmpty()
				m.SetEmptyHistogram()
				m.SetName("test")
				m.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
				dp := m.Histogram().DataPoints()
				// Points from contrib issue 6129: 0, 5_000, 10_000, 15_000, 20_000 repeated.
				bounds := []float64{0, 5_000, 10_000, 15_000, 20_000}
				for i := 1; i <= 2; i++ {
					p := dp.AppendEmpty()
					p.ExplicitBounds().FromRaw(bounds)
					cnt := uint64(i)
					p.BucketCounts().FromRaw([]uint64{0, cnt, cnt, cnt, cnt, cnt})
					p.SetCount(uint64(5 * i))
					p.SetSum(float64(50_000 * i))
					p.SetMin(0)
					p.SetMax(20_000)
				}
				return md
			},
			sum:   50_000,
			count: 5,
		})

	// Add tests for issue 7065: https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/7065
	for pos, val := range []float64{500, 5_000, 50_000} {
		tests = append(tests, struct {
			name        string
			getHist     func() pmetric.Metrics
			sum         float64
			count       uint64
			testExtrema bool
			min         float64
			max         float64
		}{
			name: fmt.Sprintf("Issue 7065 (%d, %f)", pos, val),
			getHist: func() pmetric.Metrics {
				md := pmetric.NewMetrics()
				rms := md.ResourceMetrics()
				rm := rms.AppendEmpty()
				ilms := rm.ScopeMetrics()
				ilm := ilms.AppendEmpty()
				metricsArray := ilm.Metrics()
				m := metricsArray.AppendEmpty()
				m.SetEmptyHistogram()
				m.SetName("test")

				m.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
				bounds := []float64{1_000, 10_000, 100_000}

				dp := m.Histogram().DataPoints()
				for i := 0; i < 2; i++ {
					p := dp.AppendEmpty()
					p.ExplicitBounds().FromRaw(bounds)
					counts := []uint64{0, 0, 0, 0}
					counts[pos] = uint64(i)
					t.Logf("pos: %d, val: %f, counts: %v", pos, val, counts)
					p.BucketCounts().FromRaw(counts)
					p.SetCount(uint64(i))
					p.SetSum(val * float64(i))
					p.SetMin(val)
					p.SetMax(val)
				}
				return md
			},
			sum:   val,
			count: 1,
		})
	}

	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			md := testInstance.getHist()
			consumer := &sketchConsumer{}
			_, err := tr.MapMetrics(ctx, md, consumer, nil)
			assert.NoError(t, err)
			sk := consumer.sk

			assert.Equal(t, testInstance.count, uint64(sk.Basic.Cnt), "counts differ")
			assert.Equal(t, testInstance.sum, sk.Basic.Sum, "sums differ")

			// We only assert on min/max for delta histograms
			if testInstance.testExtrema {
				assert.Equal(t, testInstance.min, sk.Basic.Min, "min differs")
				assert.Equal(t, testInstance.max, sk.Basic.Max, "max differs")
			}
			avg := testInstance.sum / float64(testInstance.count)
			assert.Equal(t, avg, sk.Basic.Avg, "averages differ")
		})
	}
}

func TestInfiniteBounds(t *testing.T) {

	tests := []struct {
		name    string
		getHist func() pmetric.Metrics
		isEmpty bool
	}{
		{
			name: "(-inf, inf): 0",
			getHist: func() pmetric.Metrics {
				p := pmetric.NewHistogramDataPoint()
				p.ExplicitBounds().FromRaw([]float64{})
				p.BucketCounts().FromRaw([]uint64{0})
				p.SetCount(0)
				p.SetSum(0)
				return newHistogramMetric(p)
			},
			isEmpty: true,
		},
		{
			name: "(-inf, inf): 100",
			getHist: func() pmetric.Metrics {
				p := pmetric.NewHistogramDataPoint()
				p.ExplicitBounds().FromRaw([]float64{})
				p.BucketCounts().FromRaw([]uint64{100})
				p.SetCount(100)
				p.SetSum(0)
				p.SetMin(-100)
				p.SetMax(100)
				return newHistogramMetric(p)
			},
		},
		{
			name: "(-inf, 0]: 100, (0, +inf]: 100",
			getHist: func() pmetric.Metrics {
				p := pmetric.NewHistogramDataPoint()
				p.ExplicitBounds().FromRaw([]float64{0})
				p.BucketCounts().FromRaw([]uint64{100, 100})
				p.SetCount(200)
				p.SetSum(0)
				p.SetMin(-100)
				p.SetMax(100)
				return newHistogramMetric(p)
			},
		},
		{
			name: "(-inf, -1]: 100, (-1, 1]: 10,  (1, +inf]: 100",
			getHist: func() pmetric.Metrics {
				p := pmetric.NewHistogramDataPoint()
				p.ExplicitBounds().FromRaw([]float64{-1, 1})
				p.BucketCounts().FromRaw([]uint64{100, 10, 100})
				p.SetCount(210)
				p.SetSum(0)
				p.SetMin(-100)
				p.SetMax(100)
				return newHistogramMetric(p)
			},
		},
	}

	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())
	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			md := testInstance.getHist()
			consumer := &sketchConsumer{}
			_, err := tr.MapMetrics(ctx, md, consumer, nil)
			assert.NoError(t, err)
			sk := consumer.sk

			p := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Histogram().DataPoints().At(0)
			if testInstance.isEmpty {
				// Check that no point is produced if the count is zero.
				assert.Zero(t, p.Count())
				assert.Nil(t, sk)
				// Nothing else to assert, end early
				return
			}
			require.NotNil(t, sk)
			assert.InDelta(t, sk.Basic.Sum, p.Sum(), 1)
			assert.Equal(t, uint64(sk.Basic.Cnt), p.Count())
			assert.Equal(t, sk.Basic.Min, p.Min())
			assert.Equal(t, sk.Basic.Max, p.Max())
		})
	}
}

// fromGoExpoHisto builds a delta exponential histogram from a go-expohisto.
// Adapted from https://github.com/open-telemetry/opentelemetry-collector-contrib/commit/a2f9e1
func fromGoExpoHisto(name string, agg *structure.Histogram[float64], startTime, timeNow time.Time) (md pmetric.Metrics) {
	md = pmetric.NewMetrics()
	nm := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	nm.SetName(name)
	expo := nm.SetEmptyExponentialHistogram()
	expo.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

	dp := expo.DataPoints().AppendEmpty()

	dp.SetCount(agg.Count())
	dp.SetSum(agg.Sum())
	if agg.Count() != 0 {
		dp.SetMin(agg.Min())
		dp.SetMax(agg.Max())
	}

	dp.SetStartTimestamp(pcommon.NewTimestampFromTime(startTime))
	dp.SetTimestamp(pcommon.NewTimestampFromTime(timeNow))

	dp.SetZeroCount(agg.ZeroCount())
	dp.SetScale(agg.Scale())

	for _, half := range []struct {
		inFunc  func() *structure.Buckets
		outFunc func() pmetric.ExponentialHistogramDataPointBuckets
	}{
		{agg.Positive, dp.Positive},
		{agg.Negative, dp.Negative},
	} {
		in := half.inFunc()
		out := half.outFunc()
		out.SetOffset(in.Offset())

		out.BucketCounts().EnsureCapacity(int(in.Len()))

		for i := uint32(0); i < in.Len(); i++ {
			out.BucketCounts().Append(in.At(i))
		}
	}
	return
}

// fromQuantile builds a delta exponential histogram from the quantile function of a known distribution.
func fromQuantile(name string, startTime, timeNow time.Time, quantile sketchtest.QuantileFunction, N, M uint64) pmetric.Metrics {
	agg := new(structure.Histogram[float64])
	// Increase maximum size since contrast on test distributions can be big.
	agg.Init(structure.NewConfig(structure.WithMaxSize(1_000)))
	for i := 0; i <= int(N); i++ {
		agg.UpdateByIncr(quantile(float64(i)/float64(N)), M)
	}

	return fromGoExpoHisto(name, agg, startTime, timeNow)
}

func TestKnownDistributionsQuantile(t *testing.T) {
	timeNow := time.Now()
	startTime := timeNow.Add(-10 * time.Second)
	name := "example.histo"
	const (
		N uint64 = 2_000
		M uint64 = 100
	)

	fN := float64(N)

	// acceptableRelativeError for quantile estimation.
	// Right now it is set to 4%. Anything below 5% is acceptable based on the SDK defaults.
	// The relative error depends on the scale as well as the range of the distribution.
	const acceptableRelativeError = 0.04

	ctx := context.Background()
	tr := newTranslator(t, zap.NewNop())

	for _, tt := range []struct {
		name     string
		quantile sketchtest.QuantileFunction
		// the map of quantiles for which the test is known to fail
		excludedQuantiles map[int]struct{}
	}{
		{
			name:     "Uniform distribution (a=0,b=N)",
			quantile: sketchtest.UniformQ(0, fN),
		},
		{
			name:     "Uniform distribution (a=-N,b=0)",
			quantile: sketchtest.UniformQ(-fN, 0),
		},
		{
			name:     "Uniform distribution (a=-N,b=N)",
			quantile: sketchtest.UniformQ(-fN, fN),
		},
		{
			name:     "U-quadratic distribution (a=0,b=N)",
			quantile: sketchtest.UQuadraticQ(0, fN),
		},
		{
			name:     "U-quadratic distribution (a=-N,b=0)",
			quantile: sketchtest.UQuadraticQ(-fN, 0),
			// Similar to the pkg/quantile tests, the p99 for this test fails, likely due to the shift of leftover bucket counts the right that is performed
			// during the DDSketch -> quantile.Sketch conversion, causing the p99 of the output sketch to fall on 0
			// (which means the InEpsilon check returns 1).
			excludedQuantiles: map[int]struct{}{99: {}},
		},
		{
			name:     "U-quadratic distribution (a=-N,b=N)",
			quantile: sketchtest.UQuadraticQ(-fN/2, fN/2),
		},
		{
			name:     "Truncated Exponential distribution (a=0,b=N,lambda=1/100)",
			quantile: sketchtest.TruncateQ(0, fN, sketchtest.ExponentialQ(1.0/100), sketchtest.ExponentialCDF(1.0/100)),
		},
		{
			name:     "Truncated Normal distribution (a=-8,b=8,mu=0, sigma=1e-3)",
			quantile: sketchtest.TruncateQ(-8, 8, sketchtest.NormalQ(0, 1e-3), sketchtest.NormalCDF(0, 1e-3)),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			md := fromQuantile(name, startTime, timeNow, tt.quantile, N, M)
			consumer := &sketchConsumer{}
			_, err := tr.MapMetrics(ctx, md, consumer, nil)
			require.NoError(t, err)
			require.NotNil(t, consumer.sk)

			sketchConfig := quantile.Default()
			for i := 0; i <= 100; i++ {
				if _, ok := tt.excludedQuantiles[i]; ok {
					// skip excluded quantile
					continue
				}
				q := float64(i) / 100.0
				expectedValue := tt.quantile(q)
				quantileValue := consumer.sk.Quantile(sketchConfig, q)
				if expectedValue == 0.0 {
					// If the expected value is 0, we can't use InEpsilon, so we directly check that
					// the value is equal (within a small float precision error margin).
					assert.InDelta(
						t,
						expectedValue,
						quantileValue,
						2e-11,
					)
				} else {
					assert.InEpsilon(t,
						expectedValue,
						quantileValue,
						acceptableRelativeError,
						fmt.Sprintf("error too high for p%d", i),
					)
				}
			}
		})
	}
}
