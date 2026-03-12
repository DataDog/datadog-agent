// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// mockLogView implements observer.LogView for testing.
type mockLogView struct {
	content     []byte
	status      string
	tags        []string
	hostname    string
	timestampMs int64
}

func (m *mockLogView) GetContent() []byte           { return m.content }
func (m *mockLogView) GetStatus() string            { return m.status }
func (m *mockLogView) GetTags() []string            { return m.tags }
func (m *mockLogView) GetHostname() string          { return m.hostname }
func (m *mockLogView) GetTimestampUnixMilli() int64 { return m.timestampMs }

// dynamicAnomalyDetector produces one anomaly per Detect with a unique source name
// based on currentIndex.
type dynamicAnomalyDetector struct {
	prefix       string
	currentIndex int
}

func (d *dynamicAnomalyDetector) Name() string { return "dynamic_anomaly_detector" }
func (d *dynamicAnomalyDetector) Detect(_ observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	return observerdef.DetectionResult{
		Anomalies: []observerdef.Anomaly{
			{
				Source:         observerdef.MetricName(fmt.Sprintf("%s%d", d.prefix, d.currentIndex)),
				SourceSeriesID: observerdef.SeriesID(fmt.Sprintf("ns|%s%d|", d.prefix, d.currentIndex)),
				DetectorName:   d.Name(),
				Title:          fmt.Sprintf("anomaly_%d", d.currentIndex),
				Timestamp:      dataTime,
			},
		},
	}
}

// dynamicCorrelator produces a unique ActiveCorrelation pattern on each Advance call.
type dynamicCorrelator struct {
	prefix       string
	currentIndex int
}

func (c *dynamicCorrelator) Name() string                         { return "dynamic_correlator" }
func (c *dynamicCorrelator) ProcessAnomaly(_ observerdef.Anomaly) {}
func (c *dynamicCorrelator) Advance(_ int64)                      {}
func (c *dynamicCorrelator) ActiveCorrelations() []observerdef.ActiveCorrelation {
	return []observerdef.ActiveCorrelation{
		{
			Pattern:     fmt.Sprintf("%s%d", c.prefix, c.currentIndex),
			Title:       fmt.Sprintf("Correlation %d", c.currentIndex),
			LastUpdated: int64(c.currentIndex),
		},
	}
}
func (c *dynamicCorrelator) Reset() { c.currentIndex = 0 }

// noopLogExtractor is a LogMetricsExtractor that returns no metrics.
// This simulates a log at a timestamp that produces no virtual metrics.
type noopLogExtractor struct{}

func (e *noopLogExtractor) Name() string { return "noop_extractor" }
func (e *noopLogExtractor) ProcessLog(_ observerdef.LogView) []observerdef.MetricOutput {
	return nil
}
