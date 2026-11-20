// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"math"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// directWindow reads into detector-owned scratch space. Production storage's
// existing snapshot primitive avoids escaping one callback closure per scope.
// Instrumented readers and test doubles retain the ordinary reader contract.
func directWindow(storage observer.StorageReader, ref observer.SeriesRef, start, end int64, agg observer.Aggregate, dst []observer.Point) (observer.Series, []observer.Point) {
	if reader, ok := storage.(interface {
		snapshotRange(observer.SeriesRef, int64, int64, observer.Aggregate, []observer.Point) (observer.Series, []observer.Point, bool)
	}); ok {
		series, points, ok := reader.snapshotRange(ref, start, end, agg, dst)
		if !ok {
			points = points[:0]
		}
		return series, points
	}
	var series observer.Series
	dst = dst[:0]
	storage.ForEachPoint(ref, start, end, agg, func(s *observer.Series, p observer.Point) {
		series = *s
		dst = append(dst, p)
	})
	return series, dst
}

// countFailureWindow counts distinct occupied intervals, rather than samples.
// Negative or zero values cannot create or renew failure evidence.
func countFailureWindow(points []observer.Point, interval int64) (total float64, occupied int, latest int64) {
	lastInterval := int64(-1)
	for _, point := range points {
		if !(point.Value > 0) || math.IsInf(point.Value, 0) {
			continue
		}
		total += point.Value
		if math.IsInf(total, 1) {
			total = math.MaxFloat64
		}
		bucket := point.Timestamp / interval
		if bucket != lastInterval {
			occupied++
			lastInterval = bucket
		}
		latest = point.Timestamp
	}
	return
}

func isDirectFailureDetector(name string) bool {
	return name == LogFailureDetectorName
}

// Apply exclusions before selection/model initialization, not after Detect has
// already paid for the statistical models. Registration filters are owned copies.
func withDirectFailureFilters(detectors []detectorRegistration) []detectorRegistration {
	if !hasLogFailureDetector(detectors) {
		return detectors
	}
	out := append([]detectorRegistration(nil), detectors...)
	for i := range out {
		if isDirectFailureDetector(out[i].detector.Name()) {
			continue
		}
		namespaces := append([]string(nil), out[i].filter.ExcludeNamespaces...)
		out[i].filter.ExcludeNamespaces = append(namespaces, LogErrorCountExtractorName)
	}
	return out
}
