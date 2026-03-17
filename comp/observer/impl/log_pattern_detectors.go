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
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
	"github.com/DataDog/datadog-agent/comp/observer/impl/queue"
)

// PatternKeyInfo contains what can identify a pattern.
type PatternKeyInfo struct {
	// Hash is the ID of this object, used to find it back in the map.
	Hash      int64
	ClusterID int64
}

// NewPatternKeyInfo creates a PatternKeyInfo for the given cluster ID.
func NewPatternKeyInfo(clusterID int64) PatternKeyInfo {
	return PatternKeyInfo{
		Hash:      clusterID + 1,
		ClusterID: clusterID,
	}
}

// LogPatternExtractor is a LogMetricsExtractor that clusters log messages into
// patterns and emits a count metric per pattern.
type LogPatternExtractor struct {
	PatternClusterer *patterns.PatternClusterer
	PatternKeys      map[int64]PatternKeyInfo
}

var _ observerdef.LogMetricsExtractor = (*LogPatternExtractor)(nil)

// NewLogPatternExtractor creates a new LogPatternExtractor.
func NewLogPatternExtractor() *LogPatternExtractor {
	return &LogPatternExtractor{
		PatternClusterer: patterns.NewPatternClusterer(patterns.IDComputeInfo{
			Offset: 0,
			Stride: 1,
			Index:  0,
		}),
		PatternKeys: make(map[int64]PatternKeyInfo),
	}
}

// Name returns the extractor name.
func (e *LogPatternExtractor) Name() string {
	return "log_pattern_extractor"
}

func (e *LogPatternExtractor) Setup(_ observerdef.GetComponentFunc) error { return nil }

// ProcessLog clusters the log message and emits a count metric for its pattern.
func (e *LogPatternExtractor) ProcessLog(log observerdef.LogView) []observerdef.MetricOutput {
	message := string(log.GetContent())
	clusterResult := e.PatternClusterer.Process(message)
	if clusterResult == nil {
		return nil
	}

	patternKey := NewPatternKeyInfo(clusterResult.Cluster.ID)
	e.PatternKeys[patternKey.Hash] = patternKey

	return []observerdef.MetricOutput{{
		Name:  fmt.Sprintf("log.%s.%x.count", e.Name(), patternKey.Hash),
		Value: 1,
		Tags:  log.GetTags(),
	}}
}

// --- Anomaly Detector ---

// LogPatternDetector detects anomalies by storing the rates for each pattern
// group and comparing new rates to historical rates.
type LogPatternDetector struct {
	MetricsPrefix string
	// WindowDurationMs is the duration of the window to compute rates.
	WindowDurationMs int64
	ZThreshold       float64
	// Rates[cluster key] = rates through time
	Rates map[int64]*queue.Queue[float64]
	// HistorySize is the maximum number of items kept in the queue.
	HistorySize int
	// TooRecentSize is the number of items skipped at the start of the queue.
	TooRecentSize int
	RateLimiter   *AnomalyRateLimiter
	extractor     *LogPatternExtractor
}

var _ observerdef.Detector = (*LogPatternDetector)(nil)

// NewLogPatternDetector creates a new LogPatternDetector.
func NewLogPatternDetector() *LogPatternDetector {
	return &LogPatternDetector{
		MetricsPrefix:    "_virtual.log.log_pattern_extractor",
		WindowDurationMs: 60 * 1000,
		ZThreshold:       3.0,
		Rates:            make(map[int64]*queue.Queue[float64]),
		HistorySize:      120,
		TooRecentSize:    5,
		// Wait at least 1 minute between anomalies for the same pattern.
		RateLimiter: NewAnomalyRateLimiter(60 * 1000),
	}
}

// Name returns the detector name.
func (d *LogPatternDetector) Name() string {
	return "log_pattern_detector"
}

// Setup resolves the LogPatternExtractor dependency.
func (d *LogPatternDetector) Setup(getComponent observerdef.GetComponentFunc) error {
	extractor, err := getComponent("log_pattern_extractor")
	if err != nil {
		return err
	}
	var ok bool
	d.extractor, ok = extractor.(*LogPatternExtractor)
	if !ok {
		return fmt.Errorf("log_pattern_extractor is not a *LogPatternExtractor")
	}
	return nil
}

// Detect implements Detector.
func (d *LogPatternDetector) Detect(storage observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	telemetry := make([]observerdef.ObserverTelemetry, 0)
	anomalies := make([]observerdef.Anomaly, 0)

	windowStart := dataTime - d.WindowDurationMs

	seriesKeys := storage.ListSeries(observerdef.SeriesFilter{
		NamePattern: d.MetricsPrefix,
	})
	for _, seriesKey := range seriesKeys {
		parts := strings.Split(seriesKey.Name, ".")
		if len(parts) < 2 {
			fmt.Printf("[cc] Error parsing key %s: not enough parts\n", seriesKey.Name)
			continue
		}
		keyStr := parts[len(parts)-2]
		key, err := strconv.ParseInt(keyStr, 16, 64)
		if err != nil {
			fmt.Printf("[cc] Error parsing key %s: %v\n", keyStr, err)
			continue
		}
		if _, ok := d.Rates[key]; !ok {
			d.Rates[key] = queue.NewQueue[float64]()
		}
		rateQueue := d.Rates[key]

		// 1. Compute rate.
		count := storage.PointCountSince(seriesKey, windowStart)
		rate := float64(count) / float64(d.WindowDurationMs)
		telemetry = append(telemetry, observerdef.ObserverTelemetry{
			Metric: &metricObs{
				name:      seriesKey.Name,
				value:     rate,
				tags:      seriesKey.Tags,
				timestamp: dataTime,
			},
		})

		// 2. Detect anomalies.
		data := rateQueue.Slice()
		if len(data) > d.TooRecentSize {
			data = data[d.TooRecentSize:]
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
			zScore := 0.0
			if standardDeviation > 0 {
				zScore = (rate - average) / standardDeviation
			}
			if math.Abs(zScore) >= d.ZThreshold && d.RateLimiter.CanCreateAnomaly(key, dataTime) {
				// Create a score between 0.5 and 1 based on the z-score (0.5 score is the baseline).
				tolerance := 0.5
				anomalyScore := 1 - math.Exp((d.ZThreshold-math.Abs(zScore))*tolerance-0.6932)

				patternKey, ok := d.extractor.PatternKeys[key]
				if !ok {
					fmt.Printf("[cc] Pattern key %s not found\n", keyStr)
					continue
				}
				cluster, err := d.extractor.PatternClusterer.GetCluster(patternKey.ClusterID)
				if err != nil {
					fmt.Printf("[cc] Error getting cluster for pattern key %s: %v\n", keyStr, err)
					continue
				}
				pattern := cluster.PatternString()
				action := "increase"
				if zScore < 0 {
					action = "decrease"
				}
				description := fmt.Sprintf("Sudden %s in rate of log pattern (z-score: %.1f, score: %.1f). Pattern: `%s`", action, zScore, anomalyScore, pattern)

				fmt.Printf("[cc] Anomaly detected (%s): %s\n", time.Unix(dataTime, 0).Format(time.RFC3339), description)
				anomalies = append(anomalies, observerdef.Anomaly{
					Source:       observerdef.MetricName(seriesKey.Name),
					DetectorName: d.Name(),
					Title:        fmt.Sprintf("Log pattern %s", action),
					Description:  description,
					Timestamp:    dataTime,
					Score:        &anomalyScore,
					Tags:         seriesKey.Tags,
				})
			}
		}

		// 3. Update rates.
		rateQueue.Enqueue(rate)
		if rateQueue.Len() > d.HistorySize {
			rateQueue.Dequeue()
		}
	}

	return observerdef.DetectionResult{
		Telemetry: telemetry,
		Anomalies: anomalies,
	}
}
