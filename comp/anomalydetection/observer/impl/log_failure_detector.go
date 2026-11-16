// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"errors"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// LogFailureDetectorName identifies the experimental history-free error rule.
const LogFailureDetectorName = "log_failure"

// LogFailureConfig exposes qualification thresholds to replay experiments.
// Bucket width and freshness remain fixed at five seconds.
type LogFailureConfig struct {
	MinErrors     int   `json:"min_errors"`
	MinIntervals  int   `json:"min_intervals"`
	WindowSeconds int64 `json:"window_seconds"`
}

// DefaultLogFailureConfig preserves the evaluated repeated-error rule.
func DefaultLogFailureConfig() LogFailureConfig {
	return LogFailureConfig{MinErrors: 3, MinIntervals: 2, WindowSeconds: 30}
}

func (cfg LogFailureConfig) validate() error {
	if cfg.MinErrors < 1 {
		return errors.New("log_failure min_errors must be positive")
	}
	if cfg.WindowSeconds < defaultLogCountBucketSeconds || cfg.WindowSeconds > 300 || cfg.WindowSeconds%defaultLogCountBucketSeconds != 0 {
		return errors.New("log_failure window_seconds must be a multiple of five between 5 and 300")
	}
	if cfg.MinIntervals < 1 || int64(cfg.MinIntervals) > cfg.WindowSeconds/defaultLogCountBucketSeconds {
		return errors.New("log_failure min_intervals must fit within the five-second intervals in the window")
	}
	return nil
}

// LogFailureDetector checks completed error-count intervals. It shares extraction,
// scope and classification with the statistical candidate, without model warmup.
// Runtime state is bounded by storage admission and removed on series eviction.
type LogFailureDetector struct {
	config       LogFailureConfig
	lastPositive map[observer.SeriesRef]int64
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
	ready        bool
	points       []observer.Point
	lastRead     map[observer.SeriesRef]seriesStatus
}

// NewLogFailureDetector returns a detector requiring at least three errors in
// at least two occupied five-second intervals in the last thirty seconds.
func NewLogFailureDetector() *LogFailureDetector {
	return newLogFailureDetector(DefaultLogFailureConfig())
}

// newLogFailureDetector receives configuration validated by the catalog.
func newLogFailureDetector(cfg LogFailureConfig) *LogFailureDetector {
	return &LogFailureDetector{config: cfg, lastPositive: make(map[observer.SeriesRef]int64), lastRead: make(map[observer.SeriesRef]seriesStatus)}
}

func (d *LogFailureDetector) DetectorPointWindow() observer.DetectorPointWindow {
	return observer.DetectorPointWindow{MinPoints: d.config.MinIntervals, MaxPoints: int(d.config.WindowSeconds / defaultLogCountBucketSeconds)}
}
func (*LogFailureDetector) Name() string  { return LogFailureDetectorName }
func (d *LogFailureDetector) Ready() bool { return d.ready }
func (d *LogFailureDetector) Reset()      { *d = *newLogFailureDetector(d.config) }
func (d *LogFailureDetector) RemoveSeries(refs []observer.SeriesRef) {
	for _, ref := range refs {
		delete(d.lastPositive, ref)
		delete(d.lastRead, ref)
	}
	// Do not retain descriptors/tags for evicted scopes until the next Detect.
	d.cachedSeries = nil
}
func (d *LogFailureDetector) Detect(storage observer.StorageReader, dataTime int64, filter observer.DetectionFilter) observer.DetectionResult {
	var out observer.DetectionResult
	for _, namespace := range filter.ExcludeNamespaces {
		if namespace == LogErrorCountExtractorName {
			return out
		}
	}
	aggs, n := detectorAggregations(filter.Aggregations)
	allowed := false
	for _, agg := range aggs[:n] {
		if agg == observer.AggregateAverage {
			allowed = true
		}
	}
	if !allowed {
		return out
	}
	if d.cachedSeries == nil || d.cachedGen != storage.SeriesGeneration() {
		selection := detectorSeriesFilter(filter)
		selection.Namespace = LogErrorCountExtractorName
		selection.NamePattern = logErrorCountMetric
		d.cachedSeries = storage.ListSeries(selection)
		d.cachedGen = storage.SeriesGeneration()
	}
	for _, meta := range d.cachedSeries {
		status := seriesStatus{pointCount: storage.PointCountUpTo(meta.Ref, dataTime), writeGeneration: storage.WriteGeneration(meta.Ref)}
		if previous, ok := d.lastRead[meta.Ref]; ok && status == previous {
			continue
		}
		d.lastRead[meta.Ref] = status
		_, d.points = directWindow(storage, meta.Ref, dataTime-d.config.WindowSeconds, dataTime, observer.AggregateAverage, d.points)
		total, occupied, latest := countFailureWindow(d.points, defaultLogCountBucketSeconds)
		if latest <= d.lastPositive[meta.Ref] {
			continue
		}
		d.lastPositive[meta.Ref] = latest
		// Old positive buckets cannot be renewed by metric activity or zero logs.
		if total < float64(d.config.MinErrors) || occupied < d.config.MinIntervals || dataTime-latest >= defaultLogCountBucketSeconds {
			continue
		}
		d.ready = true
		ref := observer.QueryHandle{Ref: meta.Ref, Aggregate: observer.AggregateAverage}
		out.Anomalies = append(out.Anomalies, observer.Anomaly{
			Type:      observer.AnomalyTypeLog,
			Source:    observer.SeriesDescriptor{Namespace: meta.Namespace, Name: meta.Name, Host: meta.Host, Tags: meta.Tags, Aggregate: observer.AggregateAverage},
			SourceRef: &ref, DetectorName: d.Name(), Timestamp: latest,
			Evidence: observer.EvidenceFailure,
			DebugInfo: &observer.AnomalyDebugInfo{
				CurrentValue: total, Threshold: float64(d.config.MinErrors),
				FailureRule: "repeated_application_errors", FailureWindowSeconds: d.config.WindowSeconds,
				FailureMinIntervals: d.config.MinIntervals,
			},
		})
	}
	return out
}

func hasLogFailureDetector(registrations []detectorRegistration) bool {
	for _, registration := range registrations {
		if registration.detector.Name() == LogFailureDetectorName {
			return true
		}
	}
	return false
}
