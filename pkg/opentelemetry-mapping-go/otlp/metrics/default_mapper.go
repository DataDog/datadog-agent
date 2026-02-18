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

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

// defaultMapper is the default implementation of the mapper interface.
// It provides the standard mapping logic for converting OTLP metrics to Datadog format.
type defaultMapper struct {
	prevPts *ttlCache
	logger  *zap.Logger
	cfg     translatorConfig
}

// newDefaultMapper creates a new defaultMapper with the given dependencies.
// defaultMapper implements mapper and uses Consumer which includes both sketch and histogram interfaces.
func newDefaultMapper(prevPts *ttlCache, logger *zap.Logger, cfg translatorConfig) mapper {
	return &defaultMapper{
		prevPts: prevPts,
		logger:  logger,
		cfg:     cfg,
	}
}

// MapNumberMetrics maps double datapoints into Datadog metrics
func (m *defaultMapper) MapNumberMetrics(
	ctx context.Context,
	consumer Consumer,
	dims *Dimensions,
	dt DataType,
	slice pmetric.NumberDataPointSlice,
) {
	mapNumberMetrics(ctx, consumer, dims, dt, slice, m.logger, m.cfg.InferDeltaInterval)
}

// MapHistogramMetrics maps double histogram metrics slices to Datadog metrics
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
func (m *defaultMapper) MapHistogramMetrics(
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
		} else if dx, ok := m.prevPts.Diff(countDims, startTs, ts, float64(p.Count())); ok {
			histInfo.count = uint64(dx)
		} else { // not ok
			histInfo.ok = false
		}

		sumDims := pointDims.WithSuffix("sum")
		if !isSkippable(m.logger, sumDims.name, p.Sum()) {
			if delta {
				histInfo.sum = p.Sum()
			} else if dx, ok := m.prevPts.Diff(sumDims, startTs, ts, p.Sum()); ok {
				histInfo.sum = dx
			} else { // not ok
				histInfo.ok = false
			}
		} else { // skippable
			histInfo.ok = false
		}

		minDims := pointDims.WithSuffix("min")
		if p.HasMin() {
			histInfo.hasMinFromLastTimeWindow = delta || m.prevPts.PutAndCheckMin(minDims, startTs, ts, p.Min())
		}

		maxDims := pointDims.WithSuffix("max")
		if p.HasMax() {
			histInfo.hasMaxFromLastTimeWindow = delta || m.prevPts.PutAndCheckMax(maxDims, startTs, ts, p.Max())
		}

		if m.cfg.SendHistogramAggregations && histInfo.ok {
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

		switch m.cfg.HistMode {
		case HistogramModeCounters:
			m.getLegacyBuckets(ctx, consumer, pointDims, p, delta)
		case HistogramModeDistributions:
			err := m.getSketchBuckets(ctx, consumer, pointDims, p, histInfo, delta)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// MapSummaryMetrics maps summary datapoints into Datadog metrics
func (m *defaultMapper) MapSummaryMetrics(
	ctx context.Context,
	consumer Consumer,
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
			dx, isFirstPoint, shouldDropPoint := m.prevPts.MonotonicDiff(countDims, startTs, ts, val)
			if !shouldDropPoint && !isSkippable(m.logger, countDims.name, dx) {
				if !isFirstPoint {
					consumer.ConsumeTimeSeries(ctx, countDims, Count, ts, 0, dx)
				} else if i == 0 && shouldConsumeInitialValue(m.cfg.InitialCumulMonoValueMode, startTs, ts) {
					// We only compute the first point in the timeseries if it is the first value in the datapoint slice.
					consumer.ConsumeTimeSeries(ctx, countDims, Count, ts, 0, val)
				}
			}
		}

		{
			sumDims := pointDims.WithSuffix("sum")
			if !isSkippable(m.logger, sumDims.name, p.Sum()) {
				if dx, ok := m.prevPts.Diff(sumDims, startTs, ts, p.Sum()); ok {
					consumer.ConsumeTimeSeries(ctx, sumDims, Count, ts, 0, dx)
				}
			}
		}

		if m.cfg.Quantiles {
			baseQuantileDims := pointDims.WithSuffix("quantile")
			quantiles := p.QuantileValues()
			for i := 0; i < quantiles.Len(); i++ {
				q := quantiles.At(i)

				if isSkippable(m.logger, baseQuantileDims.name, q.Value()) {
					continue
				}

				quantileDims := baseQuantileDims.AddTags(getQuantileTag(q.Quantile()))
				consumer.ConsumeTimeSeries(ctx, quantileDims, Gauge, ts, 0, q.Value())
			}
		}
	}
}

// MapExponentialHistogramMetrics maps exponential histogram metrics slices to Datadog metrics
//
// An ExponentialHistogram metric has:
// - The count of values in the population
// - The sum of values in the population
// - A scale, from which the base of the exponential histogram is computed
// - Two bucket stores, each with:
//   - an offset
//   - a list of bucket counts
//
// - A count of zero values in the population
func (m *defaultMapper) MapExponentialHistogramMetrics(
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
		} else if dx, ok := m.prevPts.Diff(countDims, startTs, ts, float64(p.Count())); ok {
			histInfo.count = uint64(dx)
		} else { // not ok
			histInfo.ok = false
		}

		sumDims := pointDims.WithSuffix("sum")
		if !isSkippable(m.logger, sumDims.name, p.Sum()) {
			if delta {
				histInfo.sum = p.Sum()
			} else if dx, ok := m.prevPts.Diff(sumDims, startTs, ts, p.Sum()); ok {
				histInfo.sum = dx
			} else { // not ok
				histInfo.ok = false
			}
		} else { // skippable
			histInfo.ok = false
		}

		if m.cfg.SendHistogramAggregations && histInfo.ok {
			// We only send the sum and count if both values were ok.
			consumer.ConsumeTimeSeries(ctx, countDims, Count, ts, 0, float64(histInfo.count))
			consumer.ConsumeTimeSeries(ctx, sumDims, Count, ts, 0, histInfo.sum)

			if delta {
				if p.HasMin() {
					minDims := pointDims.WithSuffix("min")
					consumer.ConsumeTimeSeries(ctx, minDims, Gauge, ts, 0, p.Min())
				}
				if p.HasMax() {
					maxDims := pointDims.WithSuffix("max")
					consumer.ConsumeTimeSeries(ctx, maxDims, Gauge, ts, 0, p.Max())
				}
			}
		}

		expHistDDSketch, err := m.exponentialHistogramToDDSketch(p, delta)
		if err != nil {
			m.logger.Debug("Failed to convert ExponentialHistogram into DDSketch",
				zap.String("metric name", dims.name),
				zap.Error(err),
			)
			continue
		}

		agentSketch, err := quantile.ConvertDDSketchIntoSketch(expHistDDSketch)
		if err != nil {
			m.logger.Debug("Failed to convert DDSketch into Sketch",
				zap.String("metric name", dims.name),
				zap.Error(err),
			)
			continue
		}

		if histInfo.ok {
			// override approximate sum, count and average in sketch with exact values if available.
			agentSketch.Basic.Cnt = int64(histInfo.count)
			agentSketch.Basic.Sum = histInfo.sum
			agentSketch.Basic.Avg = agentSketch.Basic.Sum / float64(agentSketch.Basic.Cnt)

			if histInfo.count == 1 {
				// We know the exact value of this one point: it is the sum.
				// Override approximate min and max in that special case.
				agentSketch.Basic.Min = histInfo.sum
				agentSketch.Basic.Max = histInfo.sum
			}
		}
		if delta && p.HasMin() {
			agentSketch.Basic.Min = p.Min()
		}
		if delta && p.HasMax() {
			agentSketch.Basic.Max = p.Max()
		}

		consumer.ConsumeSketch(ctx, pointDims, ts, 0, agentSketch)
	}
}

// getSketchBuckets converts histogram buckets to a sketch
func (m *defaultMapper) getSketchBuckets(
	ctx context.Context,
	consumer Consumer,
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

		// Compute temporary bucketTags to have unique keys in the m.prevPts cache for each bucket
		// The bucketTags are computed from the bounds before the InsertInterpolate fix is done,
		// otherwise in the case where p.MExplicitBounds() has a size of 1 (eg. [0]), the two buckets
		// would have the same bucketTags (lower_bound:0 and upper_bound:0), resulting in a buggy behavior.
		bucketDims := pointDims.AddTags(
			"lower_bound:"+formatFloat(lowerBound),
			"upper_bound:"+formatFloat(upperBound),
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
		} else if dx, ok := m.prevPts.Diff(bucketDims, startTs, ts, float64(count)); ok {
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
		if m.cfg.InferDeltaInterval && delta {
			interval = inferDeltaInterval(startTs, ts)
		}
		consumer.ConsumeSketch(ctx, pointDims, ts, interval, sketch)
	}
	return nil
}

// getLegacyBuckets produces legacy bucket metrics
func (m *defaultMapper) getLegacyBuckets(
	ctx context.Context,
	consumer Consumer,
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
			"lower_bound:"+formatFloat(lowerBound),
			"upper_bound:"+formatFloat(upperBound),
		)

		count := float64(p.BucketCounts().At(idx))
		if delta {
			consumer.ConsumeTimeSeries(ctx, bucketDims, Count, ts, 0, count)
		} else if dx, ok := m.prevPts.Diff(bucketDims, startTs, ts, count); ok {
			consumer.ConsumeTimeSeries(ctx, bucketDims, Count, ts, 0, dx)
		}
	}
}

// exponentialHistogramToDDSketch converts an exponential histogram to a DDSketch
func (m *defaultMapper) exponentialHistogramToDDSketch(
	p pmetric.ExponentialHistogramDataPoint,
	delta bool,
) (*ddsketch.DDSketch, error) {
	if !delta {
		return nil, errors.New("cumulative exponential histograms are not supported")
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
