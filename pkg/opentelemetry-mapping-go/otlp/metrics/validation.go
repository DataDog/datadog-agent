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

// Drop reasons, named as the OTLP metrics intake names them (ddoghq/dd-source#3197).
const (
	dropReasonNoRecordedValue    = "no_recorded_value"
	dropReasonBucketCountTooHigh = "bucket_count_too_high"
)

// sketchMaxObservationCount is the most observations a data point may put into an
// agent sketch, all of its buckets together: appendSafe gives every 65535 of them a
// bin of their own, and trimLeft cannot fold full bins back together.
var sketchMaxObservationCount = uint64(quantile.Default().MaxCount())

// validateExpHistogramDataPoint reports whether an exponential histogram data point
// must be dropped, why, and how many observations it holds when that is the reason.
// They all go into one sketch, so the bound is on their total.
//
// The limit is a parameter: a histogram forwarded without sketch conversion is
// bounded by the backend instead, which is a different number (see OTAGENT-1131).
func validateExpHistogramDataPoint(dp pmetric.ExponentialHistogramDataPoint, maxObservationCount uint64) (reason string, observations float64, drop bool) {
	if dp.Flags().NoRecordedValue() {
		return dropReasonNoRecordedValue, 0, true
	}

	// A float64 total cannot overflow, and stays exact around the limit.
	total := float64(dp.ZeroCount())
	for _, counts := range [2]pcommon.UInt64Slice{dp.Positive().BucketCounts(), dp.Negative().BucketCounts()} {
		for i := 0; i < counts.Len(); i++ {
			total += float64(counts.At(i))
		}
	}

	if total > float64(maxObservationCount) {
		return dropReasonBucketCountTooHigh, total, true
	}

	return "", 0, false
}

// warnDroppedDataPoint logs a dropped data point once per metric name. Only
// dropReasonBucketCountTooHigh is logged: the other reasons are benign or
// high-volume, and stay as silent as they were before this guard.
func warnDroppedDataPoint(logger *zap.Logger, warned *sync.Map, metricName, reason string, observations float64, maxObservationCount uint64) {
	if reason != dropReasonBucketCountTooHigh {
		return
	}

	w, _ := warned.LoadOrStore(metricName, &metricWarnings{})
	if mw := w.(*metricWarnings); mw.bucketCountTooHigh.Swap(true) {
		return
	}

	logger.Warn("Dropped histogram data point: too many observations for a distribution",
		zap.String("metric name", metricName),
		zap.Float64("observations", observations),
		zap.Uint64("limit", maxObservationCount),
	)
}
