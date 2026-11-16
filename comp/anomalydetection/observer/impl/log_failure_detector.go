// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"errors"
	"sort"

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
	checkedUpTo  int64
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
	return &LogFailureDetector{config: cfg, lastRead: make(map[observer.SeriesRef]seriesStatus)}
}

func (d *LogFailureDetector) DetectorPointWindow() observer.DetectorPointWindow {
	return observer.DetectorPointWindow{MinPoints: d.config.MinIntervals, MaxPoints: int(d.config.WindowSeconds / defaultLogCountBucketSeconds)}
}
func (*LogFailureDetector) Name() string  { return LogFailureDetectorName }
func (d *LogFailureDetector) Ready() bool { return d.ready }
func (d *LogFailureDetector) Reset()      { *d = *newLogFailureDetector(d.config) }
func (d *LogFailureDetector) RemoveSeries(refs []observer.SeriesRef) {
	for _, ref := range refs {
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
	// Completed buckets are flushed before detectors run, so buckets after the
	// previous check are exactly the newly visible ones. A first check has no
	// such boundary and judges only the latest bucket, so a reset does not
	// re-report retained history.
	newAfter := d.checkedUpTo
	if newAfter == 0 {
		newAfter = dataTime - defaultLogCountBucketSeconds
	}
	d.checkedUpTo = dataTime
	for _, meta := range d.cachedSeries {
		status := seriesStatus{pointCount: storage.PointCountUpTo(meta.Ref, dataTime), writeGeneration: storage.WriteGeneration(meta.Ref)}
		if previous, ok := d.lastRead[meta.Ref]; ok && status == previous {
			continue
		}
		d.lastRead[meta.Ref] = status
		// Analysis advances only when input arrives, so a bucket can become
		// visible long after it completed. Judge each new bucket by the window
		// ending at that bucket: late analysis neither drops nor re-reports it,
		// and zero counts or old buckets cannot renew evidence.
		_, d.points = directWindow(storage, meta.Ref, newAfter-d.config.WindowSeconds, dataTime, observer.AggregateAverage, d.points)
		latest, total, ok := d.newestQualifiedBucket(d.points, newAfter)
		if !ok {
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

// newestQualifiedBucket returns the newest positive bucket after newAfter
// whose window, ending at that bucket, satisfies the rule. Points are ascending.
func (d *LogFailureDetector) newestQualifiedBucket(points []observer.Point, newAfter int64) (int64, float64, bool) {
	for i := len(points) - 1; i >= 0; i-- {
		candidate := points[i]
		if candidate.Timestamp <= newAfter {
			break
		}
		if !(candidate.Value > 0) {
			continue
		}
		windowStart := candidate.Timestamp - d.config.WindowSeconds
		first := sort.Search(i+1, func(j int) bool { return points[j].Timestamp > windowStart })
		total, occupied, _ := countFailureWindow(points[first:i+1], defaultLogCountBucketSeconds)
		if total >= float64(d.config.MinErrors) && occupied >= d.config.MinIntervals {
			return candidate.Timestamp, total, true
		}
	}
	return 0, 0, false
}

func hasLogFailureDetector(registrations []detectorRegistration) bool {
	for _, registration := range registrations {
		if registration.detector.Name() == LogFailureDetectorName {
			return true
		}
	}
	return false
}
