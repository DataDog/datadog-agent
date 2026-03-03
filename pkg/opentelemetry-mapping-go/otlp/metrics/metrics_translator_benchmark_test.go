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

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
)

const (
	runtimeMetricWithMapping = "process.runtime.dotnet.gc.heap.size"
	runtimeMetricNoMapping   = "dotnet.gc.heap.size"
)

var inputTable = []struct {
	input int
}{
	{input: 10},
	{input: 100},
	{input: 1000},
	{input: 10000},
	{input: 100000},
	{input: 1000000},
	{input: 10000000},
}

func newBenchmarkTranslator(b *testing.B, _ *zap.Logger, opts ...TranslatorOption) *defaultTranslator {
	options := append([]TranslatorOption{
		WithFallbackSourceProvider(testProvider("fallbackHostname")),
		WithHistogramMode(HistogramModeDistributions),
		WithNumberMode(NumberModeCumulativeToDelta),
	}, opts...)

	return NewTestTranslator(b, options...)
}

// createBenchmarkGaugeMetrics creates n Gauge data points.
func createBenchmarkGaugeMetrics(n int, additionalAttributes map[string]string) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rms := md.ResourceMetrics()
	rm := rms.AppendEmpty()

	attrs := rm.Resource().Attributes()
	attrs.PutStr(attributes.AttributeDatadogHostname, testHostname)
	for attr, val := range additionalAttributes {
		attrs.PutStr(attr, val)
	}
	ilms := rm.ScopeMetrics()

	ilm := ilms.AppendEmpty()
	metricsArray := ilm.Metrics()
	metricsArray.AppendEmpty() // first one is TypeNone to test that it's ignored

	for i := 0; i < n; i++ {
		// IntGauge
		met := metricsArray.AppendEmpty()
		met.SetName(fmt.Sprintf("int.gauge.%d", i))
		met.SetEmptyGauge()
		dpsInt := met.Gauge().DataPoints()
		dpInt := dpsInt.AppendEmpty()
		dpInt.SetTimestamp(seconds(0))
		dpInt.SetIntValue(1)
	}

	return md
}

// createBenchmarkDeltaExponentialHistogramMetrics creates n ExponentialHistogram data points, each with b buckets
// in each store, with a delta aggregation temporality.
func createBenchmarkDeltaExponentialHistogramMetrics(n int, b int, additionalAttributes map[string]string) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rms := md.ResourceMetrics()
	rm := rms.AppendEmpty()

	resourceAttrs := rm.Resource().Attributes()
	resourceAttrs.PutStr(attributes.AttributeDatadogHostname, testHostname)
	for attr, val := range additionalAttributes {
		resourceAttrs.PutStr(attr, val)
	}

	ilms := rm.ScopeMetrics()
	ilm := ilms.AppendEmpty()
	metricsArray := ilm.Metrics()

	for i := 0; i < n; i++ {
		met := metricsArray.AppendEmpty()
		met.SetName("expHist.test")
		met.SetEmptyExponentialHistogram()
		met.ExponentialHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
		points := met.ExponentialHistogram().DataPoints()
		point := points.AppendEmpty()

		point.SetScale(6)

		point.SetCount(30)
		point.SetZeroCount(10)
		point.SetSum(math.Pi)

		buckets := make([]uint64, b)
		for i := 0; i < b; i++ {
			buckets[i] = 10
		}

		point.Negative().SetOffset(2)
		point.Negative().BucketCounts().FromRaw(buckets)

		point.Positive().SetOffset(3)
		point.Positive().BucketCounts().FromRaw(buckets)

		point.SetTimestamp(seconds(0))
	}

	return md
}

// createBenchmarkDeltaSumMetrics creates n Sum data points with a delta aggregation temporality.
func createBenchmarkDeltaSumMetrics(n int, additionalAttributes map[string]string) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rms := md.ResourceMetrics()
	rm := rms.AppendEmpty()

	attrs := rm.Resource().Attributes()
	attrs.PutStr(attributes.AttributeDatadogHostname, testHostname)
	for attr, val := range additionalAttributes {
		attrs.PutStr(attr, val)
	}
	ilms := rm.ScopeMetrics()

	ilm := ilms.AppendEmpty()
	metricsArray := ilm.Metrics()
	metricsArray.AppendEmpty() // first one is TypeNone to test that it's ignored

	for i := 0; i < n; i++ {
		met := metricsArray.AppendEmpty()
		met.SetName("double.delta.monotonic.sum")
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
		dpsDouble := met.Sum().DataPoints()
		dpDouble := dpsDouble.AppendEmpty()
		dpDouble.SetTimestamp(seconds(0))
		dpDouble.SetDoubleValue(math.E)
	}

	return md
}

func createBenchmarkRuntimeMetric(metricName string, metricType pmetric.MetricType, attributes []runtimeMetricAttribute, dataPoints int) pmetric.Metrics {
	md := pmetric.NewMetrics()
	met := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	met.SetName(metricName)
	var dpsInt pmetric.NumberDataPointSlice
	var hpsCount pmetric.HistogramDataPointSlice
	if metricType == pmetric.MetricTypeSum {
		met.SetEmptySum()
		met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		met.Sum().SetIsMonotonic(true)
		dpsInt = met.Sum().DataPoints()
	} else if metricType == pmetric.MetricTypeGauge {
		met.SetEmptyGauge()
		dpsInt = met.Gauge().DataPoints()
	} else if metricType == pmetric.MetricTypeHistogram {
		met.SetEmptyHistogram()
		met.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
		hpsCount = met.Histogram().DataPoints()
	}

	if metricType != pmetric.MetricTypeHistogram {
		dpsInt.EnsureCapacity(dataPoints)
		for i := 0; i < dataPoints; i++ {
			startTs := int(getProcessStartTime()) + 1
			dpInt := dpsInt.AppendEmpty()
			for _, attr := range attributes {
				dpInt.Attributes().PutStr(attr.key, attr.values[0])
			}
			dpInt.SetStartTimestamp(seconds(startTs))
			dpInt.SetTimestamp(seconds(startTs + 1 + i))
			dpInt.SetIntValue(int64(10 * (1 + i)))
		}
		return md
	}

	hpsCount.EnsureCapacity(dataPoints)
	for i := 0; i < dataPoints; i++ {
		startTs := int(getProcessStartTime()) + 1
		hpCount := hpsCount.AppendEmpty()
		for _, attr := range attributes {
			hpCount.Attributes().PutStr(attr.key, attr.values[0])
		}
		hpCount.SetStartTimestamp(seconds(startTs))
		hpCount.SetTimestamp(seconds(startTs + 1 + i))
		hpCount.ExplicitBounds().FromRaw([]float64{})
		hpCount.BucketCounts().FromRaw([]uint64{100})
		hpCount.SetCount(100)
		hpCount.SetSum(0)
		hpCount.SetMin(-100)
		hpCount.SetMax(100)
	}
	return md
}

func benchmarkMapMetrics(metrics pmetric.Metrics, b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		tr := newBenchmarkTranslator(b, zap.NewNop())
		consumer := &mockFullConsumer{}

		// Make deep copy of metrics to avoid mutation affecting benchmark tests
		metricsCopy := pmetric.NewMetrics()
		metrics.CopyTo(metricsCopy)
		_, err := tr.MapMetrics(ctx, metricsCopy, consumer, nil)
		assert.NoError(b, err)
	}
}

func BenchmarkMapDeltaExponentialHistogramMetrics1_5(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(1, 5, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics10_5(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(10, 5, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics100_5(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(100, 5, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics1000_5(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(1000, 5, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics10000_5(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(10000, 5, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics1_50(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(1, 50, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics10_50(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(10, 50, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics100_50(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(100, 50, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics1000_50(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(1000, 50, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics10000_50(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(10000, 50, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics1_500(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(1, 500, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics10_500(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(10, 500, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics100_500(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(100, 500, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics1000_500(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(1000, 500, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics10000_500(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(10000, 500, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics1_5000(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(1, 5000, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics10_5000(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(10, 5000, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics100_5000(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(100, 5000, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaExponentialHistogramMetrics1000_5000(b *testing.B) {
	metrics := createBenchmarkDeltaExponentialHistogramMetrics(1000, 5000, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapGaugeMetrics10(b *testing.B) {
	metrics := createBenchmarkGaugeMetrics(10, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapGaugeMetrics100(b *testing.B) {
	metrics := createBenchmarkGaugeMetrics(100, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapGaugeMetrics1000(b *testing.B) {
	metrics := createBenchmarkGaugeMetrics(1000, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapGaugeMetrics10000(b *testing.B) {
	metrics := createBenchmarkGaugeMetrics(10000, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapGaugeMetrics100000(b *testing.B) {
	metrics := createBenchmarkGaugeMetrics(100000, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapGaugeMetrics1000000(b *testing.B) {
	metrics := createBenchmarkGaugeMetrics(1000000, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapGaugeMetrics10000000(b *testing.B) {
	metrics := createBenchmarkGaugeMetrics(10000000, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaSumMetrics10(b *testing.B) {
	metrics := createBenchmarkDeltaSumMetrics(10, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaSumMetrics100(b *testing.B) {
	metrics := createBenchmarkDeltaSumMetrics(100, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaSumMetrics1000(b *testing.B) {
	metrics := createBenchmarkDeltaSumMetrics(1000, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaSumMetrics10000(b *testing.B) {
	metrics := createBenchmarkDeltaSumMetrics(10000, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaSumMetrics100000(b *testing.B) {
	metrics := createBenchmarkDeltaSumMetrics(100000, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaSumMetrics1000000(b *testing.B) {
	metrics := createBenchmarkDeltaSumMetrics(1000000, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapDeltaSumMetrics10000000(b *testing.B) {
	metrics := createBenchmarkDeltaSumMetrics(10000000, map[string]string{
		"attribute_tag": "attribute_value",
	})

	benchmarkMapMetrics(metrics, b)
}

func BenchmarkMapSumRuntimeMetric(b *testing.B) {
	for _, v := range inputTable {
		b.Run(fmt.Sprintf("BenchmarkMapRuntimeMetricsHasMapping-%d", v.input), func(b *testing.B) {
			benchmarkMapMetrics(createBenchmarkRuntimeMetric(runtimeMetricWithMapping, pmetric.MetricTypeSum, nil, v.input), b)
		})
		b.Run(fmt.Sprintf("BenchmarkMapRuntimeMetricsNoMapping-%d", v.input), func(b *testing.B) {
			benchmarkMapMetrics(createBenchmarkRuntimeMetric(runtimeMetricNoMapping, pmetric.MetricTypeSum, nil, v.input), b)
		})
	}
}

func BenchmarkMapGaugeRuntimeMetricWithAttributesHasMapping(b *testing.B) {
	attr := []runtimeMetricAttribute{{
		key:    "generation",
		values: []string{"gen1"},
	}}

	for _, v := range inputTable {
		b.Run(fmt.Sprintf("BenchmarkMapGaugeRuntimeMetricWithAttributesHasMapping-%d", v.input), func(b *testing.B) {
			benchmarkMapMetrics(createBenchmarkRuntimeMetric(runtimeMetricWithMapping, pmetric.MetricTypeGauge, attr, v.input), b)
		})
		b.Run(fmt.Sprintf("BenchmarkMapGaugeRuntimeMetricWithAttributesNoMapping-%d", v.input), func(b *testing.B) {
			benchmarkMapMetrics(createBenchmarkRuntimeMetric(runtimeMetricNoMapping, pmetric.MetricTypeGauge, attr, v.input), b)
		})
	}
}

func BenchmarkMapGaugeRuntimeMetricWith10AttributesHasMapping(b *testing.B) {
	var attr []runtimeMetricAttribute
	for i := 1; i <= 10; i++ {
		attr = append(attr, runtimeMetricAttribute{
			key:    "generation",
			values: []string{fmt.Sprintf("gen%d", i)},
		})
	}

	for _, v := range inputTable {
		b.Run(fmt.Sprintf("BenchmarkMapGaugeRuntimeMetricWith10AttributesHasMapping-%d", v.input), func(b *testing.B) {
			benchmarkMapMetrics(createBenchmarkRuntimeMetric(runtimeMetricWithMapping, pmetric.MetricTypeGauge, attr, v.input), b)
		})
		b.Run(fmt.Sprintf("BenchmarkMapGaugeRuntimeMetricWith10AttributesNoMapping-%d", v.input), func(b *testing.B) {
			benchmarkMapMetrics(createBenchmarkRuntimeMetric(runtimeMetricNoMapping, pmetric.MetricTypeGauge, attr, v.input), b)
		})
	}
}

func BenchmarkMapGaugeRuntimeMetricWith100AttributesHasMapping(b *testing.B) {
	var attr []runtimeMetricAttribute
	for i := 1; i <= 100; i++ {
		attr = append(attr, runtimeMetricAttribute{
			key:    "generation",
			values: []string{fmt.Sprintf("gen%d", i)},
		})
	}

	for _, v := range inputTable {
		b.Run(fmt.Sprintf("BenchmarkMapGaugeRuntimeMetricWith100AttributesHasMapping-%d", v.input), func(b *testing.B) {
			benchmarkMapMetrics(createBenchmarkRuntimeMetric(runtimeMetricWithMapping, pmetric.MetricTypeGauge, attr, v.input), b)
		})
		b.Run(fmt.Sprintf("BenchmarkMapGaugeRuntimeMetricWith100AttributesNoMapping-%d", v.input), func(b *testing.B) {
			benchmarkMapMetrics(createBenchmarkRuntimeMetric(runtimeMetricNoMapping, pmetric.MetricTypeGauge, attr, v.input), b)
		})
	}
}
