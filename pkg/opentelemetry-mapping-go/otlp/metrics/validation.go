// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

// Reasons a histogram data point is dropped. They mirror the reasons used by the
// OTLP metrics intake (ddoghq/dd-source#3197) so both ends of the pipeline name
// the same condition the same way.
const (
	dropReasonNoRecordedValue    = "no_recorded_value"
	dropReasonBucketCountTooHigh = "bucket_count_too_high"
)

// sketchMaxObservationCount is the largest number of observations a single bucket,
// or a data point as a whole, may carry on its way into an agent sketch. It bounds
// the count values, not how many buckets there are.
//
// A sketch bin holds its count in a uint16, so appendSafe (pkg/util/quantile/bin.go)
// turns a bucket count into one bin per 65535 observations, and insertCounts builds
// that whole slice before trimLeft caps it: memory and iterations scale with the
// count value. quantile.Config.MaxCount() is the largest count binLimit bins can
// represent; above it the sketch also overruns its bin budget and loses resolution
// in the low tail, with nothing reported anywhere.
var sketchMaxObservationCount = uint64(quantile.Default().MaxCount())

// exceedsSketchCapacity reports whether count is too large to be inserted into an
// agent sketch. Callers must pass the count that actually reaches the sketch: for
// a cumulative data point that is the delta from the previous point, not the raw
// lifetime count the point carries.
func exceedsSketchCapacity(count float64, maxObservationCount uint64) bool {
	return count > float64(maxObservationCount)
}

// validateExpHistogramDataPoint reports whether an exponential histogram data
// point must be dropped, why, and — for dropReasonBucketCountTooHigh — the
// offending count. reason is empty when drop is false.
//
// The limit is a parameter because it belongs to whatever consumes the point: a
// histogram forwarded without sketch conversion is bounded by what the backend
// rebuilds it into instead, which is a different number (see OTAGENT-1131).
func validateExpHistogramDataPoint(dp pmetric.ExponentialHistogramDataPoint, maxObservationCount uint64) (reason string, badCount uint64, drop bool) {
	if dp.Flags().NoRecordedValue() {
		return dropReasonNoRecordedValue, 0, true
	}

	// An early exit on the count the point claims to carry. The buckets are summed
	// below regardless: OTLP requires Count to equal their sum, but nothing
	// enforces it, and it is the sum that decides how many bins the sketch builds.
	if dp.Count() > maxObservationCount {
		return dropReasonBucketCountTooHigh, dp.Count(), true
	}

	// Observations live in three separate places, and all of them end up in the
	// same sketch, so the bound is on their total rather than on each bucket.
	total := dp.ZeroCount()
	if total > maxObservationCount {
		return dropReasonBucketCountTooHigh, total, true
	}

	for _, counts := range [2]pcommon.UInt64Slice{dp.Positive().BucketCounts(), dp.Negative().BucketCounts()} {
		for i := 0; i < counts.Len(); i++ {
			count := counts.At(i)
			// total is at most maxObservationCount here, so the subtraction cannot wrap.
			// Comparing this way also catches a sum that would overflow a uint64.
			if count > maxObservationCount-total {
				return dropReasonBucketCountTooHigh, count, true
			}
			total += count
		}
	}

	return "", 0, false
}

// warnDroppedDataPoint logs a dropped histogram data point. Only
// dropReasonBucketCountTooHigh is logged, and at most once per metric name: it
// means the sender is producing implausible input, whereas the other reasons are
// benign or high-volume and stay as silent as they were before this guard.
//
// badCount is the count that could not be accommodated: the total the point
// carries, or the bucket count that pushed that total past the limit.
func warnDroppedDataPoint(logger *zap.Logger, warned *sync.Map, metricName, reason string, badCount, maxObservationCount uint64) {
	if reason != dropReasonBucketCountTooHigh {
		return
	}

	w, _ := warned.LoadOrStore(metricName, &metricWarnings{})
	if mw := w.(*metricWarnings); mw.bucketCountTooHigh.Swap(true) {
		return
	}

	logger.Warn("Dropped histogram data point: too many observations for a distribution",
		zap.String("metric name", metricName),
		zap.Uint64("count", badCount),
		zap.Uint64("limit", maxObservationCount),
	)
}
