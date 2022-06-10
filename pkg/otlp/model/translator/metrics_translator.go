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
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/otlp/model/attributes"
	"github.com/DataDog/datadog-agent/pkg/otlp/model/internal/instrumentationlibrary"
	"github.com/DataDog/datadog-agent/pkg/quantile"
	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/store"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

const metricName string = "metric name"

// Translator is a metrics translator.
type Translator struct {
	prevPts *ttlCache
	logger  *zap.Logger
	cfg     translatorConfig
}

// New creates a new translator with given options.
func New(logger *zap.Logger, options ...Option) (*Translator, error) {
	cfg := translatorConfig{
		HistMode:                             HistogramModeDistributions,
		SendCountSum:                         false,
		Quantiles:                            false,
		SendMonotonic:                        true,
		ResourceAttributesAsTags:             false,
		InstrumentationLibraryMetadataAsTags: false,
		sweepInterval:                        1800,
		deltaTTL:                             3600,
		fallbackHostnameProvider:             &noHostProvider{},
	}

	for _, opt := range options {
		err := opt(&cfg)
		if err != nil {
			return nil, err
		}
	}

	if cfg.HistMode == HistogramModeNoBuckets && !cfg.SendCountSum {
		return nil, fmt.Errorf("no buckets mode and no send count sum are incompatible")
	}

	cache := newTTLCache(cfg.sweepInterval, cfg.deltaTTL)
	return &Translator{cache, logger, cfg}, nil
}

// isCumulativeMonotonic checks if a metric is a cumulative monotonic metric
func isCumulativeMonotonic(md pmetric.Metric) bool {
	switch md.DataType() {
	case pmetric.MetricDataTypeSum:
		return md.Sum().AggregationTemporality() == pmetric.MetricAggregationTemporalityCumulative &&
			md.Sum().IsMonotonic()
	}
	return false
}

// isSkippable checks if a value can be skipped (because it is not supported by the backend).
// It logs that the value is unsupported for debugging since this sometimes means there is a bug.
func (t *Translator) isSkippable(name string, v float64) bool {
	skippable := math.IsInf(v, 0) || math.IsNaN(v)
	if skippable {
		t.logger.Debug("Unsupported metric value", zap.String(metricName, name), zap.Float64("value", v))
	}
	return skippable
}

// mapNumberMetrics maps double datapoints into Datadog metrics
func (t *Translator) mapNumberMetrics(
	ctx context.Context,
	consumer TimeSeriesConsumer,
	dims *Dimensions,
	dt MetricDataType,
	slice pmetric.NumberDataPointSlice,
) {

	for i := 0; i < slice.Len(); i++ {
		p := slice.At(i)
		pointDims := dims.WithAttributeMap(p.Attributes())
		var val float64
		switch p.ValueType() {
		case pmetric.NumberDataPointValueTypeDouble:
			val = p.DoubleVal()
		case pmetric.NumberDataPointValueTypeInt:
			val = float64(p.IntVal())
		}

		if t.isSkippable(pointDims.name, val) {
			continue
		}

		consumer.ConsumeTimeSeries(ctx, pointDims, dt, uint64(p.Timestamp()), val)
	}
}

// mapNumberMonotonicMetrics maps monotonic datapoints into Datadog metrics
func (t *Translator) mapNumberMonotonicMetrics(
	ctx context.Context,
	consumer TimeSeriesConsumer,
	dims *Dimensions,
	slice pmetric.NumberDataPointSlice,
) {
	for i := 0; i < slice.Len(); i++ {
		p := slice.At(i)
		ts := uint64(p.Timestamp())
		startTs := uint64(p.StartTimestamp())
		pointDims := dims.WithAttributeMap(p.Attributes())

		var val float64
		switch p.ValueType() {
		case pmetric.NumberDataPointValueTypeDouble:
			val = p.DoubleVal()
		case pmetric.NumberDataPointValueTypeInt:
			val = float64(p.IntVal())
		}

		if t.isSkippable(pointDims.name, val) {
			continue
		}

		if dx, ok := t.prevPts.MonotonicDiff(pointDims, startTs, ts, val); ok {
			consumer.ConsumeTimeSeries(ctx, pointDims, Count, ts, dx)
		}
	}
}

func getBounds(p pmetric.HistogramDataPoint, idx int) (lowerBound float64, upperBound float64) {
	// See https://github.com/open-telemetry/opentelemetry-proto/blob/v0.10.0/opentelemetry/proto/metrics/v1/metrics.proto#L427-L439
	lowerBound = math.Inf(-1)
	upperBound = math.Inf(1)
	if idx > 0 {
		lowerBound = p.MExplicitBounds()[idx-1]
	}
	if idx < len(p.MExplicitBounds()) {
		upperBound = p.MExplicitBounds()[idx]
	}
	return
}

type histogramInfo struct {
	// sum of histogram (exact)
	sum float64
	// count of histogram (exact)
	count uint64
	// ok to use
	ok bool
}

func (t *Translator) getSketchBuckets(
	ctx context.Context,
	consumer SketchConsumer,
	pointDims *Dimensions,
	p pmetric.HistogramDataPoint,
	histInfo histogramInfo,
	delta bool,
) {
	startTs := uint64(p.StartTimestamp())
	ts := uint64(p.Timestamp())
	as := &quantile.Agent{}
	for j := range p.MBucketCounts() {
		lowerBound, upperBound := getBounds(p, j)

		// Compute temporary bucketTags to have unique keys in the t.prevPts cache for each bucket
		// The bucketTags are computed from the bounds before the InsertInterpolate fix is done,
		// otherwise in the case where p.MExplicitBounds() has a size of 1 (eg. [0]), the two buckets
		// would have the same bucketTags (lower_bound:0 and upper_bound:0), resulting in a buggy behavior.
		bucketDims := pointDims.AddTags(
			fmt.Sprintf("lower_bound:%s", formatFloat(lowerBound)),
			fmt.Sprintf("upper_bound:%s", formatFloat(upperBound)),
		)

		// InsertInterpolate doesn't work with an infinite bound; insert in to the bucket that contains the non-infinite bound
		// https://github.com/DataDog/datadog-agent/blob/7.31.0/pkg/aggregator/check_sampler.go#L107-L111
		if math.IsInf(upperBound, 1) {
			upperBound = lowerBound
		} else if math.IsInf(lowerBound, -1) {
			lowerBound = upperBound
		}

		count := p.MBucketCounts()[j]
		if delta {
			as.InsertInterpolate(lowerBound, upperBound, uint(count))
		} else if dx, ok := t.prevPts.Diff(bucketDims, startTs, ts, float64(count)); ok {
			as.InsertInterpolate(lowerBound, upperBound, uint(dx))
		}

	}

	sketch := as.Finish()
	if sketch != nil {
		if histInfo.ok {
			// override approximate sum, count and average in sketch with exact values if available.
			sketch.Basic.Cnt = int64(histInfo.count)
			sketch.Basic.Sum = histInfo.sum
			sketch.Basic.Avg = sketch.Basic.Sum / float64(sketch.Basic.Cnt)
		}
		consumer.ConsumeSketch(ctx, pointDims, ts, sketch)
	}
}

func (t *Translator) getLegacyBuckets(
	ctx context.Context,
	consumer TimeSeriesConsumer,
	pointDims *Dimensions,
	p pmetric.HistogramDataPoint,
	delta bool,
) {
	startTs := uint64(p.StartTimestamp())
	ts := uint64(p.Timestamp())
	// We have a single metric, 'bucket', which is tagged with the bucket bounds. See:
	// https://github.com/DataDog/integrations-core/blob/7.30.1/datadog_checks_base/datadog_checks/base/checks/openmetrics/v2/transformers/histogram.py
	baseBucketDims := pointDims.WithSuffix("bucket")
	for idx, val := range p.MBucketCounts() {
		lowerBound, upperBound := getBounds(p, idx)
		bucketDims := baseBucketDims.AddTags(
			fmt.Sprintf("lower_bound:%s", formatFloat(lowerBound)),
			fmt.Sprintf("upper_bound:%s", formatFloat(upperBound)),
		)

		count := float64(val)
		if delta {
			consumer.ConsumeTimeSeries(ctx, bucketDims, Count, ts, count)
		} else if dx, ok := t.prevPts.Diff(bucketDims, startTs, ts, count); ok {
			consumer.ConsumeTimeSeries(ctx, bucketDims, Count, ts, dx)
		}
	}
}

// mapHistogramMetrics maps double histogram metrics slices to Datadog metrics
//
// A Histogram metric has:
// - The count of values in the population
// - The sum of values in the population
// - A number of buckets, each of them having
//    - the bounds that define the bucket
//    - the count of the number of items in that bucket
//    - a sample value from each bucket
//
// We follow a similar approach to our OpenMetrics check:
// we report sum and count by default; buckets count can also
// be reported (opt-in) tagged by lower bound.
func (t *Translator) mapHistogramMetrics(
	ctx context.Context,
	consumer Consumer,
	dims *Dimensions,
	slice pmetric.HistogramDataPointSlice,
	delta bool,
) {
	for i := 0; i < slice.Len(); i++ {
		p := slice.At(i)
		startTs := uint64(p.StartTimestamp())
		ts := uint64(p.Timestamp())
		pointDims := dims.WithAttributeMap(p.Attributes())

		histInfo := histogramInfo{ok: true}

		countDims := pointDims.WithSuffix("count")
		if delta {
			histInfo.count = p.Count()
		} else if dx, ok := t.prevPts.Diff(countDims, startTs, ts, float64(p.Count())); ok {
			histInfo.count = uint64(dx)
		} else { // not ok
			histInfo.ok = false
		}

		sumDims := pointDims.WithSuffix("sum")
		if !t.isSkippable(sumDims.name, p.Sum()) {
			if delta {
				histInfo.sum = p.Sum()
			} else if dx, ok := t.prevPts.Diff(sumDims, startTs, ts, p.Sum()); ok {
				histInfo.sum = dx
			} else { // not ok
				histInfo.ok = false
			}
		} else { // skippable
			histInfo.ok = false
		}

		if t.cfg.SendCountSum && histInfo.ok {
			// We only send the sum and count if both values were ok.
			consumer.ConsumeTimeSeries(ctx, countDims, Count, ts, float64(histInfo.count))
			consumer.ConsumeTimeSeries(ctx, sumDims, Count, ts, histInfo.sum)
		}

		switch t.cfg.HistMode {
		case HistogramModeCounters:
			t.getLegacyBuckets(ctx, consumer, pointDims, p, delta)
		case HistogramModeDistributions:
			t.getSketchBuckets(ctx, consumer, pointDims, p, histInfo, delta)
		}
	}
}

// mapExponentialHistogramMetrics maps exponential histogram metrics slices to Datadog metrics
//
// An ExponentialHistogram metric has:
// - The count of values in the population
// - The sum of values in the population
// - A scale, from which the base of the exponential histogram is computed
// - Two bucket stores, each with:
//     - an offset
//     - a list of bucket counts
// - A count of zero values in the population
func (t *Translator) mapExponentialHistogramMetrics(
	ctx context.Context,
	consumer Consumer,
	dims *Dimensions,
	slice pmetric.ExponentialHistogramDataPointSlice,
	delta bool,
) {
	for i := 0; i < slice.Len(); i++ {
		p := slice.At(i)
		startTs := uint64(p.StartTimestamp())
		ts := uint64(p.Timestamp())
		pointDims := dims.WithAttributeMap(p.Attributes())

		histInfo := histogramInfo{ok: true}

		countDims := pointDims.WithSuffix("count")
		if delta {
			histInfo.count = p.Count()
		} else if dx, ok := t.prevPts.Diff(countDims, startTs, ts, float64(p.Count())); ok {
			histInfo.count = uint64(dx)
		} else { // not ok
			histInfo.ok = false
		}

		sumDims := pointDims.WithSuffix("sum")
		if !t.isSkippable(sumDims.name, p.Sum()) {
			if delta {
				histInfo.sum = p.Sum()
			} else if dx, ok := t.prevPts.Diff(sumDims, startTs, ts, p.Sum()); ok {
				histInfo.sum = dx
			} else { // not ok
				histInfo.ok = false
			}
		} else { // skippable
			histInfo.ok = false
		}

		if t.cfg.SendCountSum && histInfo.ok {
			// We only send the sum and count if both values were ok.
			consumer.ConsumeTimeSeries(ctx, countDims, Count, ts, float64(histInfo.count))
			consumer.ConsumeTimeSeries(ctx, sumDims, Count, ts, histInfo.sum)
		}

		expHistDDSketch, err := t.exponentialHistogramToDDSketch(p, delta)
		if err != nil {
			t.logger.Debug("Failed to convert ExponentialHistogram into DDSketch",
				zap.String("metric name", dims.name),
				zap.Error(err),
			)
			continue
		}

		agentSketch, err := quantile.ConvertDDSketchIntoSketch(expHistDDSketch)
		if err != nil {
			t.logger.Debug("Failed to convert DDSketch into Sketch",
				zap.String("metric name", dims.name),
				zap.Error(err),
			)
		}

		if histInfo.ok {
			// override approximate sum, count and average in sketch with exact values if available.
			agentSketch.Basic.Cnt = int64(histInfo.count)
			agentSketch.Basic.Sum = histInfo.sum
			agentSketch.Basic.Avg = agentSketch.Basic.Sum / float64(agentSketch.Basic.Cnt)
		}

		consumer.ConsumeSketch(ctx, pointDims, ts, agentSketch)
	}
}

func (t *Translator) exponentialHistogramToDDSketch(
	p pmetric.ExponentialHistogramDataPoint,
	delta bool,
) (*ddsketch.DDSketch, error) {
	if !delta {
		return nil, fmt.Errorf("cumulative exponential histograms are not supported")
	}

	// Create the DDSketch stores
	positiveStore := toStore(p.Positive())
	negativeStore := toStore(p.Negative())

	// Create the DDSketch mapping that corresponds to the ExponentialHistogram settings
	gamma := math.Pow(2, math.Pow(2, float64(-p.Scale())))
	mapping, err := mapping.NewLogarithmicMappingWithGamma(gamma, 0)
	if err != nil {
		return nil, fmt.Errorf("couldn't create LogarithmicMapping for DDSketch: %w", err)
	}

	// Create DDSketch with the above mapping and stores
	sketch := ddsketch.NewDDSketch(mapping, positiveStore, negativeStore)
	err = sketch.AddWithCount(0, float64(p.ZeroCount()))
	if err != nil {
		return nil, fmt.Errorf("failed to add ZeroCount to DDSketch: %w", err)
	}

	return sketch, nil
}

func toStore(b pmetric.Buckets) store.Store {
	offset := b.Offset()
	bucketCounts := b.MBucketCounts()

	store := store.NewDenseStore()
	for j, count := range bucketCounts {
		// Find the real index of the bucket by adding the offset
		index := j + int(offset)

		store.AddWithCount(index, float64(count))
	}
	return store
}

// formatFloat formats a float number as close as possible to what
// we do on the Datadog Agent Python OpenMetrics check, which, in turn, tries to
// follow https://github.com/OpenObservability/OpenMetrics/blob/v1.0.0/specification/OpenMetrics.md#considerations-canonical-numbers
func formatFloat(f float64) string {
	if math.IsInf(f, 1) {
		return "inf"
	} else if math.IsInf(f, -1) {
		return "-inf"
	} else if math.IsNaN(f) {
		return "nan"
	} else if f == 0 {
		return "0"
	}

	// Add .0 to whole numbers
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if f == math.Floor(f) {
		s = s + ".0"
	}
	return s
}

// getQuantileTag returns the quantile tag for summary types.
func getQuantileTag(quantile float64) string {
	return fmt.Sprintf("quantile:%s", formatFloat(quantile))
}

// mapSummaryMetrics maps summary datapoints into Datadog metrics
func (t *Translator) mapSummaryMetrics(
	ctx context.Context,
	consumer TimeSeriesConsumer,
	dims *Dimensions,
	slice pmetric.SummaryDataPointSlice,
) {

	for i := 0; i < slice.Len(); i++ {
		p := slice.At(i)
		startTs := uint64(p.StartTimestamp())
		ts := uint64(p.Timestamp())
		pointDims := dims.WithAttributeMap(p.Attributes())

		// count and sum are increasing; we treat them as cumulative monotonic sums.
		{
			countDims := pointDims.WithSuffix("count")
			if dx, ok := t.prevPts.Diff(countDims, startTs, ts, float64(p.Count())); ok && !t.isSkippable(countDims.name, dx) {
				consumer.ConsumeTimeSeries(ctx, countDims, Count, ts, dx)
			}
		}

		{
			sumDims := pointDims.WithSuffix("sum")
			if !t.isSkippable(sumDims.name, p.Sum()) {
				if dx, ok := t.prevPts.Diff(sumDims, startTs, ts, p.Sum()); ok {
					consumer.ConsumeTimeSeries(ctx, sumDims, Count, ts, dx)
				}
			}
		}

		if t.cfg.Quantiles {
			baseQuantileDims := pointDims.WithSuffix("quantile")
			quantiles := p.QuantileValues()
			for i := 0; i < quantiles.Len(); i++ {
				q := quantiles.At(i)

				if t.isSkippable(baseQuantileDims.name, q.Value()) {
					continue
				}

				quantileDims := baseQuantileDims.AddTags(getQuantileTag(q.Quantile()))
				consumer.ConsumeTimeSeries(ctx, quantileDims, Gauge, ts, q.Value())
			}
		}
	}
}

// MapMetrics maps OTLP metrics into the DataDog format
func (t *Translator) MapMetrics(ctx context.Context, md pmetric.Metrics, consumer Consumer) error {
	rms := md.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		rm := rms.At(i)

		// Fetch tags from attributes.
		attributeTags := attributes.TagsFromAttributes(rm.Resource().Attributes())

		host, ok := attributes.HostnameFromAttributes(rm.Resource().Attributes())
		if !ok {
			var err error
			host, err = t.cfg.fallbackHostnameProvider.Hostname(context.Background())
			if err != nil {
				return fmt.Errorf("failed to get fallback host: %w", err)
			}
		}

		if host != "" {
			// Track hosts if the consumer is a HostConsumer.
			if c, ok := consumer.(HostConsumer); ok {
				c.ConsumeHost(host)
			}
		} else {
			// Track task ARN if the consumer is a TagsConsumer.
			if c, ok := consumer.(TagsConsumer); ok {
				tags := attributes.RunningTagsFromAttributes(rm.Resource().Attributes())
				for _, tag := range tags {
					c.ConsumeTag(tag)
				}
			}
		}

		ilms := rm.ScopeMetrics()
		for j := 0; j < ilms.Len(); j++ {
			ilm := ilms.At(j)
			metricsArray := ilm.Metrics()

			var additionalTags []string
			if t.cfg.InstrumentationLibraryMetadataAsTags {
				additionalTags = append(attributeTags, instrumentationlibrary.TagsFromInstrumentationLibraryMetadata(ilm.Scope())...)
			} else {
				additionalTags = attributeTags
			}

			for k := 0; k < metricsArray.Len(); k++ {
				md := metricsArray.At(k)
				baseDims := &Dimensions{
					name:     md.Name(),
					tags:     additionalTags,
					host:     host,
					originID: attributes.OriginIDFromAttributes(rm.Resource().Attributes()),
				}
				switch md.DataType() {
				case pmetric.MetricDataTypeGauge:
					t.mapNumberMetrics(ctx, consumer, baseDims, Gauge, md.Gauge().DataPoints())
				case pmetric.MetricDataTypeSum:
					switch md.Sum().AggregationTemporality() {
					case pmetric.MetricAggregationTemporalityCumulative:
						if t.cfg.SendMonotonic && isCumulativeMonotonic(md) {
							t.mapNumberMonotonicMetrics(ctx, consumer, baseDims, md.Sum().DataPoints())
						} else {
							t.mapNumberMetrics(ctx, consumer, baseDims, Gauge, md.Sum().DataPoints())
						}
					case pmetric.MetricAggregationTemporalityDelta:
						t.mapNumberMetrics(ctx, consumer, baseDims, Count, md.Sum().DataPoints())
					default: // pmetric.AggregationTemporalityUnspecified or any other not supported type
						t.logger.Debug("Unknown or unsupported aggregation temporality",
							zap.String(metricName, md.Name()),
							zap.Any("aggregation temporality", md.Sum().AggregationTemporality()),
						)
						continue
					}
				case pmetric.MetricDataTypeHistogram:
					switch md.Histogram().AggregationTemporality() {
					case pmetric.MetricAggregationTemporalityCumulative, pmetric.MetricAggregationTemporalityDelta:
						delta := md.Histogram().AggregationTemporality() == pmetric.MetricAggregationTemporalityDelta
						t.mapHistogramMetrics(ctx, consumer, baseDims, md.Histogram().DataPoints(), delta)
					default: // pmetric.AggregationTemporalityUnspecified or any other not supported type
						t.logger.Debug("Unknown or unsupported aggregation temporality",
							zap.String("metric name", md.Name()),
							zap.Any("aggregation temporality", md.Histogram().AggregationTemporality()),
						)
						continue
					}
				case pmetric.MetricDataTypeExponentialHistogram:
					switch md.ExponentialHistogram().AggregationTemporality() {
					case pmetric.MetricAggregationTemporalityDelta:
						delta := md.ExponentialHistogram().AggregationTemporality() == pmetric.MetricAggregationTemporalityDelta
						t.mapExponentialHistogramMetrics(ctx, consumer, baseDims, md.ExponentialHistogram().DataPoints(), delta)
					default: // pmetric.MetricAggregationTemporalityCumulative, pmetric.AggregationTemporalityUnspecified or any other not supported type
						t.logger.Debug("Unknown or unsupported aggregation temporality",
							zap.String("metric name", md.Name()),
							zap.Any("aggregation temporality", md.ExponentialHistogram().AggregationTemporality()),
						)
						continue
					}
				case pmetric.MetricDataTypeSummary:
					t.mapSummaryMetrics(ctx, consumer, baseDims, md.Summary().DataPoints())
				default: // pmetric.MetricDataTypeNone or any other not supported type
					t.logger.Debug("Unknown or unsupported metric type", zap.String(metricName, md.Name()), zap.Any("data type", md.DataType()))
					continue
				}
			}
		}
	}
	return nil
}
