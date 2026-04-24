// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// seriesStatus holds point count and write generation for a single series.
// Used by bulkSeriesStatus and by detectors that want to skip series with no
// new data since the last detection pass.
type seriesStatus struct {
	pointCount      int
	writeGeneration int64
}

// bulkStatusReader is an optional optimization interface for StorageReader
// implementations that support batch status queries in a single lock acquisition.
type bulkStatusReader interface {
	BulkSeriesStatus(refs []observer.SeriesRef, endTime int64) []seriesStatus
}

// bulkSeriesStatus returns the point count and write generation for each ref.
// If storage implements bulkStatusReader (e.g. timeSeriesStorage), it uses a
// single lock acquisition. Otherwise falls back to individual PointCountUpTo +
// WriteGeneration calls per ref.
func bulkSeriesStatus(storage observer.StorageReader, refs []observer.SeriesRef, endTime int64) []seriesStatus {
	if br, ok := storage.(bulkStatusReader); ok {
		return br.BulkSeriesStatus(refs, endTime)
	}
	result := make([]seriesStatus, len(refs))
	for i, h := range refs {
		result[i] = seriesStatus{
			pointCount:      storage.PointCountUpTo(h, endTime),
			writeGeneration: storage.WriteGeneration(h),
		}
	}
	return result
}
