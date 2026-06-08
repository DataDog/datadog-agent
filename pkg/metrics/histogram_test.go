// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"math/rand"
	"runtime"
	"sort"
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistogramConf(t *testing.T) {
	assert.Equal(t, []int{95, 96, 28, 57, 58}, ParsePercentiles([]string{"0.95", "0.96", "0.28", "0.57", "0.58"}))
}

func TestHistogramConfError(t *testing.T) {
	assert.Equal(t, []int{95, 22}, ParsePercentiles([]string{"0.95", "test", "0.12test", "0.22", "200", "-50"}))
}

func TestConfigureDefault(t *testing.T) {
	cfg := setupConfig(t)
	hist := NewHistogram(10, cfg)
	hist.addSample(&MetricSample{Value: 1}, 50)
	hist.addSample(&MetricSample{Value: 2}, 55)

	_, err := hist.flush(60)
	require.Nil(t, err)
	assert.Equal(t, []string{"max", "median", "avg", "count"}, hist.aggregates)
	assert.Equal(t, []int{95}, hist.percentiles)
}

func TestConfigure(t *testing.T) {
	mockConfig := configmock.New(t)

	defaultAggregates = nil
	defaultPercentiles = nil
	aggregates := []string{"max", "min", "test"}
	mockConfig.SetInTest("histogram_aggregates", aggregates)
	mockConfig.SetInTest("histogram_percentiles", []string{"0.50", "0.30", "0.98"})

	hist := NewHistogram(10, mockConfig)
	assert.Equal(t, aggregates, hist.aggregates)
	assert.Equal(t, []int{30, 50, 98}, hist.percentiles)
}

func TestDefaultHistogramSampling(t *testing.T) {
	// Initialize default histogram
	cfg := setupConfig(t)

	defaultAggregates = nil
	defaultPercentiles = nil
	mHistogram := NewHistogram(10, cfg)

	// Empty flush
	_, err := mHistogram.flush(50)
	assert.NotNil(t, err)

	// Add samples
	mHistogram.addSample(&MetricSample{Value: 1}, 50)
	mHistogram.addSample(&MetricSample{Value: 10}, 51)
	mHistogram.addSample(&MetricSample{Value: 4}, 55)
	mHistogram.addSample(&MetricSample{Value: 5}, 55)
	mHistogram.addSample(&MetricSample{Value: 2}, 55)
	mHistogram.addSample(&MetricSample{Value: 2}, 55)

	series, err := mHistogram.flush(60)
	assert.Nil(t, err)
	if assert.Len(t, series, 5) {
		for _, serie := range series {
			assert.Len(t, serie.Points, 1)
			assert.EqualValues(t, 60, serie.Points[0].Ts)
		}
		assert.InEpsilon(t, 10, series[0].Points[0].Value, epsilon)     // max
		assert.Equal(t, ".max", series[0].NameSuffix)                   // max
		assert.InEpsilon(t, 2, series[1].Points[0].Value, epsilon)      // median
		assert.Equal(t, ".median", series[1].NameSuffix)                // median
		assert.InEpsilon(t, 12./3., series[2].Points[0].Value, epsilon) // avg
		assert.Equal(t, ".avg", series[2].NameSuffix)                   // avg
		assert.InEpsilon(t, 0.6, series[3].Points[0].Value, epsilon)    // count
		assert.Equal(t, ".count", series[3].NameSuffix)                 // count
		assert.InEpsilon(t, 10, series[4].Points[0].Value, epsilon)     // 0.95
		assert.Equal(t, ".95percentile", series[4].NameSuffix)          // 0.95
	}

	_, err = mHistogram.flush(61)
	assert.NotNil(t, err)
}

func TestCustomHistogramSampling(t *testing.T) {
	// Initialize custom histogram, with an invalid aggregate
	cfg := setupConfig(t)
	mHistogram := NewHistogram(10, cfg)
	mHistogram.configure([]string{"min", "sum", "invalid"}, []int{})

	// Empty flush
	_, err := mHistogram.flush(50)
	assert.NotNil(t, err)

	// Add samples
	mHistogram.addSample(&MetricSample{Value: 1}, 50)
	mHistogram.addSample(&MetricSample{Value: 10}, 51)
	mHistogram.addSample(&MetricSample{Value: 4}, 55)
	mHistogram.addSample(&MetricSample{Value: 5}, 55)
	mHistogram.addSample(&MetricSample{Value: 2}, 55)
	mHistogram.addSample(&MetricSample{Value: 2}, 55)

	series, err := mHistogram.flush(60)
	assert.Nil(t, err)
	if assert.Len(t, series, 2) {
		// Only 2 series are returned (the invalid aggregate is ignored)
		for _, serie := range series {
			assert.Len(t, serie.Points, 1)
			assert.EqualValues(t, 60, serie.Points[0].Ts)
		}
		assert.InEpsilon(t, 1, series[0].Points[0].Value, epsilon)            // min
		assert.Equal(t, ".min", series[0].NameSuffix)                         // min
		assert.InEpsilon(t, 1+10+4+5+2+2, series[1].Points[0].Value, epsilon) // sum
		assert.Equal(t, ".sum", series[1].NameSuffix)                         // sum
	}

	_, err = mHistogram.flush(61)
	assert.NotNil(t, err)
}

func shuffle(slice []float64) {
	for i := len(slice) - 1; i > 0; i-- {
		j := rand.Intn(i)
		slice[i], slice[j] = slice[j], slice[i]
	}
}

func TestHistogramPercentiles(t *testing.T) {
	// Initialize custom histogram
	cfg := setupConfig(t)
	mHistogram := NewHistogram(10, cfg)
	mHistogram.configure([]string{"max", "median", "avg", "count", "min"}, []int{95, 80})

	// Empty flush
	_, err := mHistogram.flush(50)
	assert.NotNil(t, err)

	// Sample 20 times all numbers between 1 and 100.
	// This means our percentiles should be relatively close to themselves.
	var percentiles []float64
	for i := 1; i <= 100; i++ {
		percentiles = append(percentiles, float64(i))
	}
	shuffle(percentiles) // in place
	for _, p := range percentiles {
		for j := 0; j < 20; j++ {
			mHistogram.addSample(&MetricSample{Value: p}, 50)
		}
	}

	series, err := mHistogram.flush(60)
	assert.Nil(t, err)
	if assert.Len(t, series, 7) {
		for _, serie := range series {
			assert.Len(t, serie.Points, 1)
			assert.EqualValues(t, 60, serie.Points[0].Ts)
		}
		assert.InEpsilon(t, 100, series[0].Points[0].Value, epsilon)                         // max
		assert.Equal(t, ".max", series[0].NameSuffix)                                        // max
		assert.InEpsilon(t, 50, series[1].Points[0].Value, epsilon)                          // median
		assert.Equal(t, ".median", series[1].NameSuffix)                                     // median
		assert.InEpsilon(t, 50, series[2].Points[0].Value, epsilon)                          // avg
		assert.Equal(t, ".avg", series[2].NameSuffix)                                        // avg
		assert.InEpsilon(t, float64(100*20)/float64(10), series[3].Points[0].Value, epsilon) // count
		assert.Equal(t, ".count", series[3].NameSuffix)                                      // count
		assert.InEpsilon(t, 1, series[4].Points[0].Value, epsilon)                           // min
		assert.Equal(t, ".min", series[4].NameSuffix)                                        // min
		assert.InEpsilon(t, 80, series[5].Points[0].Value, epsilon)                          // 0.80
		assert.Equal(t, ".80percentile", series[5].NameSuffix)                               // 0.80
		assert.InEpsilon(t, 95, series[6].Points[0].Value, epsilon)                          // 0.95
		assert.Equal(t, ".95percentile", series[6].NameSuffix)                               // 0.95
	}

	_, err = mHistogram.flush(61)
	assert.NotNil(t, err)
}

func TestHistogramSampleRate(t *testing.T) {
	cfg := setupConfig(t)
	mHistogram := NewHistogram(10, cfg)
	mHistogram.configure([]string{"max", "min", "median", "avg", "sum", "count"}, []int{20, 95, 80})

	mHistogram.addSample(&MetricSample{Value: 1}, 50)
	mHistogram.addSample(&MetricSample{Value: 2, SampleRate: 0.5}, 50)
	mHistogram.addSample(&MetricSample{Value: 3, SampleRate: 0.2}, 50)
	mHistogram.addSample(&MetricSample{Value: 10, SampleRate: 0.5}, 50)

	series, err := mHistogram.flush(60)
	assert.Nil(t, err)
	require.Len(t, series, 9)

	for _, serie := range series {
		assert.Len(t, serie.Points, 1)
		assert.EqualValues(t, 60, serie.Points[0].Ts)
	}
	assert.InEpsilon(t, 10, series[0].Points[0].Value, epsilon) // max
	assert.Equal(t, ".max", series[0].NameSuffix)               // max
	assert.InEpsilon(t, 1, series[1].Points[0].Value, epsilon)  // min
	assert.Equal(t, ".min", series[1].NameSuffix)               // min
	assert.InEpsilon(t, 3, series[2].Points[0].Value, epsilon)  // median
	assert.Equal(t, ".median", series[2].NameSuffix)            // median
	assert.InEpsilon(t, 4, series[3].Points[0].Value, epsilon)  // avg
	assert.Equal(t, ".avg", series[3].NameSuffix)               // avg
	assert.InEpsilon(t, 40, series[4].Points[0].Value, epsilon) // sum
	assert.Equal(t, ".sum", series[4].NameSuffix)               // sum
	assert.InEpsilon(t, 1, series[5].Points[0].Value, epsilon)  // count
	assert.Equal(t, ".count", series[5].NameSuffix)             // count
	assert.InEpsilon(t, 2, series[6].Points[0].Value, epsilon)  // 0.20
	assert.Equal(t, ".20percentile", series[6].NameSuffix)      // 0.20
	assert.InEpsilon(t, 3, series[7].Points[0].Value, epsilon)  // 0.80
	assert.Equal(t, ".80percentile", series[7].NameSuffix)      // 0.80
	assert.InEpsilon(t, 10, series[8].Points[0].Value, epsilon) // 0.95
	assert.Equal(t, ".95percentile", series[8].NameSuffix)      // 0.95

	_, err = mHistogram.flush(61)
	assert.NotNil(t, err)
}

func TestHistogramReset(t *testing.T) {
	cfg := setupConfig(t)
	mHistogram := NewHistogram(10, cfg)
	mHistogram.configure([]string{"max", "min", "median", "avg", "sum", "count"}, []int{20, 95, 80})

	mHistogram.addSample(&MetricSample{Value: 1}, 50)
	mHistogram.addSample(&MetricSample{Value: 2, SampleRate: 0.5}, 50)
	_, err := mHistogram.flush(60)
	assert.Nil(t, err)

	mHistogram.addSample(&MetricSample{Value: 10}, 50)
	series, err := mHistogram.flush(70)
	assert.Nil(t, err)
	require.Len(t, series, 9)

	for _, serie := range series {
		assert.Len(t, serie.Points, 1)
		assert.EqualValues(t, 70, serie.Points[0].Ts)
	}
	assert.InEpsilon(t, 10, series[0].Points[0].Value, epsilon)  // max
	assert.Equal(t, ".max", series[0].NameSuffix)                // max
	assert.InEpsilon(t, 10, series[1].Points[0].Value, epsilon)  // min
	assert.Equal(t, ".min", series[1].NameSuffix)                // min
	assert.InEpsilon(t, 10, series[2].Points[0].Value, epsilon)  // median
	assert.Equal(t, ".median", series[2].NameSuffix)             // median
	assert.InEpsilon(t, 10, series[3].Points[0].Value, epsilon)  // avg
	assert.Equal(t, ".avg", series[3].NameSuffix)                // avg
	assert.InEpsilon(t, 10, series[4].Points[0].Value, epsilon)  // sum
	assert.Equal(t, ".sum", series[4].NameSuffix)                // sum
	assert.InEpsilon(t, 0.1, series[5].Points[0].Value, epsilon) // count
	assert.Equal(t, ".count", series[5].NameSuffix)              // count
	assert.InEpsilon(t, 10, series[6].Points[0].Value, epsilon)  // 0.20
	assert.Equal(t, ".20percentile", series[6].NameSuffix)       // 0.20
	assert.InEpsilon(t, 10, series[7].Points[0].Value, epsilon)  // 0.80
	assert.Equal(t, ".80percentile", series[7].NameSuffix)       // 0.80
	assert.InEpsilon(t, 10, series[8].Points[0].Value, epsilon)  // 0.95
	assert.Equal(t, ".95percentile", series[8].NameSuffix)       // 0.95

	_, err = mHistogram.flush(71)
	assert.NotNil(t, err)
}

// TestHistogramAverageExtremeScale uses the classic float-pathology sequence
// {1, +1e100, 1, -1e100} (see Kahan / Peters on
// https://en.wikipedia.org/wiki/Kahan_summation_algorithm). The exact sum in ℝ
// is 2.0; the flushed .sum is compared to that (addSample order matches the slice).
func TestHistogramAverageExtremeScale(t *testing.T) {
	defaultAggregates = nil
	defaultPercentiles = nil
	cfg := setupConfig(t)
	hist := NewHistogram(10, cfg)
	hist.configure([]string{"sum"}, nil)

	petersExtreme := []float64{1.0, 1e100, 1.0, -1e100}
	const wantSum = 2.0 // 1 + 1e100 + 1 + (-1e100) in exact arithmetic

	for _, v := range petersExtreme {
		hist.addSample(&MetricSample{Value: v}, 0)
	}

	series, err := hist.flush(1)
	require.NoError(t, err)
	require.Len(t, series, 1)
	assert.Equal(t, ".sum", series[0].NameSuffix)
	assert.Equal(t, wantSum, series[0].Points[0].Value,
		"flushed .sum should be 2.0 in ℝ; Neumaier summation in addSample recovers it in float64.")
}

// TestHistogramNonReciprocalSampleRate pins the .sum/.avg/.count scaling for DogStatsD
// sample rates whose reciprocal is not an integer (e.g. @0.21 → 1/rate ≈ 4.7619).
//
// The historical implementation stored an int64-truncated weight and used it for the
// sum, producing .sum = value*4 instead of value*4.7619 — a ~16% undercount versus the
// Counter metric type, which scales by the exact 1/SampleRate. All three aggregates now
// use the exact 1/rate scaling, matching Counter and the documented DogStatsD semantics.
//
// Regression check: an int-truncated weight (= 4 for @0.21) would give .sum = 200 and
// .count = 2.0 — both off by ~16%, well outside the 1e-12 epsilon below.
func TestHistogramNonReciprocalSampleRate(t *testing.T) {
	defaultAggregates = nil
	defaultPercentiles = nil
	cfg := setupConfig(t)
	hist := NewHistogram(10, cfg)
	hist.configure([]string{"sum", "avg", "count"}, nil)

	for i := 0; i < 5; i++ {
		hist.addSample(&MetricSample{Value: 10, SampleRate: 0.21}, 0)
	}

	series, err := hist.flush(1)
	require.NoError(t, err)
	require.Len(t, series, 3)

	const (
		wantSum   = 238.09523809523807 // 50 / 0.21 at float64 precision
		wantAvg   = 10.0
		wantCount = 2.380952380952381 // (5 / 0.21) / interval(10)
	)

	assert.Equal(t, ".sum", series[0].NameSuffix)
	assert.InEpsilon(t, wantSum, series[0].Points[0].Value, 1e-12,
		"sum must scale by exact 1/rate (matches Counter), not int64-truncated 1/rate")

	assert.Equal(t, ".avg", series[1].NameSuffix)
	assert.InEpsilon(t, wantAvg, series[1].Points[0].Value, 1e-12)

	assert.Equal(t, ".count", series[2].NameSuffix)
	assert.InEpsilon(t, wantCount, series[2].Points[0].Value, 1e-12,
		"count must scale by exact 1/rate, matching Counter and DogStatsD docs")
}

//
// Benchmark
//

func benchHistogram(b *testing.B, number int, sampleRate float64) {
	cfg := setupConfig(b)
	for n := 0; n < b.N; n++ {
		h := NewHistogram(1, cfg)
		h.configure([]string{"max", "min", "median", "avg", "sum", "count"}, []int{20, 95, 80})
		m := MetricSample{Value: 21, SampleRate: sampleRate}

		for i := 0; i < number; i++ {
			h.addSample(&m, 10)
		}
		h.flush(10)
	}
}

func BenchmarkHistogram2SampleRate1(b *testing.B) {
	benchHistogram(b, 2, 1.0)
}

func BenchmarkHistogram10SampleRate1(b *testing.B) {
	benchHistogram(b, 10, 1.0)
}

func BenchmarkHistogram100SampleRate1(b *testing.B) {
	benchHistogram(b, 100, 1.0)
}

func BenchmarkHistogram1000SampleRate1(b *testing.B) {
	benchHistogram(b, 1000, 1.0)
}

func BenchmarkHistogram10000SampleRate1(b *testing.B) {
	benchHistogram(b, 10000, 1.0)
}

func BenchmarkHistogram100000SampleRate1(b *testing.B) {
	benchHistogram(b, 100000, 1.0)
}

func BenchmarkHistogram2SampleRate05(b *testing.B) {
	benchHistogram(b, 2, 0.5)
}

func BenchmarkHistogram10SampleRate05(b *testing.B) {
	benchHistogram(b, 10, 0.5)
}

func BenchmarkHistogram100SampleRate05(b *testing.B) {
	benchHistogram(b, 100, 0.5)
}

func BenchmarkHistogram1000SampleRate05(b *testing.B) {
	benchHistogram(b, 1000, 0.5)
}

func BenchmarkHistogram10000SampleRate05(b *testing.B) {
	benchHistogram(b, 10000, 0.5)
}

func BenchmarkHistogram100000SampleRate05(b *testing.B) {
	benchHistogram(b, 100000, 0.5)
}

func BenchmarkHistogram2SampleRate02(b *testing.B) {
	benchHistogram(b, 2, 0.2)
}

func BenchmarkHistogram10SampleRate02(b *testing.B) {
	benchHistogram(b, 10, 0.2)
}

func BenchmarkHistogram100SampleRate02(b *testing.B) {
	benchHistogram(b, 100, 0.2)
}

func BenchmarkHistogram1000SampleRate02(b *testing.B) {
	benchHistogram(b, 1000, 0.2)
}

func BenchmarkHistogram10000SampleRate02(b *testing.B) {
	benchHistogram(b, 10000, 0.2)
}

func BenchmarkHistogram100000SampleRate02(b *testing.B) {
	benchHistogram(b, 100000, 0.2)
}

func TestHistogramUnitPropagation(t *testing.T) {
	aggregates := []string{"max", "avg", "count"}
	percentiles := []int{95}

	// Timing sample: unit must appear on every flushed serie.
	h := &Histogram{interval: 10, aggregates: aggregates, percentiles: percentiles}
	h.addSample(&MetricSample{Value: 100, SampleRate: 1, Unit: UnitMilliseconds}, 50)
	h.addSample(&MetricSample{Value: 200, SampleRate: 1, Unit: UnitMilliseconds}, 51)

	series, err := h.flush(60)
	require.Nil(t, err)
	require.NotEmpty(t, series)
	for _, s := range series {
		if s.NameSuffix == ".count" {
			assert.Emptyf(t, s.Unit, "serie %s should be dimensionless", s.NameSuffix)
		} else {
			assert.Equal(t, UnitMilliseconds, s.Unit, "serie %s should carry unit", s.NameSuffix)
		}
	}

	// Plain histogram sample (no unit): every flushed serie must have empty unit.
	h2 := &Histogram{interval: 10, aggregates: aggregates, percentiles: percentiles}
	h2.addSample(&MetricSample{Value: 100, SampleRate: 1}, 50)

	series2, err := h2.flush(60)
	require.Nil(t, err)
	require.NotEmpty(t, series2)
	for _, s := range series2 {
		assert.Equal(t, "", s.Unit, "serie %s should have no unit", s.NameSuffix)
	}
}

// benchHistogramMixedSign exercises the mixed-sign path of sampleSum by alternating
// positive and negative values. Used to measure the split-by-sign optimization; the
// all-positive benchHistogram above never reaches that code path.
func benchHistogramMixedSign(b *testing.B, number int, sampleRate float64) {
	cfg := setupConfig(b)
	for n := 0; n < b.N; n++ {
		h := NewHistogram(1, cfg)
		h.configure([]string{"max", "min", "median", "avg", "sum", "count"}, []int{20, 95, 80})
		pos := MetricSample{Value: 21, SampleRate: sampleRate}
		neg := MetricSample{Value: -21, SampleRate: sampleRate}
		for i := 0; i < number; i++ {
			if i%2 == 0 {
				h.addSample(&pos, 10)
			} else {
				h.addSample(&neg, 10)
			}
		}
		h.flush(10)
	}
}

func BenchmarkHistogram1000MixedSignSampleRate1(b *testing.B) {
	benchHistogramMixedSign(b, 1000, 1.0)
}

func BenchmarkHistogram10000MixedSignSampleRate1(b *testing.B) {
	benchHistogramMixedSign(b, 10000, 1.0)
}

func BenchmarkHistogram100000MixedSignSampleRate1(b *testing.B) {
	benchHistogramMixedSign(b, 100000, 1.0)
}

// benchSampleSum isolates sampleSum (no sort, no flush, no alloc in the hot loop) so the
// summation algorithm is actually what's being measured, not slice growth/sort/percentile
// computation. Samples are pre-sorted to match the sort.Sort invariant of flush().
func benchSampleSum(b *testing.B, values []float64) {
	h := &Histogram{
		samples:      make(weightSamples, len(values)),
		count:        float64(len(values)),
		sharedWeight: 1,
	}
	for i, v := range values {
		h.samples[i] = weightSample{value: v, weight: 1}
	}
	sort.Sort(h.samples)
	b.ReportAllocs()
	b.ResetTimer()
	var sink float64
	for n := 0; n < b.N; n++ {
		sink += h.sampleSum()
	}
	b.StopTimer()
	runtime.KeepAlive(sink)
}

func buildMixedSignValues(n int) []float64 {
	vs := make([]float64, n)
	for i := range vs {
		if i%2 == 0 {
			vs[i] = 21
		} else {
			vs[i] = -21
		}
	}
	return vs
}

func buildAllPositiveValues(n int) []float64 {
	vs := make([]float64, n)
	for i := range vs {
		vs[i] = 21
	}
	return vs
}

func BenchmarkSampleSum100AllPositive(b *testing.B) { benchSampleSum(b, buildAllPositiveValues(100)) }
func BenchmarkSampleSum10000AllPositive(b *testing.B) {
	benchSampleSum(b, buildAllPositiveValues(10000))
}
func BenchmarkSampleSum100000AllPositive(b *testing.B) {
	benchSampleSum(b, buildAllPositiveValues(100000))
}
func BenchmarkSampleSum100MixedSign(b *testing.B)    { benchSampleSum(b, buildMixedSignValues(100)) }
func BenchmarkSampleSum10000MixedSign(b *testing.B)  { benchSampleSum(b, buildMixedSignValues(10000)) }
func BenchmarkSampleSum100000MixedSign(b *testing.B) { benchSampleSum(b, buildMixedSignValues(100000)) }
