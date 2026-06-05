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
	"path/filepath"
	"sort"
	"time"
)

// PhaseScoreSummary holds distribution statistics for a set of float64 values.
type PhaseScoreSummary struct {
	Count  int     `json:"count"`
	Mean   float64 `json:"mean"`
	Stddev float64 `json:"stddev"`
	P50    float64 `json:"p50"`
	P95    float64 `json:"p95"`
	P99    float64 `json:"p99"`
}

// DetectorSummary aggregates score distribution for a single detector.
type DetectorSummary struct {
	Global  PhaseScoreSummary            `json:"global"`
	ByPhase map[string]PhaseScoreSummary `json:"by_phase"`
}

// PhaseEWMASummary holds EWMA percentile statistics.
type PhaseEWMASummary struct {
	Count int     `json:"count"`
	P5    float64 `json:"p5"`
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
}

// EWMASummary holds global and per-phase EWMA distribution.
type EWMASummary struct {
	Global  PhaseEWMASummary            `json:"global"`
	ByPhase map[string]PhaseEWMASummary `json:"by_phase"`
}

// SaturatedInputSummary holds global and per-phase distribution for one k value.
type SaturatedInputSummary struct {
	Global  PhaseEWMASummary            `json:"global"`
	ByPhase map[string]PhaseEWMASummary `json:"by_phase"`
}

// CalibrationSummary is the aggregated output of SummarizeDir.
type CalibrationSummary struct {
	GeneratedAt       string                           `json:"generated_at"`
	ScenariosIncluded []string                         `json:"scenarios_included"`
	Detectors         map[string]DetectorSummary       `json:"detectors"`
	EWMA              EWMASummary                      `json:"ewma"`
	SaturatedInputs   map[string]SaturatedInputSummary `json:"saturated_inputs"`
}

// SummarizeDir reads all ScoreRecording JSON files in dir, aggregates them, and
// writes a CalibrationSummary JSON to outputPath.
func SummarizeDir(dir, outputPath string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading directory %s: %w", dir, err)
	}

	// Accumulators keyed by (detector, phase) for scores, and by phase for EWMA / saturated inputs.
	type key struct{ detector, phase string }
	detectorScores := make(map[key][]float64)

	type ewmaKey struct{ phase string }
	ewmaValues := make(map[ewmaKey][]float64)

	// saturated inputs: keyed by (kLabel, phase)
	type satKey struct{ kLabel, phase string }
	satValues := make(map[satKey][]float64)

	var scenarios []string

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		filePath := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", filePath, err)
		}
		var rec ScoreRecording
		if err := json.Unmarshal(data, &rec); err != nil {
			// Skip files that are not ScoreRecordings (e.g. existing calibration-summary.json).
			continue
		}
		// AnomalyEvents is always initialized as a non-nil slice in WriteScoreRecording.
		// Files that parse but have nil AnomalyEvents (e.g. ObserverOutput JSONs that share
		// the metadata.scenario field) are not valid ScoreRecordings and must be skipped.
		if rec.Metadata.Scenario == "" || rec.AnomalyEvents == nil {
			continue
		}
		scenarios = append(scenarios, rec.Metadata.Scenario)

		// Accumulate anomaly scores per detector × phase.
		for _, a := range rec.AnomalyEvents {
			if a.Score == nil {
				continue
			}
			k := key{detector: a.Detector, phase: a.Phase}
			detectorScores[k] = append(detectorScores[k], *a.Score)
			kGlobal := key{detector: a.Detector, phase: "global"}
			detectorScores[kGlobal] = append(detectorScores[kGlobal], *a.Score)
		}

		// Accumulate EWMA and saturated inputs per bucket.
		for _, b := range rec.TimeBuckets {
			ek := ewmaKey{phase: b.Phase}
			ewmaValues[ek] = append(ewmaValues[ek], b.EWMAValue)
			ekGlobal := ewmaKey{phase: "global"}
			ewmaValues[ekGlobal] = append(ewmaValues[ekGlobal], b.EWMAValue)

			for kLabel, v := range b.SaturatedInputs {
				sk := satKey{kLabel: kLabel, phase: b.Phase}
				satValues[sk] = append(satValues[sk], v)
				skGlobal := satKey{kLabel: kLabel, phase: "global"}
				satValues[skGlobal] = append(satValues[skGlobal], v)
			}
		}
	}

	if len(scenarios) == 0 {
		return fmt.Errorf("no valid ScoreRecording JSON files found in %s", dir)
	}
	sort.Strings(scenarios)

	// Build detector summaries.
	detectorMap := make(map[string]DetectorSummary)
	detectorNames := make(map[string]bool)
	for k := range detectorScores {
		detectorNames[k.detector] = true
	}
	for det := range detectorNames {
		global := scoreStats(detectorScores[key{detector: det, phase: "global"}])
		byPhase := make(map[string]PhaseScoreSummary)
		for k, vals := range detectorScores {
			if k.detector == det && k.phase != "global" {
				byPhase[k.phase] = scoreStats(vals)
			}
		}
		detectorMap[det] = DetectorSummary{Global: global, ByPhase: byPhase}
	}

	// Build EWMA summary.
	ewmaByPhase := make(map[string]PhaseEWMASummary)
	for k, vals := range ewmaValues {
		if k.phase != "global" {
			ewmaByPhase[k.phase] = ewmaStats(vals)
		}
	}
	ewmaSummary := EWMASummary{
		Global:  ewmaStats(ewmaValues[ewmaKey{phase: "global"}]),
		ByPhase: ewmaByPhase,
	}

	// Build saturated input summaries.
	kLabels := make(map[string]bool)
	for k := range satValues {
		kLabels[k.kLabel] = true
	}
	satSummaries := make(map[string]SaturatedInputSummary)
	for kLabel := range kLabels {
		byPhase := make(map[string]PhaseEWMASummary)
		for k, vals := range satValues {
			if k.kLabel == kLabel && k.phase != "global" {
				byPhase[k.phase] = ewmaStats(vals)
			}
		}
		satSummaries[kLabel] = SaturatedInputSummary{
			Global:  ewmaStats(satValues[satKey{kLabel: kLabel, phase: "global"}]),
			ByPhase: byPhase,
		}
	}

	summary := CalibrationSummary{
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
		ScenariosIncluded: scenarios,
		Detectors:         detectorMap,
		EWMA:              ewmaSummary,
		SaturatedInputs:   satSummaries,
	}

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling calibration summary: %w", err)
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("writing calibration summary to %s: %w", outputPath, err)
	}
	return nil
}

// scoreStats computes mean, stddev, P50, P95, P99 for a slice of raw score values.
func scoreStats(vals []float64) PhaseScoreSummary {
	n := len(vals)
	if n == 0 {
		return PhaseScoreSummary{}
	}
	sorted := make([]float64, n)
	copy(sorted, vals)
	sort.Float64s(sorted)

	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	mean := sum / float64(n)

	variance := 0.0
	for _, v := range sorted {
		d := v - mean
		variance += d * d
	}
	stddev := 0.0
	if n > 1 {
		stddev = math.Sqrt(variance / float64(n-1))
	}

	return PhaseScoreSummary{
		Count:  n,
		Mean:   mean,
		Stddev: stddev,
		P50:    percentile(sorted, 50),
		P95:    percentile(sorted, 95),
		P99:    percentile(sorted, 99),
	}
}

// ewmaStats computes P5, P50, P95, P99 for a slice of EWMA or saturated-input values.
func ewmaStats(vals []float64) PhaseEWMASummary {
	n := len(vals)
	if n == 0 {
		return PhaseEWMASummary{}
	}
	sorted := make([]float64, n)
	copy(sorted, vals)
	sort.Float64s(sorted)

	return PhaseEWMASummary{
		Count: n,
		P5:    percentile(sorted, 5),
		P50:   percentile(sorted, 50),
		P95:   percentile(sorted, 95),
		P99:   percentile(sorted, 99),
	}
}

// percentile returns the p-th percentile of a pre-sorted slice using linear interpolation.
func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	rank := (p / 100) * float64(n-1)
	lower := int(rank)
	upper := lower + 1
	if upper >= n {
		return sorted[n-1]
	}
	frac := rank - float64(lower)
	return sorted[lower] + frac*(sorted[upper]-sorted[lower])
}
