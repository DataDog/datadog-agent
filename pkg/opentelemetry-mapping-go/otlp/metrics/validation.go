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

// validateHistogramDataPoint reports whether an explicit-bounds histogram data
// point must be dropped, why, and — for dropReasonBucketCountTooHigh — the
// offending count. reason is empty when drop is false.
//
// The limit is a parameter because it belongs to whatever consumes the point: a
// histogram forwarded without sketch conversion is bounded by what the backend
// rebuilds it into instead, which is a different number (see OTAGENT-1131).
func validateHistogramDataPoint(dp pmetric.HistogramDataPoint, maxObservationCount uint64) (reason string, badCount uint64, drop bool) {
	if dp.Flags().NoRecordedValue() {
		return dropReasonNoRecordedValue, 0, true
	}

	// The total count bounds the number of bins the whole point can produce, which
	// per-bucket limits alone do not. The intake validates buckets only.
	if dp.Count() > maxObservationCount {
		return dropReasonBucketCountTooHigh, dp.Count(), true
	}

	if count, ok := firstCountAbove(dp.BucketCounts(), maxObservationCount); ok {
		return dropReasonBucketCountTooHigh, count, true
	}

	return "", 0, false
}

// validateExpHistogramDataPoint reports whether an exponential histogram data
// point must be dropped, why, and — for dropReasonBucketCountTooHigh — the
// offending count. reason is empty when drop is false.
func validateExpHistogramDataPoint(dp pmetric.ExponentialHistogramDataPoint, maxObservationCount uint64) (reason string, badCount uint64, drop bool) {
	if dp.Flags().NoRecordedValue() {
		return dropReasonNoRecordedValue, 0, true
	}

	if dp.Count() > maxObservationCount {
		return dropReasonBucketCountTooHigh, dp.Count(), true
	}

	// Observations live in three separate places, all of which reach the sketch.
	if dp.ZeroCount() > maxObservationCount {
		return dropReasonBucketCountTooHigh, dp.ZeroCount(), true
	}

	if count, ok := firstCountAbove(dp.Positive().BucketCounts(), maxObservationCount); ok {
		return dropReasonBucketCountTooHigh, count, true
	}

	if count, ok := firstCountAbove(dp.Negative().BucketCounts(), maxObservationCount); ok {
		return dropReasonBucketCountTooHigh, count, true
	}

	return "", 0, false
}

// firstCountAbove returns the first bucket count greater than maxCount, if any.
// It indexes the slice rather than calling AsRaw, which would copy every bucket
// of every data point.
func firstCountAbove(counts pcommon.UInt64Slice, maxCount uint64) (uint64, bool) {
	for i := 0; i < counts.Len(); i++ {
		if count := counts.At(i); count > maxCount {
			return count, true
		}
	}

	return 0, false
}

// warnDroppedDataPoint logs a dropped histogram data point. Only
// dropReasonBucketCountTooHigh is logged, and at most once per metric name: it
// means the sender is producing implausible input, whereas the other reasons are
// benign or high-volume and stay as silent as they were before this guard.
func warnDroppedDataPoint(logger *zap.Logger, warned *sync.Map, metricName, reason string, badCount, maxObservationCount uint64) {
	if reason != dropReasonBucketCountTooHigh {
		return
	}

	w, _ := warned.LoadOrStore(metricName, &metricWarnings{})
	if mw := w.(*metricWarnings); mw.bucketCountTooHigh.Swap(true) {
		return
	}

	logger.Warn("Dropped histogram data point: bucket count too high",
		zap.String("metric name", metricName),
		zap.Uint64("bucket count", badCount),
		zap.Uint64("limit", maxObservationCount),
	)
}
