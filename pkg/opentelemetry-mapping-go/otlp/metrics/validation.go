// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"math"
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

// sketchMaxObservationCount is the largest number of observations a data point may
// put into an agent sketch, all of its buckets together. It bounds the count
// values, not how many buckets there are.
//
// A sketch bin holds its count in a uint16, so appendSafe (pkg/util/quantile/bin.go)
// gives every 65535 observations a bin of their own, and trimLeft cannot fold full
// bins back together: the bins a sketch keeps, and the work to build it, grow with
// the number of observations. quantile.Config.MaxCount() is the most observations
// binLimit bins can hold; past it the sketch overruns its bin budget and loses
// resolution in the low tail, with nothing reported anywhere.
var sketchMaxObservationCount = uint64(quantile.Default().MaxCount())

// exceedsSketchCapacity reports whether count observations are too many for one
// agent sketch. Callers must pass what actually reaches the sketch: the total of
// the buckets inserted so far, which for a cumulative data point are the deltas
// from the previous point, not the lifetime counts the point carries.
func exceedsSketchCapacity(count float64, maxObservationCount uint64) bool {
	return count > float64(maxObservationCount)
}

// saturatingUint64 converts a non-negative total of observations to uint64,
// clamping it to math.MaxUint64: converting a float64 past the uint64 range
// gives an architecture-dependent result.
func saturatingUint64(total float64) uint64 {
	if total >= math.MaxUint64 {
		return math.MaxUint64
	}
	return uint64(total)
}

// validateExpHistogramDataPoint reports whether an exponential histogram data
// point must be dropped, why, and — for dropReasonBucketCountTooHigh — the
// observations counted when the limit was crossed. reason is empty when drop is
// false.
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
	// same sketch, so the bound is on their total rather than on each bucket. The
	// total is a float64 so that it cannot overflow; every value near the limit is
	// still exact.
	total := float64(dp.ZeroCount())
	if exceedsSketchCapacity(total, maxObservationCount) {
		return dropReasonBucketCountTooHigh, saturatingUint64(total), true
	}

	for _, counts := range [2]pcommon.UInt64Slice{dp.Positive().BucketCounts(), dp.Negative().BucketCounts()} {
		for i := 0; i < counts.Len(); i++ {
			total += float64(counts.At(i))
			if exceedsSketchCapacity(total, maxObservationCount) {
				return dropReasonBucketCountTooHigh, saturatingUint64(total), true
			}
		}
	}

	return "", 0, false
}

// warnDroppedDataPoint logs a dropped histogram data point. Only
// dropReasonBucketCountTooHigh is logged, and at most once per metric name: it
// means the sender is producing implausible input, whereas the other reasons are
// benign or high-volume and stay as silent as they were before this guard.
//
// badCount is the number of observations that did not fit: the total the point
// declares, or the total of its buckets up to the one that crossed the limit.
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
