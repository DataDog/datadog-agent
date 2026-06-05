// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	reporterimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/impl"
)

const (
	recordingNumBuckets = 80
	recordingEWMAAlpha  = 0.16
)

var recordingKValues = []float64{5, 10, 20}

// AnomalyRecord is one anomaly event captured during a scenario replay.
type AnomalyRecord struct {
	Timestamp      int64    `json:"timestamp"`
	Detector       string   `json:"detector"`
	Score          *float64 `json:"score"`
	DeviationSigma *float64 `json:"deviation_sigma"`
	Phase          string   `json:"phase"`
	SourceKind     string   `json:"source_kind"`
}

// BucketRecord is one time-bucket of the aggregated anomaly timeline.
type BucketRecord struct {
	BucketTimestamp int64              `json:"bucket_timestamp"`
	AnomalyCount    int                `json:"anomaly_count"`
	RawMeanScore    float64            `json:"raw_mean_score"`
	SaturatedInputs map[string]float64 `json:"saturated_inputs"`
	EWMAValue       float64            `json:"ewma_value"`
	Phase           string             `json:"phase"`
}

// PhaseBoundary holds the start and end unix timestamps of an episode phase.
type PhaseBoundary struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// ScoreRecordingMetadata describes the scenario and pipeline configuration for the recording.
type ScoreRecordingMetadata struct {
	Scenario        string                   `json:"scenario"`
	TimelineStart   int64                    `json:"timeline_start"`
	TimelineEnd     int64                    `json:"timeline_end"`
	DetectorsActive []string                 `json:"detectors_active"`
	PhaseBoundaries map[string]PhaseBoundary `json:"phase_boundaries,omitempty"`
}

// ScoreRecording is the top-level JSON structure produced by WriteScoreRecording.
type ScoreRecording struct {
	Metadata      ScoreRecordingMetadata `json:"metadata"`
	AnomalyEvents []AnomalyRecord        `json:"anomaly_events"`
	TimeBuckets   []BucketRecord         `json:"time_buckets"`
}

// parseEpisodePhaseUnix parses start/end unix timestamps from an EpisodePhase.
// Returns (0, 0, false) if ep is nil or the timestamps cannot be parsed.
func parseEpisodePhaseUnix(ep *EpisodePhase) (start, end int64, ok bool) {
	if ep == nil {
		return 0, 0, false
	}
	s, err1 := time.Parse(time.RFC3339, ep.Start)
	e, err2 := time.Parse(time.RFC3339, ep.End)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return s.Unix(), e.Unix(), true
}

// phaseForTimestamp returns the episode phase name for the given unix timestamp.
// Phases are checked in order: Warmup, Baseline, Disruption, Cooldown.
// Returns "unknown" if no phase covers the timestamp or episodeInfo is nil.
func phaseForTimestamp(ts int64, info *EpisodeInfo) string {
	if info == nil {
		return "unknown"
	}
	type namedPhase struct {
		name string
		ep   *EpisodePhase
	}
	phases := []namedPhase{
		{"warmup", info.Warmup},
		{"baseline", info.Baseline},
		{"disruption", info.Disruption},
		{"cooldown", info.Cooldown},
	}
	for _, p := range phases {
		start, end, ok := parseEpisodePhaseUnix(p.ep)
		if !ok {
			continue
		}
		if ts >= start && ts <= end {
			return p.name
		}
	}
	return "unknown"
}

// sourceKindForAnomaly returns "log-derived" when the anomaly originates from a
// log-metrics or log-pattern extractor, and "standard" otherwise.
func sourceKindForAnomaly(a observerdef.Anomaly) string {
	if reporterimpl.IsLogDerivedAnomaly(a) {
		return "log-derived"
	}
	return "standard"
}

// WriteScoreRecording collects anomalies from the last replay, computes bucketed
// EWMA inputs, and writes a ScoreRecording JSON file to path.
func (tb *Bench) WriteScoreRecording(path string) error {
	tb.mu.RLock()
	sv := tb.debug.StateView()
	allAnomalies := sv.Anomalies()
	timelineStart, timelineEnd, hasBounds := sv.ScenarioBounds()
	episodeInfo := tb.episodeInfo
	scenario := tb.loadedScenario

	var detectorNames []string
	for _, d := range sv.ListDetectors() {
		if d.Enabled {
			detectorNames = append(detectorNames, d.Name)
		}
	}
	tb.mu.RUnlock()

	sort.Strings(detectorNames)

	if !hasBounds {
		timelineStart = 0
		timelineEnd = 0
	}

	// Build phase-boundary map for metadata.
	phaseBoundaries := make(map[string]PhaseBoundary)
	if episodeInfo != nil {
		for _, entry := range []struct {
			name string
			ep   *EpisodePhase
		}{
			{"warmup", episodeInfo.Warmup},
			{"baseline", episodeInfo.Baseline},
			{"disruption", episodeInfo.Disruption},
			{"cooldown", episodeInfo.Cooldown},
		} {
			s, e, ok := parseEpisodePhaseUnix(entry.ep)
			if ok {
				phaseBoundaries[entry.name] = PhaseBoundary{Start: s, End: e}
			}
		}
	}

	// Build per-anomaly records (metric anomalies only — log anomalies are a
	// separate concern and their phase / score distributions differ).
	anomalyRecords := make([]AnomalyRecord, 0, len(allAnomalies))
	for _, a := range allAnomalies {
		if a.Type == observerdef.AnomalyTypeLog {
			continue
		}
		rec := AnomalyRecord{
			Timestamp:  a.Timestamp,
			Detector:   a.DetectorName,
			Score:      a.Score,
			Phase:      phaseForTimestamp(a.Timestamp, episodeInfo),
			SourceKind: sourceKindForAnomaly(a),
		}
		if a.DebugInfo != nil && a.DebugInfo.DeviationSigma != 0 {
			v := a.DebugInfo.DeviationSigma
			rec.DeviationSigma = &v
		}
		anomalyRecords = append(anomalyRecords, rec)
	}

	// Compute time buckets.
	buckets := computeBuckets(anomalyRecords, timelineStart, timelineEnd, episodeInfo)

	recording := ScoreRecording{
		Metadata: ScoreRecordingMetadata{
			Scenario:        scenario,
			TimelineStart:   timelineStart,
			TimelineEnd:     timelineEnd,
			DetectorsActive: detectorNames,
			PhaseBoundaries: phaseBoundaries,
		},
		AnomalyEvents: anomalyRecords,
		TimeBuckets:   buckets,
	}

	data, err := json.MarshalIndent(recording, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling score recording: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing score recording to %s: %w", path, err)
	}
	return nil
}

// computeBuckets divides [timelineStart, timelineEnd] into recordingNumBuckets
// equal slices, aggregates anomalies into each, and computes raw EWMA + saturated
// inputs per bucket.
func computeBuckets(anomalies []AnomalyRecord, timelineStart, timelineEnd int64, episodeInfo *EpisodeInfo) []BucketRecord {
	if timelineStart == 0 && timelineEnd == 0 {
		return nil
	}
	duration := timelineEnd - timelineStart
	if duration <= 0 {
		return nil
	}

	bucketDuration := float64(duration) / float64(recordingNumBuckets)

	type bucketAccum struct {
		scoreSum float64
		count    int
	}
	accums := make([]bucketAccum, recordingNumBuckets)

	for _, a := range anomalies {
		idx := int(float64(a.Timestamp-timelineStart) / bucketDuration)
		if idx < 0 {
			idx = 0
		}
		if idx >= recordingNumBuckets {
			idx = recordingNumBuckets - 1
		}
		score := 0.0
		if a.Score != nil {
			score = *a.Score
		}
		accums[idx].scoreSum += score
		accums[idx].count++
	}

	results := make([]BucketRecord, recordingNumBuckets)
	ewma := 0.0

	for i := 0; i < recordingNumBuckets; i++ {
		bucketMid := timelineStart + int64(float64(i)*bucketDuration+bucketDuration/2)

		count := accums[i].count
		var meanScore float64
		if count > 0 {
			meanScore = accums[i].scoreSum / float64(count)
		}

		// Compute saturated inputs for each k.
		saturated := make(map[string]float64, len(recordingKValues))
		for _, k := range recordingKValues {
			key := fmt.Sprintf("k%.0f", k)
			saturated[key] = meanScore * (1 - math.Exp(-float64(count)/k))
		}

		// EWMA uses the saturated input for the canonical k (first in list).
		canonicalInput := saturated[fmt.Sprintf("k%.0f", recordingKValues[0])]
		ewma = recordingEWMAAlpha*canonicalInput + (1-recordingEWMAAlpha)*ewma

		results[i] = BucketRecord{
			BucketTimestamp: bucketMid,
			AnomalyCount:    count,
			RawMeanScore:    meanScore,
			SaturatedInputs: saturated,
			EWMAValue:       ewma,
			Phase:           phaseForTimestamp(bucketMid, episodeInfo),
		}
	}

	return results
}
