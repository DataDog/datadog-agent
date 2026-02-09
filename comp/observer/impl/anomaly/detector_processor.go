// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomaly

import (
	"strings"
	"time"

	logger "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly/internal/collector"
	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly/internal/comparator"
	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly/internal/detector"
	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly/internal/log_grouping"
	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly/internal/types"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	// ProcessedRequests tracks the number requests processed by the handler
	SingleAnomalyScore = telemetry.NewGauge(
		"anomaly",
		"single_score",
		[]string{"type"},
		"",
	)

	// ProcessedRequests tracks the number requests processed by the handler
	AggregatedAnomalyScore = telemetry.NewGauge(
		"anomaly",
		"aggregated_score",
		[]string{"algorithm_type"},
		"Aggregated score of all anomalies detected",
	)
)

// TracePercentiles contains percentile values for trace durations.
type TracePercentiles struct {
	P50 int64
	P95 int64
	P99 int64
}

// PercentilesProvider supplies trace percentile values.
type PercentilesProvider interface {
	P50Value() float64
	P95Value() float64
	P99Value() float64
}

// DetectorProcessor manages detector runner state and score computation.
type DetectorProcessor struct {
	log              logger.Component
	detectorRunner   *detector.DetectorRunner
	telemetryHistory *comparator.TelemetryHistory
	currentStep      int
}

// NewDetectorProcessor creates a DetectorProcessor with the default detector set.
func NewDetectorProcessor(log logger.Component) *DetectorProcessor {
	// Create list of detectors (exclude external anomaly detector)
	detectors := []detector.Detector{
		detector.NewWeightedDetector(),
		detector.NewKofMDetector(),
		detector.NewPValueDetector(),
		detector.NewIsolationForestDetector(),
		detector.NewEVTDetector(),
		detector.NewChangePointDetector(),
	}

	// Create detector runner with minimum step threshold of 10
	detectorRunner := detector.NewDetectorRunner(detectors, 10)

	telemetryComparator := comparator.NewTelemetryComparator()
	comparisonMode := collector.ComparisonMode{
		UseCPUMemV2:  true,
		UseErrorV2:   true,
		UseMetricV2:  true,
		UseNetworkV2: true,
		UseTraceV2:   true,
	}
	history := comparator.NewTelemetryHistory(false, nil, comparisonMode, telemetryComparator)

	return &DetectorProcessor{
		log:              log,
		detectorRunner:   detectorRunner,
		telemetryHistory: history,
	}
}

// ComputeScores builds a telemetry result from top functions, metrics, logs,
// and trace percentiles, then computes anomaly scores for each detector.
func (dp *DetectorProcessor) ComputeScores(
	topFuncs TopFunctions,
	metrics map[string]float64,
	logMessages []string,
	percentiles PercentilesProvider,
) (map[string]detector.DetectorScoreResult, error) {
	dp.currentStep++

	telemetry := dp.buildTelemetry(topFuncs, metrics, logMessages, percentiles)

	dp.log.Infof("Telemetry: %v", telemetry)

	results, err := dp.telemetryHistory.Add(telemetry)
	if err != nil {
		return nil, err
	}

	dp.log.Infof("**** Telemetry comparison result *** \n%v\n", results)
	SingleAnomalyScore.Set(results.CPU, "cpu")
	SingleAnomalyScore.Set(results.Mem, "mem")
	SingleAnomalyScore.Set(results.Err, "error")
	SingleAnomalyScore.Set(results.TraceP50, "trace_p50")
	SingleAnomalyScore.Set(results.TraceP95, "trace_p95")
	SingleAnomalyScore.Set(results.TraceP99, "trace_p99")
	SingleAnomalyScore.Set(results.Metrics, "metrics")

	scores, err := dp.detectorRunner.ComputeScores(results, dp.currentStep)
	if err != nil {
		return nil, err
	}

	dp.log.Infof("**** Detector scores *** \n")
	for name, score := range scores {
		AggregatedAnomalyScore.Set(score.Score, name)
		dp.log.Infof("  %s: %v (best: %v)", name, score.Score, score.BestScore)
	}

	return scores, nil
}

func (dp *DetectorProcessor) buildTelemetry(
	topFuncs TopFunctions,
	metrics map[string]float64,
	logMessages []string,
	percentiles PercentilesProvider,
) collector.Telemetry {
	traceP50, traceP95, traceP99 := percentilesToFloat(percentiles)

	return collector.Telemetry{
		Time:          time.Now(),
		CPU:           buildTelemetrySignal("cpu", cpuValues(topFuncs)),
		Memory:        buildTelemetrySignal("mem", memValues(topFuncs)),
		Error:         buildTelemetrySignal("error", errorValues(logMessages)),
		NetworkClient: collector.NetworkMetrics{},
		NetworkServer: collector.NetworkMetrics{},
		Trace: collector.TraceMetrics{
			P50Duration: traceP50,
			P95Duration: traceP95,
			P99Duration: traceP99,
		},
		Metrics: metricTimeseries(metrics),
	}
}

func buildTelemetrySignal(signalType string, values map[string]float64) collector.TelemetrySignal {
	normalizeValues(values)
	return collector.TelemetrySignal{
		Type:   signalType,
		Values: values,
	}
}

func cpuValues(topFuncs TopFunctions) map[string]float64 {
	values := make(map[string]float64, len(topFuncs.CPU))
	for _, fn := range topFuncs.CPU {
		values[fn.Name] = float64(fn.Flat)
	}
	return values
}

func memValues(topFuncs TopFunctions) map[string]float64 {
	values := make(map[string]float64, len(topFuncs.Memory))
	for _, fn := range topFuncs.Memory {
		values[fn.Name] = float64(fn.Bytes)
	}
	return values
}

func errorValues(logMessages []string) map[string]float64 {
	values := make(map[string]float64)
	logErrors := make([]types.LogError, 0, len(logMessages))
	now := time.Now()
	for _, message := range logMessages {
		logErrors = append(logErrors, types.LogError{
			Message:   message,
			Count:     1,
			Timestamp: now,
		})
	}

	// Group similar logs using DBSCAN clustering
	grouper := log_grouping.NewLogGrouper()
	groupedLogs := grouper.GroupWithDBSCAN(logErrors)
	for _, grouped := range groupedLogs {
		if strings.TrimSpace(grouped.Message) == "" {
			continue
		}
		values[grouped.Message] += float64(grouped.Count)
	}
	return values
}

func normalizeValues(values map[string]float64) {
	if len(values) == 0 {
		return
	}

	sum := 0.0
	for _, value := range values {
		sum += value
	}

	if sum <= 0 {
		return
	}

	for key, value := range values {
		values[key] = value / sum
	}
}

func metricTimeseries(metrics map[string]float64) []collector.MetricTimeseries {

	timeseries := make([]collector.MetricTimeseries, 0, len(metrics))
	for name, value := range metrics {
		timeseries = append(timeseries, collector.MetricTimeseries{
			MetricName: name,
			Average:    value,
		})
	}
	return timeseries
}

func percentilesToFloat(percentiles PercentilesProvider) (float64, float64, float64) {
	if percentiles == nil {
		return 0, 0, 0
	}

	return percentiles.P50Value(), percentiles.P95Value(), percentiles.P99Value()
}
