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
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics/internal/instrumentationlibrary"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics/internal/instrumentationscope"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

const (
	metricName             string = "metric name"
	errNoBucketsNoSumCount string = "no buckets mode and no send count sum are incompatible"

	// intervalTolerance is the tolerance for interval calculation in seconds
	// We use 0.05 seconds as tolerance to allow for some jitter.
	intervalTolerance float64 = 0.05
)

var (
	signalTypeSet      = attribute.NewSet(attribute.String("signal", "metrics"))
	rateAsGaugeMetrics = map[string]struct{}{
		"kafka.net.bytes_out.rate":                        {},
		"kafka.net.bytes_in.rate":                         {},
		"kafka.replication.isr_shrinks.rate":              {},
		"kafka.replication.isr_expands.rate":              {},
		"kafka.replication.leader_elections.rate":         {},
		"jvm.gc.minor_collection_count":                   {},
		"jvm.gc.major_collection_count":                   {},
		"jvm.gc.minor_collection_time":                    {},
		"jvm.gc.major_collection_time":                    {},
		"kafka.messages_in.rate":                          {},
		"kafka.request.produce.failed.rate":               {},
		"kafka.request.fetch.failed.rate":                 {},
		"kafka.replication.unclean_leader_elections.rate": {},
		"kafka.log.flush_rate.rate":                       {},
	}
)

// inferDeltaInterval calculates the interval for Datadog counts from OTLP delta sums.
// It returns the interval in seconds if the time difference between start and end timestamps
// is close to a whole number of seconds (within intervalTolerance), otherwise returns 0.
func inferDeltaInterval(startTimestamp, timestamp uint64) int64 {
	if startTimestamp == 0 {
		// We can't infer the interval without the startTimestamp
		return 0
	}

	if startTimestamp > timestamp {
		// malformed data
		return 0
	}

	// Convert nanoseconds to seconds
	deltaSeconds := float64(timestamp-startTimestamp) / 1e9
	roundedDelta := math.Round(deltaSeconds)

	if math.Abs(roundedDelta-deltaSeconds) < intervalTolerance {
		return int64(roundedDelta)
	}

	// delta is outside of tolerance range
	return 0
}

var _ source.Provider = (*noSourceProvider)(nil)

type noSourceProvider struct{}

func (*noSourceProvider) Source(context.Context) (source.Source, error) {
	return source.Source{Kind: source.HostnameKind, Identifier: ""}, nil
}

// Translator is a metrics translator.
type Translator struct {
	prevPts              *ttlCache
	logger               *zap.Logger
	attributesTranslator *attributes.Translator
	cfg                  translatorConfig
}

// Metadata specifies information about the outcome of the MapMetrics call.
type Metadata struct {
	// Languages specifies a list of languages for which runtime metrics were found.
	Languages []string
}

// NewTranslator creates a new translator with given options.
func NewTranslator(set component.TelemetrySettings, attributesTranslator *attributes.Translator, options ...TranslatorOption) (*Translator, error) {
	cfg := translatorConfig{
		HistMode:                             HistogramModeDistributions,
		SendHistogramAggregations:            false,
		Quantiles:                            false,
		NumberMode:                           NumberModeCumulativeToDelta,
		InitialCumulMonoValueMode:            InitialCumulMonoValueModeAuto,
		InstrumentationLibraryMetadataAsTags: false,
		sweepInterval:                        1800,
		deltaTTL:                             3600,
		fallbackSourceProvider:               &noSourceProvider{},
		originProduct:                        OriginProductUnknown,
	}

	for _, opt := range options {
		err := opt(&cfg)
		if err != nil {
			return nil, err
		}
	}

	if cfg.HistMode == HistogramModeNoBuckets && !cfg.SendHistogramAggregations {
		return nil, errors.New(errNoBucketsNoSumCount)
	}

	cache := newTTLCache(cfg.sweepInterval, cfg.deltaTTL)

	return &Translator{
		prevPts:              cache,
		logger:               set.Logger.With(zap.String("component", "metrics translator")),
		attributesTranslator: attributesTranslator,
		cfg:                  cfg,
	}, nil
}

// isCumulativeMonotonic checks if a metric is a cumulative monotonic metric
func isCumulativeMonotonic(md pmetric.Metric) bool {
	switch md.Type() {
	case pmetric.MetricTypeSum:
		return md.Sum().AggregationTemporality() == pmetric.AggregationTemporalityCumulative &&
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
	dt DataType,
	slice pmetric.NumberDataPointSlice,
) {

	for i := 0; i < slice.Len(); i++ {
		p := slice.At(i)
		if p.Flags().NoRecordedValue() {
			// No recorded value, skip.
			continue
		}

		pointDims := dims.WithAttributeMap(p.Attributes())
		var val float64
		switch p.ValueType() {
		case pmetric.NumberDataPointValueTypeDouble:
			val = p.DoubleValue()
		case pmetric.NumberDataPointValueTypeInt:
			val = float64(p.IntValue())
		}

		if t.isSkippable(pointDims.name, val) {
			continue
		}

		// Calculate interval for Count type metrics (from OTLP delta sums)
		var interval int64
		if t.cfg.InferDeltaInterval && dt == Count {
			interval = inferDeltaInterval(uint64(p.StartTimestamp()), uint64(p.Timestamp()))
		}

		consumer.ConsumeTimeSeries(ctx, pointDims, dt, uint64(p.Timestamp()), interval, val)
	}
}

// TODO(songy23): consider changing this to a Translator start time that must be initialized
// if the package-level variable causes any issue.
var startTime = uint64(time.Now().UnixNano())

// getProcessStartTime returns the start time of the Agent process in seconds since epoch
func getProcessStartTime() uint64 {
	return startTime / 1_000_000_000
}

// getProcessStartTimeNano returns the start time of the Agent process in nanoseconds since epoch
func getProcessStartTimeNano() uint64 {
	return startTime
}

// shouldConsumeInitialValue checks if the initial value of a cumulative monotonic metric
// should be consumed or dropped.
func (t *Translator) shouldConsumeInitialValue(startTs, ts uint64) bool {
	switch t.cfg.InitialCumulMonoValueMode {
	case InitialCumulMonoValueModeAuto:
		if getProcessStartTimeNano() < startTs && startTs != ts {
			// Report the first value if the timeseries started after the Datadog Agent process started.
			return true
		}
	case InitialCumulMonoValueModeKeep:
		return true
	case InitialCumulMonoValueModeDrop:
		// do nothing, drop the point
	}
	return false
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
		if p.Flags().NoRecordedValue() {
			// No recorded value, skip.
			continue
		}

		ts := uint64(p.Timestamp())
		startTs := uint64(p.StartTimestamp())
		pointDims := dims.WithAttributeMap(p.Attributes())

		var val float64
		switch p.ValueType() {
		case pmetric.NumberDataPointValueTypeDouble:
			val = p.DoubleValue()
		case pmetric.NumberDataPointValueTypeInt:
			val = float64(p.IntValue())
		}

		if t.isSkippable(pointDims.name, val) {
			continue
		}

		if _, ok := rateAsGaugeMetrics[pointDims.name]; ok {
			dx, isFirstPoint, shouldDropPoint := t.prevPts.MonotonicRate(pointDims, startTs, ts, val)
			if shouldDropPoint {
				t.logger.Debug("Dropping point: timestamp is older or equal to timestamp of previous point received", zap.String(metricName, pointDims.name))
			} else if !isFirstPoint {
				consumer.ConsumeTimeSeries(ctx, pointDims, Gauge, ts, 0, dx)
			}
			continue
		}

		dx, isFirstPoint, shouldDropPoint := t.prevPts.MonotonicDiff(pointDims, startTs, ts, val)
		if shouldDropPoint {
			t.logger.Debug("Dropping point: timestamp is older or equal to timestamp of previous point received", zap.String(metricName, pointDims.name))
			continue
		}

		if !isFirstPoint {
			consumer.ConsumeTimeSeries(ctx, pointDims, Count, ts, 0, dx)
		} else if i == 0 && t.shouldConsumeInitialValue(startTs, ts) {
			// We only compute the first point in the timeseries if it is the first value in the datapoint slice.
			// Todo: Investigate why we don't compute first val if i > 0 and add reason as comment.
			consumer.ConsumeTimeSeries(ctx, pointDims, Count, ts, 0, val)
		}
	}
}

type histogramInfo struct {
	// sum of histogram (exact)
	sum float64
	// count of histogram (exact)
	count uint64

	// hasMinFromLastTimeWindow indicates whether the minimum was reached in the last time window.
	// If the minimum is NOT available, its value is false.
	hasMinFromLastTimeWindow bool

	// hasMaxFromLastTimeWindow indicates whether the maximum was reached in the last time window.
	// If the maximum is NOT available, its value is false.
	hasMaxFromLastTimeWindow bool

	// ok to use sum/count.
	ok bool
}

func (t *Translator) getSketchBuckets(
	ctx context.Context,
	consumer SketchConsumer,
	pointDims *Dimensions,
	p pmetric.HistogramDataPoint,
	histInfo histogramInfo,
	delta bool,
) error {
	startTs := uint64(p.StartTimestamp())
	ts := uint64(p.Timestamp())
	as := &quantile.Agent{}

	bucketCounts := p.BucketCounts()
	explicitBounds := p.ExplicitBounds()
	// From the spec (https://github.com/open-telemetry/opentelemetry-specification/blob/v1.29.0/specification/metrics/data-model.md#histogram):
	// > A Histogram without buckets conveys a population in terms of only the sum and count,
	// > and may be interpreted as a histogram with single bucket covering (-Inf, +Inf).
	if bucketCounts.Len() == 0 && histInfo.ok {
		bucketCounts = pcommon.NewUInt64Slice()
		explicitBounds = pcommon.NewFloat64Slice()

		if histInfo.hasMinFromLastTimeWindow {
			// Add an empty bucket from -inf to min.
			bucketCounts.Append(0)
			explicitBounds.Append(p.Min())
		}

		// Add a single bucket with the total histogram count to the sketch.
		bucketCounts.Append(histInfo.count)

		if histInfo.hasMaxFromLastTimeWindow {
			// Add an empty bucket from max to +inf.
			bucketCounts.Append(0)
			explicitBounds.Append(p.Max())
		}
	}

	// After the loop,
	// - minBound contains the lower bound of the lowest nonzero bucket,
	// - maxBound contains the upper bound of the highest nonzero bucket
	// - minBoundSet indicates if the minBound is set, effectively because
	//   there was at least a nonzero bucket.
	var minBound, maxBound float64
	var minBoundSet bool
	for j := 0; j < bucketCounts.Len(); j++ {
		lowerBound, upperBound := getBounds(explicitBounds, j)
		originalLowerBound, originalUpperBound := lowerBound, upperBound

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

		count := bucketCounts.At(j)
		var nonZeroBucket bool
		if delta {
			nonZeroBucket = count > 0
			err := as.InsertInterpolate(lowerBound, upperBound, uint(count))
			if err != nil {
				return err
			}
		} else if dx, ok := t.prevPts.Diff(bucketDims, startTs, ts, float64(count)); ok {
			nonZeroBucket = dx > 0
			err := as.InsertInterpolate(lowerBound, upperBound, uint(dx))
			if err != nil {
				return err
			}
		}

		if nonZeroBucket {
			if !minBoundSet {
				minBound = originalLowerBound
				minBoundSet = true
			}
			maxBound = originalUpperBound
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

		// If there is at least one bucket with nonzero count,
		// override min/max with bounds if they are not infinite.
		if minBoundSet {
			if !math.IsInf(minBound, 0) {
				sketch.Basic.Min = minBound
			}
			if !math.IsInf(maxBound, 0) {
				sketch.Basic.Max = maxBound
			}
		}

		if histInfo.hasMinFromLastTimeWindow {
			// We know exact minimum for the last time window.
			sketch.Basic.Min = p.Min()
		} else if p.HasMin() {
			// Clamp minimum with the global minimum (p.Min()) to account for sketch mapping error.
			sketch.Basic.Min = math.Max(p.Min(), sketch.Basic.Min)
		}

		if histInfo.hasMaxFromLastTimeWindow {
			// We know exact maximum for the last time window.
			sketch.Basic.Max = p.Max()
		} else if p.HasMax() {
			// Clamp maximum with global maximum (p.Max()) to account for sketch mapping error.
			sketch.Basic.Max = math.Min(p.Max(), sketch.Basic.Max)
		}

		var interval int64
		if t.cfg.InferDeltaInterval && delta {
			interval = inferDeltaInterval(startTs, ts)
		}
		consumer.ConsumeSketch(ctx, pointDims, ts, interval, sketch)
	}
	return nil
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
	for idx := 0; idx < p.BucketCounts().Len(); idx++ {
		lowerBound, upperBound := getBounds(p.ExplicitBounds(), idx)
		bucketDims := baseBucketDims.AddTags(
			fmt.Sprintf("lower_bound:%s", formatFloat(lowerBound)),
			fmt.Sprintf("upper_bound:%s", formatFloat(upperBound)),
		)

		count := float64(p.BucketCounts().At(idx))
		if delta {
			consumer.ConsumeTimeSeries(ctx, bucketDims, Count, ts, 0, count)
		} else if dx, ok := t.prevPts.Diff(bucketDims, startTs, ts, count); ok {
			consumer.ConsumeTimeSeries(ctx, bucketDims, Count, ts, 0, dx)
		}
	}
}

// mapHistogramMetrics maps double histogram metrics slices to Datadog metrics
//
// A Histogram metric has:
// - The count of values in the population
// - The sum of values in the population
// - A number of buckets, each of them having
//   - the bounds that define the bucket
//   - the count of the number of items in that bucket
//   - a sample value from each bucket
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
) error {
	for i := 0; i < slice.Len(); i++ {
		p := slice.At(i)
		if p.Flags().NoRecordedValue() {
			// No recorded value, skip.
			continue
		}

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

		minDims := pointDims.WithSuffix("min")
		if p.HasMin() {
			histInfo.hasMinFromLastTimeWindow = delta || t.prevPts.PutAndCheckMin(minDims, startTs, ts, p.Min())
		}

		maxDims := pointDims.WithSuffix("max")
		if p.HasMax() {
			histInfo.hasMaxFromLastTimeWindow = delta || t.prevPts.PutAndCheckMax(maxDims, startTs, ts, p.Max())
		}

		if t.cfg.SendHistogramAggregations && histInfo.ok {
			// We only send the sum and count if both values were ok.
			consumer.ConsumeTimeSeries(ctx, countDims, Count, ts, 0, float64(histInfo.count))
			consumer.ConsumeTimeSeries(ctx, sumDims, Count, ts, 0, histInfo.sum)

			if delta {
				// We could check is[Min/Max]FromLastTimeWindow here, and report the minimum/maximum
				// for cumulative timeseries when we know it. These would be metrics with progressively
				// less frequency which would be confusing, so we limit reporting these metrics to delta points,
				// where the min/max is (pressumably) available in either all or none of the points.

				if p.HasMin() {
					consumer.ConsumeTimeSeries(ctx, minDims, Gauge, ts, 0, p.Min())
				}
				if p.HasMax() {
					consumer.ConsumeTimeSeries(ctx, maxDims, Gauge, ts, 0, p.Max())
				}
			}
		}

		switch t.cfg.HistMode {
		case HistogramModeCounters:
			t.getLegacyBuckets(ctx, consumer, pointDims, p, delta)
		case HistogramModeDistributions:
			err := t.getSketchBuckets(ctx, consumer, pointDims, p, histInfo, delta)
			if err != nil {
				return err
			}
		}
	}
	return nil
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
		if p.Flags().NoRecordedValue() {
			// No recorded value, skip.
			continue
		}

		startTs := uint64(p.StartTimestamp())
		ts := uint64(p.Timestamp())
		pointDims := dims.WithAttributeMap(p.Attributes())

		// treat count as a cumulative monotonic metric
		// and sum as a non-monotonic metric
		// https://prometheus.io/docs/practices/histograms/#count-and-sum-of-observations
		{
			countDims := pointDims.WithSuffix("count")
			val := float64(p.Count())
			dx, isFirstPoint, shouldDropPoint := t.prevPts.MonotonicDiff(countDims, startTs, ts, val)
			if !shouldDropPoint && !t.isSkippable(countDims.name, dx) {
				if !isFirstPoint {
					consumer.ConsumeTimeSeries(ctx, countDims, Count, ts, 0, dx)
				} else if i == 0 && t.shouldConsumeInitialValue(startTs, ts) {
					// We only compute the first point in the timeseries if it is the first value in the datapoint slice.
					consumer.ConsumeTimeSeries(ctx, countDims, Count, ts, 0, val)
				}
			}
		}

		{
			sumDims := pointDims.WithSuffix("sum")
			if !t.isSkippable(sumDims.name, p.Sum()) {
				if dx, ok := t.prevPts.Diff(sumDims, startTs, ts, p.Sum()); ok {
					consumer.ConsumeTimeSeries(ctx, sumDims, Count, ts, 0, dx)
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
				consumer.ConsumeTimeSeries(ctx, quantileDims, Gauge, ts, 0, q.Value())
			}
		}
	}
}

func (t *Translator) source(ctx context.Context, res pcommon.Resource, hostFromAttributesHandler attributes.HostFromAttributesHandler) (source.Source, error) {
	src, hasSource := t.attributesTranslator.ResourceToSource(ctx, res, signalTypeSet, hostFromAttributesHandler)
	if !hasSource {
		var err error
		src, err = t.cfg.fallbackSourceProvider.Source(ctx)
		if err != nil {
			return source.Source{}, fmt.Errorf("failed to get fallback source: %w", err)
		}
	}
	return src, nil
}

// extractLanguageTag appends a new language tag to languageTags if a new language tag is found from the given name
func extractLanguageTag(name string, languageTags []string) []string {
	for prefix, lang := range runtimeMetricPrefixLanguageMap {
		if !slices.Contains(languageTags, lang) && strings.HasPrefix(name, prefix) {
			return append(languageTags, lang)
		}
	}
	return languageTags
}

// mapGaugeRuntimeMetricWithAttributes maps the specified runtime metric from metric attributes into a new Gauge metric
func mapGaugeRuntimeMetricWithAttributes(md pmetric.Metric, metricsArray pmetric.MetricSlice, mp runtimeMetricMapping) {
	for i := 0; i < md.Gauge().DataPoints().Len(); i++ {
		matchesAttributes := true
		for _, attribute := range mp.attributes {
			attributeValue, res := md.Gauge().DataPoints().At(i).Attributes().Get(attribute.key)
			if !res || !slices.Contains(attribute.values, attributeValue.AsString()) {
				matchesAttributes = false
				break
			}
		}
		if matchesAttributes {
			cp := metricsArray.AppendEmpty()
			cp.SetEmptyGauge()
			dataPoint := cp.Gauge().DataPoints().AppendEmpty()
			md.Gauge().DataPoints().At(i).CopyTo(dataPoint)
			dataPoint.Attributes().RemoveIf(func(s string, _ pcommon.Value) bool {
				for _, attribute := range mp.attributes {
					if s == attribute.key {
						return true
					}
				}
				return false
			})
			cp.SetName(mp.mappedName)
		}
	}
}

// mapSumRuntimeMetricWithAttributes maps the specified runtime metric from metric attributes into a new Sum metric
func mapSumRuntimeMetricWithAttributes(md pmetric.Metric, metricsArray pmetric.MetricSlice, mp runtimeMetricMapping) {
	for i := 0; i < md.Sum().DataPoints().Len(); i++ {
		matchesAttributes := true
		for _, attribute := range mp.attributes {
			attributeValue, res := md.Sum().DataPoints().At(i).Attributes().Get(attribute.key)
			if !res || !slices.Contains(attribute.values, attributeValue.AsString()) {
				matchesAttributes = false
				break
			}
		}
		if matchesAttributes {
			cp := metricsArray.AppendEmpty()
			cp.SetEmptySum()
			cp.Sum().SetAggregationTemporality(md.Sum().AggregationTemporality())
			cp.Sum().SetIsMonotonic(md.Sum().IsMonotonic())
			dataPoint := cp.Sum().DataPoints().AppendEmpty()
			md.Sum().DataPoints().At(i).CopyTo(dataPoint)
			dataPoint.Attributes().RemoveIf(func(s string, _ pcommon.Value) bool {
				for _, attribute := range mp.attributes {
					if s == attribute.key {
						return true
					}
				}
				return false
			})
			cp.SetName(mp.mappedName)
		}
	}
}

// mapHistogramRuntimeMetricWithAttributes maps the specified runtime metric from metric attributes into a new Histogram metric
func mapHistogramRuntimeMetricWithAttributes(md pmetric.Metric, metricsArray pmetric.MetricSlice, mp runtimeMetricMapping) {
	for i := 0; i < md.Histogram().DataPoints().Len(); i++ {
		matchesAttributes := true
		for _, attribute := range mp.attributes {
			attributeValue, res := md.Histogram().DataPoints().At(i).Attributes().Get(attribute.key)
			if !res || !slices.Contains(attribute.values, attributeValue.AsString()) {
				matchesAttributes = false
				break
			}
		}
		if matchesAttributes {
			cp := metricsArray.AppendEmpty()
			cp.SetEmptyHistogram()
			cp.Histogram().SetAggregationTemporality(md.Histogram().AggregationTemporality())
			dataPoint := cp.Histogram().DataPoints().AppendEmpty()
			md.Histogram().DataPoints().At(i).CopyTo(dataPoint)
			dataPoint.Attributes().RemoveIf(func(s string, _ pcommon.Value) bool {
				for _, attribute := range mp.attributes {
					if s == attribute.key {
						return true
					}
				}
				return false
			})
			cp.SetName(mp.mappedName)
			break
		}
	}
}

// MapMetrics maps OTLP metrics into the Datadog format
func (t *Translator) MapMetrics(ctx context.Context, md pmetric.Metrics, consumer Consumer, hostFromAttributesHandler attributes.HostFromAttributesHandler) (Metadata, error) {
	metadata := Metadata{
		Languages: []string{},
	}
	rms := md.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		rm := rms.At(i)
		src, err := t.source(ctx, rm.Resource(), hostFromAttributesHandler)
		if err != nil {
			return metadata, err
		}

		var host string
		if src.Kind == source.HostnameKind {
			host = src.Identifier
			// Don't consume the host yet, first check if we have any nonAPM metrics.
		}

		// seenNonAPMMetrics is used to determine if we have seen any non-APM metrics in this ResourceMetrics.
		// If we have only seen APM metrics, we don't want to consume the host.
		var seenNonAPMMetrics bool

		// Fetch tags from attributes.
		attributeTags := attributes.TagsFromAttributes(rm.Resource().Attributes())
		ilms := rm.ScopeMetrics()
		rattrs := rm.Resource().Attributes()
		for j := 0; j < ilms.Len(); j++ {
			ilm := ilms.At(j)
			metricsArray := ilm.Metrics()

			var additionalTags []string
			if t.cfg.InstrumentationScopeMetadataAsTags {
				additionalTags = append(attributeTags, instrumentationscope.TagsFromInstrumentationScopeMetadata(ilm.Scope())...)
			} else if t.cfg.InstrumentationLibraryMetadataAsTags {
				additionalTags = append(attributeTags, instrumentationlibrary.TagsFromInstrumentationLibraryMetadata(ilm.Scope())...)
			} else {
				additionalTags = attributeTags
			}

			scopeName := ilm.Scope().Name()

			newMetrics := pmetric.NewMetricSlice()
			for k := 0; k < metricsArray.Len(); k++ {
				md := metricsArray.At(k)
				if md.Name() == keyStatsPayload && md.Type() == pmetric.MetricTypeSum {

					// these metrics are an APM Stats payload; consume it as such
					for l := 0; l < md.Sum().DataPoints().Len(); l++ {
						if payload, ok := md.Sum().DataPoints().At(l).Attributes().Get(keyStatsPayload); ok && t.cfg.statsOut != nil && payload.Type() == pcommon.ValueTypeBytes {
							t.cfg.statsOut <- payload.Bytes().AsRaw()
						}
					}
					continue
				}
				if v, ok := runtimeMetricsMappings[md.Name()]; ok {
					metadata.Languages = extractLanguageTag(md.Name(), metadata.Languages)
					for _, mp := range v {
						if mp.attributes == nil {
							// duplicate runtime metrics as Datadog runtime metrics
							cp := newMetrics.AppendEmpty()
							md.CopyTo(cp)
							cp.SetName(mp.mappedName)
							break
						}
						if md.Type() == pmetric.MetricTypeSum {
							mapSumRuntimeMetricWithAttributes(md, newMetrics, mp)
						} else if md.Type() == pmetric.MetricTypeGauge {
							mapGaugeRuntimeMetricWithAttributes(md, newMetrics, mp)
						} else if md.Type() == pmetric.MetricTypeHistogram {
							mapHistogramRuntimeMetricWithAttributes(md, newMetrics, mp)
						}
					}
				} else {
					// If we are here, we have a non-APM metric:
					// it is not a stats metric, nor a runtime metric.
					seenNonAPMMetrics = true
				}

				if t.cfg.withRemapping {
					remapMetrics(newMetrics, md)
				}
				if t.cfg.withOTelPrefix {
					renameMetrics(md)
				}

				err := t.mapToDDFormat(ctx, md, consumer, additionalTags, host, scopeName, rattrs)
				if err != nil {
					return metadata, err
				}
			}

			for k := 0; k < newMetrics.Len(); k++ {
				md := newMetrics.At(k)
				err := t.mapToDDFormat(ctx, md, consumer, additionalTags, host, scopeName, rattrs)
				if err != nil {
					return metadata, err
				}
			}
		}

		// Only consume the source if we have seen non-APM metrics.
		if seenNonAPMMetrics {
			switch src.Kind {
			case source.HostnameKind:
				if c, ok := consumer.(HostConsumer); ok {
					c.ConsumeHost(host)
				}
			case source.AWSECSFargateKind:
				if c, ok := consumer.(TagsConsumer); ok {
					c.ConsumeTag(src.Tag())
				}
			}
		}
	}
	return metadata, nil
}

func (t *Translator) mapToDDFormat(ctx context.Context, md pmetric.Metric, consumer Consumer, additionalTags []string, host string, scopeName string, rattrs pcommon.Map) error {
	baseDims := &Dimensions{
		name:                md.Name(),
		tags:                additionalTags,
		host:                host,
		originID:            attributes.OriginIDFromAttributes(rattrs),
		originProduct:       t.cfg.originProduct,
		originSubProduct:    OriginSubProductOTLP,
		originProductDetail: originProductDetailFromScopeName(scopeName),
	}
	switch md.Type() {
	case pmetric.MetricTypeGauge:
		t.mapNumberMetrics(ctx, consumer, baseDims, Gauge, md.Gauge().DataPoints())
	case pmetric.MetricTypeSum:
		switch md.Sum().AggregationTemporality() {
		case pmetric.AggregationTemporalityCumulative:
			if isCumulativeMonotonic(md) {
				switch t.cfg.NumberMode {
				case NumberModeCumulativeToDelta:
					t.mapNumberMonotonicMetrics(ctx, consumer, baseDims, md.Sum().DataPoints())
				case NumberModeRawValue:
					t.mapNumberMetrics(ctx, consumer, baseDims, Gauge, md.Sum().DataPoints())
				}
			} else { // delta and cumulative non-monotonic sums
				t.mapNumberMetrics(ctx, consumer, baseDims, Gauge, md.Sum().DataPoints())
			}
		case pmetric.AggregationTemporalityDelta:
			t.mapNumberMetrics(ctx, consumer, baseDims, Count, md.Sum().DataPoints())
		default: // pmetric.AggregationTemporalityUnspecified or any other not supported type
			t.logger.Debug("Unknown or unsupported aggregation temporality",
				zap.String(metricName, md.Name()),
				zap.Any("aggregation temporality", md.Sum().AggregationTemporality()),
			)
		}
	case pmetric.MetricTypeHistogram:
		switch md.Histogram().AggregationTemporality() {
		case pmetric.AggregationTemporalityCumulative, pmetric.AggregationTemporalityDelta:
			delta := md.Histogram().AggregationTemporality() == pmetric.AggregationTemporalityDelta
			err := t.mapHistogramMetrics(ctx, consumer, baseDims, md.Histogram().DataPoints(), delta)
			if err != nil {
				return err
			}
		default: // pmetric.AggregationTemporalityUnspecified or any other not supported type
			t.logger.Debug("Unknown or unsupported aggregation temporality",
				zap.String("metric name", md.Name()),
				zap.Any("aggregation temporality", md.Histogram().AggregationTemporality()),
			)
		}
	case pmetric.MetricTypeExponentialHistogram:
		switch md.ExponentialHistogram().AggregationTemporality() {
		case pmetric.AggregationTemporalityDelta:
			delta := md.ExponentialHistogram().AggregationTemporality() == pmetric.AggregationTemporalityDelta
			t.mapExponentialHistogramMetrics(ctx, consumer, baseDims, md.ExponentialHistogram().DataPoints(), delta)
		default: // pmetric.AggregationTemporalityCumulative, pmetric.AggregationTemporalityUnspecified or any other not supported type
			t.logger.Debug("Unknown or unsupported aggregation temporality",
				zap.String("metric name", md.Name()),
				zap.Any("aggregation temporality", md.ExponentialHistogram().AggregationTemporality()),
			)
		}
	case pmetric.MetricTypeSummary:
		t.mapSummaryMetrics(ctx, consumer, baseDims, md.Summary().DataPoints())
	default: // pmetric.MetricDataTypeNone or any other not supported type
		t.logger.Debug("Unknown or unsupported metric type", zap.String(metricName, md.Name()), zap.Any("data type", md.Type()))
	}
	return nil
}
