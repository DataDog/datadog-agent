// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
	"github.com/DataDog/datadog-agent/comp/observer/impl/queue"
)

// --- Metrics Extractor ---
// TODO(celian): Pattern keys
// // This contains what can identify a pattern
// type PatternKeyInfo struct {
// 	ClusterID int64
// }

type LogPatternExtractor struct {
	PatternClusterer *patterns.PatternClusterer
}

var _ observerdef.LogMetricsExtractor = (*LogPatternExtractor)(nil)

func NewLogPatternExtractor() *LogPatternExtractor {
	return &LogPatternExtractor{
		PatternClusterer: patterns.NewPatternClusterer(patterns.IDComputeInfo{
			Offset: 0,
			Stride: 1,
			Index:  0,
		}),
	}
}

func (e *LogPatternExtractor) Name() string {
	return "log_pattern_extractor"
}

func (e *LogPatternExtractor) ProcessLog(log observerdef.LogView) []observerdef.MetricOutput {
	message := string(log.GetContent())
	// Extract pattern
	clusterResult := e.PatternClusterer.Process(message)
	if clusterResult == nil {
		return nil
	}

	// TODO(celian): Create a pattern key

	// Emit metric for the pattern
	return []observerdef.MetricOutput{{
		Name:  fmt.Sprintf("log.%s.%d.count", e.Name(), clusterResult.Cluster.ID),
		Value: 1,
		Tags:  log.GetTags(),
	}}
}

// --- Anomaly Detector ---
// We detect anomalies by storing the rates for each pattern group and comparing new rates to the historical rates.
type LogPatternDetector struct {
	MetricsPrefix string
	// The duration of the window to compute rates
	WindowDurationMs int64
	ZThreshold       float64
	// Rates[cluster key] = rates through time
	Rates map[int64]*queue.Queue[float64]
	// We have up to HistorySize items in the queue
	HistorySize int
	// We skipp TooRecentSize items at the beginning of the queue to detect anomalies
	TooRecentSize int
	RateLimiter   *AnomalyRateLimiter
}

var _ observerdef.Detector = (*LogPatternDetector)(nil)

func NewLogPatternDetector() *LogPatternDetector {
	// TODO(celian): Method to get prefix to avoid boilerplate
	return &LogPatternDetector{
		MetricsPrefix:    "_virtual.log.log_pattern_extractor",
		WindowDurationMs: 60 * 1000,
		ZThreshold:       2.0,
		Rates:            make(map[int64]*queue.Queue[float64]),
		HistorySize:      120,
		TooRecentSize:    5,
		// TODO(celian): Should we amend anomalies at some point to have a more accurate score? The score is triggered at ~ the threshold now
		// Wait at least 1 minute between anomalies for the same pattern
		RateLimiter: NewAnomalyRateLimiter(60 * 1000),
	}
}

func (d *LogPatternDetector) Name() string {
	return "log_pattern_detector"
}

func (d *LogPatternDetector) Detect(storage observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	telemetry := make([]observerdef.ObserverTelemetry, 0)
	anomalies := make([]observerdef.Anomaly, 0)

	windowStart := dataTime - d.WindowDurationMs

	// Get all series produced by the extractor
	seriesKeys := storage.ListSeries(observerdef.SeriesFilter{
		NamePattern: d.MetricsPrefix,
	})
	// fmt.Printf("[cc] Detecting log patterns at %d, %d series\n", dataTime, len(seriesKeys))
	for _, seriesKey := range seriesKeys {
		// Add the rate to the queue
		parts := strings.Split(seriesKey.Name, ".")
		if len(parts) < 2 {
			// TODO(celian): Handle errors properly
			fmt.Printf("[cc] Error parsing key %s: not enough parts\n", seriesKey.Name)
			continue
		}
		keyStr := parts[len(parts)-2]
		// TODO(celian): Use key, not cluster ID
		key, err := strconv.ParseInt(keyStr, 10, 64)
		if err != nil {
			fmt.Printf("[cc] Error parsing key %s: %v\n", keyStr, err)
			continue
		}
		if _, ok := d.Rates[key]; !ok {
			d.Rates[key] = queue.NewQueue[float64]()
		}
		queue := d.Rates[key]

		// 1. Compute rate
		count := storage.PointCountSince(seriesKey, windowStart)
		// TODO(celian): What do we do if we don't have this metric anymore?
		// if count == 0 {
		// 	continue
		// }
		rate := float64(count) / float64(d.WindowDurationMs)
		// fmt.Printf("[cc] Series %s has rate %f\n", seriesKey.Name, rate)
		// TODO(celian): Check telemetry
		telemetry = append(telemetry, observerdef.ObserverTelemetry{
			Metric: &metricObs{
				name:      seriesKey.Name,
				value:     rate,
				tags:      seriesKey.Tags,
				timestamp: dataTime,
			},
		})

		// 2. Detect anomalies
		// TODO(celian): We may skip some of them for optimization
		data := queue.Slice()
		// Skip TooRecentSize items at the beginning
		if len(data) > d.TooRecentSize {
			data = data[d.TooRecentSize:]
			// Compute the average and standard deviation
			average := 0.0
			standardDeviation := 0.0
			for _, v := range data {
				average += v
			}
			average /= float64(len(data))
			for _, v := range data {
				standardDeviation += (v - average) * (v - average)
			}
			standardDeviation = math.Sqrt(standardDeviation / float64(len(data)))
			// Compute the z-score, ignore if standard deviation is 0 (anomalies will be detected later)
			zScore := 0.0
			if standardDeviation > 0 {
				zScore = (rate - average) / standardDeviation
			}
			// TODO(celian): We can add an absolute threshold for the exact minimum log rate change
			if math.Abs(zScore) >= d.ZThreshold && d.RateLimiter.CanCreateAnomaly(key, dataTime) {
				fmt.Printf("[cc] Anomaly detected for pattern key %s with z-score %f\n", keyStr, zScore)
				// TODO(celian): Should we include the recent rates to have a smoother score?
				// Convert the z-score to a score between 0 and 1: 1 - exp(thres - abs(z))
				anomalyScore := 1 - math.Exp(d.ZThreshold-math.Abs(zScore))
				anomalies = append(anomalies, observerdef.Anomaly{
					Source:       observerdef.MetricName(seriesKey.Name),
					DetectorName: d.Name(),
					Title:        fmt.Sprintf("Anomaly detected for pattern key %s", keyStr),
					Timestamp:    dataTime,
					Score:        &anomalyScore,
					Tags:         seriesKey.Tags,
				})
			}
		}

		// 3. Update rates
		queue.Enqueue(rate)
		// Ensure we have the correct size
		if queue.Len() > d.HistorySize {
			queue.Dequeue()
		}
	}

	return observerdef.DetectionResult{
		Telemetry: telemetry,
		Anomalies: anomalies,
	}
}
