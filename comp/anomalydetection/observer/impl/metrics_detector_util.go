// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// seriesStatus holds point count and write generation for a single series.
// Used by storage.BulkSeriesStatus and scan-based detectors (added in algorithm PRs).
type seriesStatus struct {
	pointCount      int
	writeGeneration int64
}

// bulkStatusReader, bulkSeriesStatus, detectorMedian, detectorMAD,
// medianPointInterval, rankBiserialCorrelation, normalCDFUpper are scan-detector
// utilities defined in algorithm PRs alongside ScanMW and ScanWelch.
