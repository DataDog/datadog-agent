// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// SpikeDetector is a simple time series analysis that detects spikes.
// It flags an anomaly when the latest value is more than 2x the average of prior values.
// This serves as a template for implementing real time series analyses.
type SpikeDetector struct{}

// Name returns the analysis name.
func (s *SpikeDetector) Name() string {
	return "spike_detector"
}

// Analyze checks if the latest value in a series is a spike.
func (s *SpikeDetector) Analyze(series *observer.SeriesStats) observer.TimeSeriesAnalysisResult {
	if len(series.Points) < 2 {
		return observer.TimeSeriesAnalysisResult{}
	}

	// Calculate average of all but last point
	var sum float64
	for i := 0; i < len(series.Points)-1; i++ {
		sum += series.Points[i].Value()
	}
	avg := sum / float64(len(series.Points)-1)

	// Check if latest is a spike (> 2x average)
	latest := series.Points[len(series.Points)-1].Value()
	if avg > 0 && latest > 2*avg {
		return observer.TimeSeriesAnalysisResult{
			Anomalies: []observer.AnomalyOutput{{
				Title:       "Spike detected",
				Description: fmt.Sprintf("%s/%s spiked to %.2f (avg: %.2f)", series.Namespace, series.Name, latest, avg),
				Tags:        series.Tags,
			}},
		}
	}

	return observer.TimeSeriesAnalysisResult{}
}
