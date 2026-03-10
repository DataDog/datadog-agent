// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"sort"
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// detectorMedian computes the median of a float64 slice without modifying the input.
func detectorMedian(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// detectorMAD computes the Median Absolute Deviation from a given median.
// MAD = median(|x_i - median|).
// When scaleToSigma is true, the result is scaled by 1.4826 to estimate the
// standard deviation for normally distributed data. Use scaleToSigma=true when
// comparing against sigma-based thresholds (e.g. Mann-Whitney's deviation check),
// and false when using raw MAD as a denominator for relative change scores (e.g. TopK).
func detectorMAD(vals []float64, median float64, scaleToSigma bool) float64 {
	if len(vals) == 0 {
		return 0
	}
	absDevs := make([]float64, len(vals))
	for i, v := range vals {
		absDevs[i] = math.Abs(v - median)
	}
	sort.Float64s(absDevs)
	n := len(absDevs)
	var mad float64
	if n%2 == 0 {
		mad = (absDevs[n/2-1] + absDevs[n/2]) / 2
	} else {
		mad = absDevs[n/2]
	}
	if scaleToSigma {
		mad *= 1.4826
	}
	return mad
}

// detectorMeanValues computes the arithmetic mean of a float64 slice.
func detectorMeanValues(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

// detectorSampleStddev computes the sample standard deviation of a float64 slice.
func detectorSampleStddev(vals []float64, mean float64) float64 {
	n := len(vals)
	if n < 2 {
		return 0
	}
	var sumSq float64
	for _, v := range vals {
		d := v - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(n-1))
}

// detectorSeriesLabel builds a human-readable label from a SeriesKey.
// Format: "service/metricName" if a service tag exists, else "namespace/metricName".
func detectorSeriesLabel(key observer.SeriesKey) string {
	svc := detectorService(key)
	if svc != "" {
		return svc + "/" + key.Name
	}
	if key.Namespace != "" {
		return key.Namespace + "/" + key.Name
	}
	return key.Name
}

// detectorMetricID builds a metric identifier in "service:metricName" format,
// matching the scorer's expected format for service-level fallback matching.
func detectorMetricID(key observer.SeriesKey) string {
	svc := detectorService(key)
	if svc != "" {
		return svc + ":" + key.Name
	}
	return key.Name
}

// detectorService extracts the service name from a SeriesKey's tags.
func detectorService(key observer.SeriesKey) string {
	for _, tag := range key.Tags {
		if strings.HasPrefix(tag, "service:") {
			return tag[len("service:"):]
		}
	}
	return ""
}

// detectorHasServiceTag checks whether any of the tags is a service: tag.
func detectorHasServiceTag(tags []string) bool {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "service:") {
			return true
		}
	}
	return false
}
