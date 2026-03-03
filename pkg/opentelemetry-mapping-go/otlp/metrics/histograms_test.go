// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package metrics

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
	"github.com/DataDog/sketches-go/ddsketch"
)

func TestDeltaHistogramTranslatorOptions(t *testing.T) {
	tests := []struct {
		name     string
		otlpfile string
		ddogfile string
		options  []TranslatorOption
		err      string
	}{
		{
			name:     "distributions",
			otlpfile: "test/otlp/hist/simple-delta.json",
			ddogfile: "test/datadog/hist/simple-delta_dist-nocs.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeDistributions),
			},
		},
		{
			name:     "distributions-test-min-max",
			otlpfile: "test/otlp/hist/simple-delta-min-max.json",
			ddogfile: "test/datadog/hist/simple-delta-min-max_dist-nocs.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeDistributions),
			},
		},
		{
			name:     "distributions-count-sum",
			otlpfile: "test/otlp/hist/simple-delta.json",
			ddogfile: "test/datadog/hist/simple-delta_dist-cs.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeDistributions),
				WithHistogramAggregations(),
			},
		},
		{
			name:     "zero-count-histogram",
			otlpfile: "test/otlp/hist/zero-delta.json",
			ddogfile: "test/datadog/hist/zero-delta_dist-cs.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeDistributions),
				WithHistogramAggregations(),
			},
		},
		{
			name:     "buckets",
			otlpfile: "test/otlp/hist/simple-delta.json",
			ddogfile: "test/datadog/hist/simple-delta_counters-nocs.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeCounters),
			},
		},
		{
			name:     "buckets-count-sum",
			otlpfile: "test/otlp/hist/simple-delta.json",
			ddogfile: "test/datadog/hist/simple-delta_counters-cs.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeCounters),
				WithHistogramAggregations(),
			},
		},
		{
			name:     "count-sum",
			otlpfile: "test/otlp/hist/simple-delta.json",
			ddogfile: "test/datadog/hist/simple-delta_nobuckets-cs.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeNoBuckets),
				WithHistogramAggregations(),
			},
		},
		{
			name:     "empty-delta-no-min-max",
			otlpfile: "test/otlp/hist/empty-delta-no-min-max.json",
			ddogfile: "test/datadog/hist/empty-delta-no-min-max.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeDistributions),
			},
		},
		{
			name:     "empty-delta-with-min-max",
			otlpfile: "test/otlp/hist/empty-delta-with-min-max.json",
			ddogfile: "test/datadog/hist/empty-delta-with-min-max.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeDistributions),
			},
		},
		{
			name:     "single-bucket-delta-no-min-max",
			otlpfile: "test/otlp/hist/single-bucket-delta-no-min-max.json",
			ddogfile: "test/datadog/hist/single-bucket-delta-no-min-max.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeDistributions),
			},
		},
		{
			name:     "single-bucket-delta-with-min-max",
			otlpfile: "test/otlp/hist/single-bucket-delta-min-max.json",
			ddogfile: "test/datadog/hist/single-bucket-delta-min-max.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeDistributions),
			},
		},
		{
			name: "no-count-sum-no-buckets",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeNoBuckets),
			},
			err: errNoBucketsNoSumCount,
		},
	}

	for _, testinstance := range tests {
		t.Run(testinstance.name, func(t *testing.T) {
			options := append(
				[]TranslatorOption{WithOriginProduct(OriginProductDatadogAgent)},
				testinstance.options...,
			)
			set := componenttest.NewNopTelemetrySettings()
			attributesTranslator, err := attributes.NewTranslator(set)
			require.NoError(t, err)
			translator, err := NewDefaultTranslator(set, attributesTranslator, options...)
			if testinstance.err != "" {
				assert.EqualError(t, err, testinstance.err)
				return
			}
			require.NoError(t, err)
			AssertTranslatorMap(t, translator, testinstance.otlpfile, testinstance.ddogfile)
		})
	}
}

const nonMonotonicTestFile = "test/otlp/hist/simple-delta-non-monotonic-bound.json"

func TestNonMonotonicCount(t *testing.T) {
	// Unmarshal OTLP data.
	otlpbytes, err := os.ReadFile(nonMonotonicTestFile)
	require.NoError(t, err, "failed to read OTLP file %q", nonMonotonicTestFile)

	var unmarshaler pmetric.JSONUnmarshaler
	otlpdata, err := unmarshaler.UnmarshalMetrics(otlpbytes)
	require.NoError(t, err, "failed to unmarshal OTLP data from file %q", nonMonotonicTestFile)
	// Map metrics using translator.
	consumer := newTestConsumer()
	options := []TranslatorOption{WithOriginProduct(OriginProductDatadogAgent)}
	set := componenttest.NewNopTelemetrySettings()
	attributesTranslator, err := attributes.NewTranslator(set)
	assert.NoError(t, err)

	translator, err := NewDefaultTranslator(set, attributesTranslator, options...)
	assert.NoError(t, err)

	_, err = translator.MapMetrics(context.Background(), otlpdata, &consumer, nil)
	assert.EqualError(t, err, quantile.ErrNonMonotonicBoundaries)
}

func TestCumulativeHistogramTranslatorOptions(t *testing.T) {
	tests := []struct {
		name     string
		otlpfile string
		ddogfile string
		options  []TranslatorOption
	}{
		{
			name:     "distributions",
			otlpfile: "test/otlp/hist/simple-cum.json",
			ddogfile: "test/datadog/hist/simple-cum_dist-nocs.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeDistributions),
			},
		},
		{
			name:     "distributions",
			otlpfile: "test/otlp/hist/static-cum.json",
			ddogfile: "test/datadog/hist/static-cum_dist-cs.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeDistributions),
				WithHistogramAggregations(),
			},
		},
		{
			name:     "distributions-count-sum",
			otlpfile: "test/otlp/hist/simple-cum.json",
			ddogfile: "test/datadog/hist/simple-cum_dist-cs.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeDistributions),
				WithHistogramAggregations(),
			},
		},
		{
			name:     "buckets",
			otlpfile: "test/otlp/hist/simple-cum.json",
			ddogfile: "test/datadog/hist/simple-cum_counters-nocs.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeCounters),
			},
		},
		{
			name:     "buckets-count-sum",
			otlpfile: "test/otlp/hist/simple-cum.json",
			ddogfile: "test/datadog/hist/simple-cum_counters-cs.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeCounters),
				WithHistogramAggregations(),
			},
		},
		{
			name:     "count-sum",
			otlpfile: "test/otlp/hist/simple-cum.json",
			ddogfile: "test/datadog/hist/simple-cum_nobuckets-cs.json",
			options: []TranslatorOption{
				WithHistogramMode(HistogramModeNoBuckets),
				WithHistogramAggregations(),
			},
		},
	}

	for _, testinstance := range tests {
		t.Run(testinstance.name, func(t *testing.T) {
			options := append(
				[]TranslatorOption{WithOriginProduct(OriginProductDatadogAgent)},
				testinstance.options...,
			)
			translator := NewTestTranslator(t, options...)
			AssertTranslatorMap(t, translator, testinstance.otlpfile, testinstance.ddogfile)
		})
	}
}

func TestExponentialHistogramTranslatorOptions(t *testing.T) {
	tests := []struct {
		name                                      string
		otlpfile                                  string
		ddogfile                                  string
		options                                   []TranslatorOption
		expectedUnknownMetricType                 int
		expectedUnsupportedAggregationTemporality int
	}{
		{
			name:                      "no-options",
			otlpfile:                  "test/otlp/hist/simple-exp.json",
			ddogfile:                  "test/datadog/hist/simple-exp.json",
			expectedUnknownMetricType: 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			// https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/26103
			name:     "empty-delta-issue-26103",
			otlpfile: "test/otlp/hist/empty-delta-exponential.json",
			ddogfile: "test/datadog/hist/empty-delta-exponential.json",
		},
		{
			// https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/26103
			name:     "empty-cumulative-issue-26103",
			otlpfile: "test/otlp/hist/empty-cum-exponential.json",
			ddogfile: "test/datadog/hist/empty-cum-exponential.json",
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:                      "resource-attributes-as-tags",
			otlpfile:                  "test/otlp/hist/simple-exp.json",
			ddogfile:                  "test/datadog/hist/simple-exp_res-tags.json",
			options:                   []TranslatorOption{},
			expectedUnknownMetricType: 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "count-sum",
			otlpfile: "test/otlp/hist/simple-exp.json",
			ddogfile: "test/datadog/hist/simple-exp_cs.json",
			options: []TranslatorOption{
				WithHistogramAggregations(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "instrumentation-library-metadata-as-tags",
			otlpfile: "test/otlp/hist/simple-exp.json",
			ddogfile: "test/datadog/hist/simple-exp_ilmd-tags.json",
			options: []TranslatorOption{
				WithInstrumentationLibraryMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "instrumentation-scope-metadata-as-tags",
			otlpfile: "test/otlp/hist/simple-exp.json",
			ddogfile: "test/datadog/hist/simple-exp_ismd-tags.json",
			options: []TranslatorOption{
				WithInstrumentationScopeMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "count-sum-instrumentation-library-metadata-as-tags",
			otlpfile: "test/otlp/hist/simple-exp.json",
			ddogfile: "test/datadog/hist/simple-exp_cs-ilmd-tags.json",
			options: []TranslatorOption{
				WithHistogramAggregations(),
				WithInstrumentationLibraryMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "resource-tags-instrumentation-library-metadata-as-tags",
			otlpfile: "test/otlp/hist/simple-exp.json",
			ddogfile: "test/datadog/hist/simple-exp_res-ilmd-tags.json",
			options: []TranslatorOption{
				WithInstrumentationLibraryMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "count-sum-resource-tags-instrumentation-library-metadata-as-tags",
			otlpfile: "test/otlp/hist/simple-exp.json",
			ddogfile: "test/datadog/hist/simple-exp_cs-both-tags.json",
			options: []TranslatorOption{
				WithHistogramAggregations(),
				WithInstrumentationLibraryMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "with-all",
			otlpfile: "test/otlp/hist/simple-exp.json",
			ddogfile: "test/datadog/hist/simple-exp_all.json",
			options: []TranslatorOption{
				WithHistogramAggregations(),
				WithInstrumentationLibraryMetadataAsTags(),
				WithInstrumentationScopeMetadataAsTags(),
			},
			expectedUnknownMetricType:                 1,
			expectedUnsupportedAggregationTemporality: 1,
		},
		{
			name:     "single-point-no-min-max",
			otlpfile: "test/otlp/hist/single-point-exp-no-min-max.json",
			ddogfile: "test/datadog/hist/single-point-exp-no-min-max.json",
		},
	}

	for _, testinstance := range tests {
		t.Run(testinstance.name, func(t *testing.T) {
			set := componenttest.NewNopTelemetrySettings()
			core, observed := observer.New(zapcore.DebugLevel)
			options := append(
				[]TranslatorOption{WithOriginProduct(OriginProductDatadogAgent)},
				testinstance.options...,
			)
			set.Logger = zap.New(core)
			attributesTranslator, err := attributes.NewTranslator(set)
			require.NoError(t, err)
			translator, err := NewDefaultTranslator(set, attributesTranslator, options...)
			require.NoError(t, err)
			AssertTranslatorMap(t, translator, testinstance.otlpfile, testinstance.ddogfile)
			assert.Equal(t, testinstance.expectedUnknownMetricType, observed.FilterMessage("Unknown or unsupported metric type").Len())
			assert.Equal(t, testinstance.expectedUnsupportedAggregationTemporality, observed.FilterMessage("Unknown or unsupported aggregation temporality").Len())
		})
	}
}

func TestCreateDDSketchFromHistogramOfDuration(t *testing.T) {
	tests := []struct {
		name        string
		unit        string
		bounds      []float64
		counts      []uint64
		min         *float64
		max         *float64
		hasError    bool
		expectedP50 float64
		expectedP95 float64
	}{
		{
			name:        "heavy skew to first bucket - ms",
			unit:        "ms",
			bounds:      []float64{10.0, 100.0, 1000.0},
			counts:      []uint64{1000, 5, 2, 1},          // 95% in first bucket
			expectedP50: 10.0 * float64(time.Millisecond), // p50 is in the first bucket, will be placed at min
			expectedP95: 10.0 * float64(time.Millisecond), // same bucket
		},
		{
			name:        "midpoints - ns",
			unit:        "ns",
			bounds:      []float64{100.0, 200.0, 300.0},
			counts:      []uint64{100, 100, 100, 0}, // 100 in (-inf, 100), 100 in [100, 200), 100 in [200, 300), 0 in [300, +inf)
			expectedP50: 150.0,                      // p50 is in the second bucket
			expectedP95: 250.0,                      // p95 is in the third bucket
		},
		{
			name:        "heavy skew to last bucket - s",
			unit:        "s",
			bounds:      []float64{1.0, 2.0, 3.0},
			counts:      []uint64{1, 2, 5, 1000},    // 95% in last bucket
			expectedP50: 3.0 * float64(time.Second), // p50 is in the last bucket, will be placed at max
			expectedP95: 3.0 * float64(time.Second), // same bucket
		},
		{
			name:   "empty histogram",
			unit:   "ms",
			bounds: []float64{1.0, 5.0},
			counts: []uint64{0, 0, 0},
		},
		{
			name:        "single value with min/max - us",
			unit:        "us",
			bounds:      []float64{1000.0},
			counts:      []uint64{0, 100}, // all values in [1000, +inf)
			min:         func() *float64 { v := 1200.0; return &v }(),
			max:         func() *float64 { v := 1200.0; return &v }(),
			expectedP50: 1.20e+06, // p50 is in the last bucket, will be placed at max
			expectedP95: 1.20e+06, // same bucket
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create histogram data point
			dp := pmetric.NewHistogramDataPoint()

			// Set bounds
			bounds := dp.ExplicitBounds()
			for _, bound := range tt.bounds {
				bounds.Append(bound)
			}

			// Set counts
			counts := dp.BucketCounts()
			for _, count := range tt.counts {
				counts.Append(count)
			}

			// Set min/max if provided
			if tt.min != nil {
				dp.SetMin(*tt.min)
			}
			if tt.max != nil {
				dp.SetMax(*tt.max)
			}

			// Test the function
			sketch, err := CreateDDSketchFromHistogramOfDuration(&dp, tt.unit)
			if tt.hasError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, sketch)

			// Only test percentiles if we have non-zero counts
			totalCount := uint64(0)
			for _, count := range tt.counts {
				totalCount += count
			}
			if totalCount > 0 && tt.expectedP50 > 0 {
				// Test that percentiles match expected values within DDSketch accuracy of 1%
				p50, err := sketch.GetValueAtQuantile(0.5)
				assert.NoError(t, err)
				assert.InDelta(t, tt.expectedP50, p50, tt.expectedP50*0.01, "p50 should be within relative accuracy")

				p95, err := sketch.GetValueAtQuantile(0.95)
				assert.NoError(t, err)
				assert.InDelta(t, tt.expectedP95, p95, tt.expectedP95*0.01, "p95 should be within relative accuracy")
			}
		})
	}
}

func TestCreateDDSketchFromHistogramOfDuration_Nil(t *testing.T) {
	sketch, err := CreateDDSketchFromHistogramOfDuration(nil, "ms")
	assert.NoError(t, err)
	assert.NotNil(t, sketch)
	assert.Equal(t, 0.0, sketch.GetCount())
	assert.Equal(t, 0.0, sketch.GetSum())
	assert.Equal(t, 0.0, sketch.GetZeroCount())
}

func TestCreateDDSketchFromExponentialHistogramOfDuration(t *testing.T) {
	tests := []struct {
		name        string
		unit        string
		scale       int32
		zeroCount   uint64
		posCounts   []uint64
		posOffset   int32
		negCounts   []uint64
		negOffset   int32
		hasError    bool
		expectedP50 float64
		expectedP95 float64
	}{
		{
			name:        "heavily skewed to zero - ms",
			unit:        "ms",
			scale:       4,                            // fine granularity
			zeroCount:   500,                          // 50% at zero
			posCounts:   []uint64{100, 100, 100, 100}, // decreasing counts
			posOffset:   0,
			expectedP50: 0.0,                                 // median is zero
			expectedP95: 1.13879 * float64(time.Millisecond), // bucket 3 lower bound scaled to ns
		},
		{
			name:        "uniform exponential - ns",
			unit:        "ns",
			scale:       2,
			zeroCount:   0,
			posCounts:   []uint64{100, 100, 100, 100, 100}, // uniform distribution
			posOffset:   0,
			expectedP50: 1.42, // bucket 3 lower bound scaled to ns
			expectedP95: 2.0,  // bucket 4 lower bound scaled to ns
		},
		{
			name:        "single bucket with known values - us",
			unit:        "us",
			scale:       0, // coarse scale
			zeroCount:   0,
			posCounts:   []uint64{1000},                // all in one bucket
			posOffset:   1,                             // bucket index 1
			expectedP50: 2 * float64(time.Microsecond), // bucket 1's lower bound scaled to ns
			expectedP95: 2 * float64(time.Microsecond), // same bucket
		},
		{
			name:      "empty exponential histogram",
			unit:      "s",
			scale:     1,
			zeroCount: 0,
			posCounts: []uint64{},
			posOffset: 0,
		},
		{
			name:        "mostly in high buckets - s",
			unit:        "s",
			scale:       3,
			zeroCount:   1,
			posCounts:   []uint64{1, 1, 10, 100}, // 90% in last bucket
			posOffset:   2,
			expectedP50: 1.54221 * float64(time.Second), // bucket 3 (+ offset 2) lower bound scaled to ns
			expectedP95: 1.54221 * float64(time.Second), // same bucket
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create exponential histogram data point
			dp := pmetric.NewExponentialHistogramDataPoint()
			dp.SetScale(tt.scale)
			dp.SetZeroCount(tt.zeroCount)

			// Set positive buckets
			if len(tt.posCounts) > 0 {
				posBuckets := dp.Positive()
				posBuckets.SetOffset(tt.posOffset)
				counts := posBuckets.BucketCounts()
				totalCount := tt.zeroCount
				for _, count := range tt.posCounts {
					counts.Append(count)
					totalCount += count
				}
				dp.SetCount(totalCount)
			}

			// Set negative buckets if provided
			if len(tt.negCounts) > 0 {
				negBuckets := dp.Negative()
				negBuckets.SetOffset(tt.negOffset)
				counts := negBuckets.BucketCounts()
				totalCount := dp.Count()
				for _, count := range tt.negCounts {
					counts.Append(count)
					totalCount += count
				}
				dp.SetCount(totalCount)
			}

			// Test the function
			sketch, err := CreateDDSketchFromExponentialHistogramOfDuration(&dp, tt.scale, tt.unit)
			if tt.hasError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, sketch)

			// Only test percentiles if we have expected values and non-zero counts
			totalCount := tt.zeroCount
			for _, count := range tt.posCounts {
				totalCount += count
			}
			for _, count := range tt.negCounts {
				totalCount += count
			}

			fmt.Println("=== TEST ===")
			fmt.Println(tt.name)
			fmt.Println("=== Data Point ===")
			b := PrettyPrintEHDP(dp)
			fmt.Println(string(b))
			fmt.Println("=== Sketch ===")
			fmt.Println(PrettyPrintDDSketch(sketch))
			if totalCount > 0 {
				// Test that percentiles match expected values within DDSketch accuracy of 1%
				p50, err := sketch.GetValueAtQuantile(0.5)
				fmt.Println("p50 is: ", p50)
				assert.NoError(t, err)
				assert.InDelta(t, tt.expectedP50, p50, tt.expectedP50*0.01, "p50 should be within tolerance")

				p95, err := sketch.GetValueAtQuantile(0.95)
				fmt.Println("p95 is: ", p95)
				assert.NoError(t, err)
				assert.InDelta(t, tt.expectedP95, p95, tt.expectedP95*0.01, "p95 should be within tolerance")
			}
		})
	}
}

func TestCreateDDSketchFromExponentialHistogramOfDuration_Nil(t *testing.T) {
	sketch, err := CreateDDSketchFromExponentialHistogramOfDuration(nil, 0, "ms")
	assert.NoError(t, err)
	assert.NotNil(t, sketch)
	assert.Equal(t, 0.0, sketch.GetCount())
	assert.Equal(t, 0.0, sketch.GetSum())
	assert.Equal(t, 0.0, sketch.GetZeroCount())
}

// PrettyPrintEHDP returns a readable string for one ExponentialHistogramDataPoint.
func PrettyPrintEHDP(dp pmetric.ExponentialHistogramDataPoint) string {
	var b strings.Builder

	// Basic stats
	var minStr, maxStr string
	if dp.HasMin() {
		minStr = fmt.Sprintf("%.6g", dp.Min())
	} else {
		minStr = "nil"
	}
	if dp.HasMax() {
		maxStr = fmt.Sprintf("%.6g", dp.Max())
	} else {
		maxStr = "nil"
	}

	fmt.Fprintf(&b, "ExponentialHistogramDataPoint {\n")
	fmt.Fprintf(&b, "  Count: %d\n", dp.Count())
	fmt.Fprintf(&b, "  Sum: %.6g\n", dp.Sum())
	fmt.Fprintf(&b, "  Min: %s, Max: %s\n", minStr, maxStr)
	fmt.Fprintf(&b, "  Scale: %d\n", dp.Scale())
	fmt.Fprintf(&b, "  ZeroCount: %d\n", dp.ZeroCount())

	// Bucket base r = 2^(2^(-scale))
	r := math.Pow(2, math.Pow(2, float64(-dp.Scale())))

	// Positive buckets: indices [offset, offset+len-1] cover [r^i, r^(i+1))
	fmt.Fprintf(&b, "  Positive Buckets (offset=%d):\n", dp.Positive().Offset())
	for j := 0; j < dp.Positive().BucketCounts().Len(); j++ {
		c := dp.Positive().BucketCounts().At(j)
		i := int(dp.Positive().Offset()) + int(j)
		lb := math.Pow(r, float64(i))
		ub := math.Pow(r, float64(i+1))
		fmt.Fprintf(&b, "    [% .6g, % .6g): %d\n", lb, ub, c)
	}

	// Negative buckets: indices [offset, offset+len-1] cover (-r^(i+1), -r^i]
	fmt.Fprintf(&b, "  Negative Buckets (offset=%d):\n", dp.Negative().Offset())
	for j := 0; j < dp.Negative().BucketCounts().Len(); j++ {
		c := dp.Negative().BucketCounts().At(j)
		i := int(dp.Negative().Offset()) + int(j)
		lb := -math.Pow(r, float64(i+1))
		ub := -math.Pow(r, float64(i))
		fmt.Fprintf(&b, "    (% .6g, % .6g]: %d\n", lb, ub, c)
	}

	fmt.Fprintf(&b, "}\n")
	return b.String()
}

func PrettyPrintDDSketch(sketch *ddsketch.DDSketch) string {
	var b strings.Builder

	// Basic stats
	fmt.Fprintf(&b, "DDSketch {\n")
	fmt.Fprintf(&b, "  Count: %.0f\n", sketch.GetCount())
	fmt.Fprintf(&b, "  Sum: %.6g\n", sketch.GetSum())
	fmt.Fprintf(&b, "  ZeroCount: %.0f\n", sketch.GetZeroCount())

	min, _ := sketch.GetMinValue()
	max, _ := sketch.GetMaxValue()
	fmt.Fprintf(&b, "  Min: %.6g, Max: %.6g\n", min, max)

	// Quantiles
	fmt.Fprintf(&b, "  Quantiles:\n")
	for _, q := range []float64{0.5, 0.75, 0.90, 0.95, 0.99} {
		val, err := sketch.GetValueAtQuantile(q)
		if err == nil {
			fmt.Fprintf(&b, "    p%.0f: %.6g\n", q*100, val)
		}
	}

	// Stores (bins)
	fmt.Fprintf(&b, "  Positive Store:\n")
	sketch.GetPositiveValueStore().ForEach(func(index int, count float64) bool {
		v := sketch.Value(index)
		fmt.Fprintf(&b, "    index=%d count=%.2f value=%.2f\n", index, count, v)
		return false // continue
	})

	fmt.Fprintf(&b, "  Negative Store:\n")
	sketch.GetNegativeValueStore().ForEach(func(index int, count float64) bool {
		v := sketch.Value(index)
		fmt.Fprintf(&b, "    index=%d count=%.2f value=%.2f\n", index, count, v)
		return false // continue
	})

	fmt.Fprintf(&b, "}\n")
	return b.String()
}
