// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// This log based detector will cluster logs into patterns and then detect anomalies based on the pattern rate.

import (
	"fmt"
	"strings"
	"time"

	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
	"github.com/DataDog/datadog-agent/comp/observer/impl/queue"
)

// We keep all the patterns during this period to have a moving average of the pattern rate.
// TODO(celian): 5m?
const logPatternDetectorRatePeriod = 20 * time.Second
const logPatternDetectorOnlyErrorOrWarningLogs = true

// A pre-processed log entry
type LogPatternEntry struct {
	Log     observer.LogView
	Cluster *patterns.Cluster
}

// 1. Process: Logs are processed (clustered) and registered to the ToProcess batch
// 2. Flush: We update the pattern rate
type LogPatternDetector struct {
	PatternClusterer *patterns.PatternClusterer
	// This is the batch we process when we flush.
	ToProcess *queue.Queue[*LogPatternEntry]
	// This is used to compute the rate of the recent patterns.
	// See logPatternDetectorRatePeriod for the amount of time we keep the patterns.
	RecentPatternBatch *queue.Queue[*LogPatternEntry]
	// Used to O(1) compute the rate of the recent patterns.
	// [cluster_id] = count
	RecentPatternRate map[int64]int
}

func NewLogPatternDetector() *LogPatternDetector {
	return &LogPatternDetector{
		PatternClusterer: patterns.NewPatternClusterer(patterns.IDComputeInfo{
			Offset: 0,
			Stride: 1,
			Index:  0,
		}),
		ToProcess:          queue.NewQueue[*LogPatternEntry](),
		RecentPatternBatch: queue.NewQueue[*LogPatternEntry](),
		RecentPatternRate:  make(map[int64]int),
	}
}

func (d *LogPatternDetector) Name() string {
	return "log_pattern_detector"
}

func (d *LogPatternDetector) Process(log observer.LogView) observer.LogDetectionResult {
	if logPatternDetectorOnlyErrorOrWarningLogs {
		status := strings.ToLower(log.GetStatus())
		if status != "error" && status != "warning" && status != "warn" {
			return observer.LogDetectionResult{}
		}
	}

	telemetry := []observer.ObserverTelemetry{}
	cluster := d.PatternClusterer.Process(string(log.GetContent()))

	// Could be nil if empty log
	if cluster != nil {
		d.ToProcess.Enqueue(&LogPatternEntry{
			Log:     log,
			Cluster: cluster.Cluster,
		})

		if cluster.IsNew {
			telemetry = append(telemetry, newTelemetryLog(fmt.Sprintf("New cluster(id: %d): %s", cluster.Cluster.ID, cluster.Cluster.PatternString()), log))
		}
	}

	return observer.LogDetectionResult{Telemetry: telemetry}
}

func (d *LogPatternDetector) Flush(timestampMs int64) observer.LogDetectionResult {
	telemetry := []observer.ObserverTelemetry{}
	telemetry = append(telemetry, observer.ObserverTelemetry{
		Log: &logDataView{
			data: &recorder.LogData{
				Content:     []byte(fmt.Sprintf("Flush(timestampMs: %d)", timestampMs)),
				Tags:        []string{},
				Hostname:    "",
				TimestampMs: timestampMs,
			},
		},
	})
	telemetry = append(telemetry, observer.ObserverTelemetry{
		Metric: &metricObs{
			name:      "cluster_count",
			value:     float64(d.PatternClusterer.NumClusters()),
			tags:      []string{},
			timestamp: timestampMs / 1000,
		},
	})

	// Update recent patterns
	for entry, ok := d.ToProcess.Dequeue(); ok; entry, ok = d.ToProcess.Dequeue() {
		// TODO: We should have a P queue in case timestamps are not in order (or use the batch timestamp)
		d.addRecentPattern(entry)
	}
	d.flushRecentPatterns(timestampMs)

	// Flush batch
	d.ToProcess.Flush()

	// Emit metrics based on pattern rate
	// TODO: 0 metrics could be optimized
	metrics := []observer.MetricOutput{}
	// TODO: Split by GroupID, not cluster id to properly handle tags
	for _, cluster := range d.PatternClusterer.GetClusters() {
		rate := 0.0
		if count, ok := d.RecentPatternRate[cluster.ID]; ok {
			rate = float64(count) / float64(logPatternDetectorRatePeriod/time.Nanosecond) * 1e9
		}

		// TODO: Remove
		if rate == 0.0 {
			continue
		}

		metrics = append(metrics, observer.MetricOutput{
			// TODO: Single metrics, different tags
			Name:  fmt.Sprintf("pattern_rate_%d", cluster.ID),
			Value: rate,
			Tags:  []string{fmt.Sprintf("cluster_id:%d", cluster.ID)},
		})

	}

	// fmt.Printf("[cc] LogPatternDetector: Flush(timestampMs: %d)\n", timestampMs)

	return observer.LogDetectionResult{Telemetry: telemetry, Metrics: metrics}
}

// Will add the pattern to the RecentPatternBatch and update the RecentPatternRate
func (d *LogPatternDetector) addRecentPattern(entry *LogPatternEntry) {
	d.RecentPatternBatch.Enqueue(entry)
	d.RecentPatternRate[entry.Cluster.ID]++
}

func (d *LogPatternDetector) flushRecentPatterns(timestampMs int64) {
	// Remove entries that are too old
	lowerBound := timestampMs - int64(logPatternDetectorRatePeriod/time.Millisecond)
	for entry, ok := d.RecentPatternBatch.Peek(); ok && entry.Log.GetTimestampMs() < lowerBound; entry, ok = d.RecentPatternBatch.Peek() {
		d.RecentPatternBatch.Dequeue()
		d.RecentPatternRate[entry.Cluster.ID]--
	}
}

// New telemetry based on a log we process.
func newTelemetryLog(content string, log observer.LogView) observer.ObserverTelemetry {
	return observer.ObserverTelemetry{
		Log: &logDataView{
			data: &recorder.LogData{
				Content:     []byte(content),
				Tags:        log.GetTags(),
				Hostname:    log.GetHostname(),
				TimestampMs: log.GetTimestampMs(),
			},
		},
	}
}
