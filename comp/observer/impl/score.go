// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"
)

// ScoreInput contains the inputs for Gaussian F1 scoring.
type ScoreInput struct {
	PredictionTimestamps  []int64 // period_start from each anomaly period
	GroundTruthTimestamps []int64 // disruption onset timestamp(s)
	Sigma                 float64 // Gaussian width in seconds
}

// ScoreResult contains the Gaussian F1 scoring output.
type ScoreResult struct {
	F1                   float64 `json:"f1"`
	Precision            float64 `json:"precision"`
	Recall               float64 `json:"recall"`
	TP                   float64 `json:"tp"`
	FP                   float64 `json:"fp"`
	FN                   float64 `json:"fn"`
	NumPredictions       int     `json:"num_predictions"`
	NumGroundTruths      int     `json:"num_ground_truths"`
	NumFilteredWarmup    int     `json:"num_filtered_warmup"`
	NumFilteredCascading int     `json:"num_filtered_cascading"`
	NumBaselineFPs       int     `json:"num_baseline_fps"`
	Sigma                float64 `json:"sigma"`
}

// ComputeGaussianF1 scores predicted anomaly events against ground truth events
// using Gaussian overlap with right-sided half-Gaussians.
//
// For each ground truth event, the prediction with the highest overlap is selected
// as the match. Predictions before the ground truth onset get zero overlap and
// cannot match. All other predictions are false positives.
func ComputeGaussianF1(input ScoreInput) ScoreResult {
	result := ScoreResult{
		NumPredictions:  len(input.PredictionTimestamps),
		NumGroundTruths: len(input.GroundTruthTimestamps),
		Sigma:           input.Sigma,
	}

	if len(input.PredictionTimestamps) == 0 && len(input.GroundTruthTimestamps) == 0 {
		result.F1 = 1.0
		result.Precision = 1.0
		result.Recall = 1.0
		return result
	}

	if len(input.PredictionTimestamps) == 0 {
		result.FN = float64(len(input.GroundTruthTimestamps))
		return result
	}

	if len(input.GroundTruthTimestamps) == 0 {
		result.FP = float64(len(input.PredictionTimestamps))
		return result
	}

	// For each GT, find the prediction with the best overlap.
	// Predictions before GT onset get zero overlap (no credit for early alarms).
	matchedPred := make(map[int]bool)
	var tp, fp, fn float64

	for _, gt := range input.GroundTruthTimestamps {
		bestOverlap := 0.0
		bestIdx := -1

		for i, p := range input.PredictionTimestamps {
			if matchedPred[i] || p < gt {
				continue
			}
			overlap := halfGaussianOverlap(p, gt, input.Sigma)
			if overlap > bestOverlap {
				bestOverlap = overlap
				bestIdx = i
			}
		}

		if bestIdx >= 0 {
			matchedPred[bestIdx] = true
			tp += bestOverlap
			fp += 1.0 - bestOverlap
			fn += 1.0 - bestOverlap
		} else {
			fn += 1.0
		}
	}

	// Unmatched predictions → full FP
	for i := range input.PredictionTimestamps {
		if !matchedPred[i] {
			fp += 1.0
		}
	}

	result.TP = tp
	result.FP = fp
	result.FN = fn

	if tp+fp > 0 {
		result.Precision = tp / (tp + fp)
	}
	if tp+fn > 0 {
		result.Recall = tp / (tp + fn)
	}
	if result.Precision+result.Recall > 0 {
		result.F1 = 2 * result.Precision * result.Recall / (result.Precision + result.Recall)
	}

	return result
}

// halfGaussianOverlap computes the overlap between two half-Gaussians:
// one centered at predTS (the prediction) and one at gtTS (ground truth).
//
// Both are right-sided half-Gaussians: zero mass before their center, double
// the normal density after (total area = 1 each). This reflects that both
// predictions and ground truth are forward-looking — a detection at time T
// means "something started here," not "something happened before here."
//
// The prediction half-Gaussian is still penalized for being before ground truth
// onset because its mass extends to the right (after the prediction) and may
// not overlap with the ground truth's mass (which starts at gtTS).
func halfGaussianOverlap(predTS, gtTS int64, sigma float64) float64 {
	d := float64(predTS - gtTS) // positive = prediction is after ground truth
	return numericalOverlapHalfHalf(d, sigma)
}

// numericalOverlapHalfHalf computes the overlap integral between two right-sided
// half-Gaussians numerically using the trapezoidal rule.
// d = predTS - gtTS (positive means prediction is after ground truth).
// Both half-Gaussians have density 2*φ(t) for t >= their center, 0 otherwise.
func numericalOverlapHalfHalf(d, sigma float64) float64 {
	// Integration range: cover from the leftmost center to 5σ past the rightmost
	lower := math.Min(0, d)
	upper := math.Max(0, d) + 5*sigma

	nSteps := 1000
	dt := (upper - lower) / float64(nSteps)

	var overlap float64
	for i := 0; i <= nSteps; i++ {
		// t is relative to gtTS (so gt center is at 0, pred center is at d)
		t := lower + float64(i)*dt

		// Half-Gaussian prediction (centered at d, zero for t < d)
		var predDensity float64
		if t >= d {
			predDensity = 2.0 * scoreGaussianPDF(t-d, sigma)
		}

		// Half-Gaussian ground truth (centered at 0, zero for t < 0)
		var gtDensity float64
		if t >= 0 {
			gtDensity = 2.0 * scoreGaussianPDF(t, sigma)
		}

		minDensity := math.Min(predDensity, gtDensity)

		// Trapezoidal rule
		if i == 0 || i == nSteps {
			overlap += minDensity * dt / 2
		} else {
			overlap += minDensity * dt
		}
	}

	// Clamp to [0, 1] to handle numerical imprecision
	if overlap < 0 {
		overlap = 0
	}
	if overlap > 1 {
		overlap = 1
	}

	return overlap
}

// scoreGaussianPDF returns the value of a zero-mean Gaussian PDF with given sigma at point x.
func scoreGaussianPDF(x, sigma float64) float64 {
	return math.Exp(-x*x/(2*sigma*sigma)) / (sigma * math.Sqrt(2*math.Pi))
}

// scenarioMetadata is the subset of metadata.json fields the scorer needs.
type scenarioMetadata struct {
	Baseline struct {
		Start string `json:"start"`
	} `json:"baseline"`
	Disruption struct {
		Start string `json:"start"`
	} `json:"disruption"`
}

// scoringMetadata holds timestamps extracted from a scenario's metadata.json.
type scoringMetadata struct {
	groundTruthTimestamps []int64
	baselineStart         int64 // 0 if not available
}

// loadScoringMetadata reads disruption.start and baseline.start from a scenario's metadata.json.
func loadScoringMetadata(scenariosDir, scenarioName string) (*scoringMetadata, error) {
	path := filepath.Join(scenariosDir, scenarioName, "metadata.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading metadata %s: %w", path, err)
	}

	var meta scenarioMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing metadata JSON: %w", err)
	}

	if meta.Disruption.Start == "" {
		return nil, errors.New("metadata.json missing disruption.start")
	}

	dt, err := time.Parse(time.RFC3339, meta.Disruption.Start)
	if err != nil {
		return nil, fmt.Errorf("parsing disruption.start %q: %w", meta.Disruption.Start, err)
	}

	result := &scoringMetadata{
		groundTruthTimestamps: []int64{dt.Unix()},
	}

	if meta.Baseline.Start != "" {
		bt, err := time.Parse(time.RFC3339, meta.Baseline.Start)
		if err != nil {
			return nil, fmt.Errorf("parsing baseline.start %q: %w", meta.Baseline.Start, err)
		}
		result.baselineStart = bt.Unix()
	}

	return result, nil
}

// ScoreOutputFile loads a headless output JSON file, extracts prediction timestamps,
// and scores them against the given ground truth.
// If groundTruthTimestamps is nil and scenariosDir is non-empty, ground truth is
// inferred from the scenario's metadata.json (using the scenario name from the output).
// Explicit groundTruthTimestamps override metadata inference.
func ScoreOutputFile(outputPath string, groundTruthTimestamps []int64, scenariosDir string, sigma float64) (*ScoreResult, error) {
	if sigma <= 0 {
		return nil, fmt.Errorf("sigma must be positive, got %f", sigma)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("reading output file: %w", err)
	}

	var output ObserverOutput
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, fmt.Errorf("parsing output JSON: %w", err)
	}

	// Load metadata if needed (for ground truth and/or baseline start).
	var baselineStart int64
	if scenariosDir != "" && output.Metadata.Scenario != "" {
		sm, err := loadScoringMetadata(scenariosDir, output.Metadata.Scenario)
		if err != nil {
			if len(groundTruthTimestamps) == 0 {
				return nil, fmt.Errorf("inferring ground truth: %w", err)
			}
			// Metadata load failed but we have explicit GT — continue without baseline filter.
		} else {
			if len(groundTruthTimestamps) == 0 {
				groundTruthTimestamps = sm.groundTruthTimestamps
			}
			baselineStart = sm.baselineStart
		}
	}

	if len(groundTruthTimestamps) == 0 {
		return nil, errors.New("no ground truth: provide --ground-truth-ts or --scenarios-dir with metadata.json")
	}

	// Compute prediction window: filter warmup (before baseline.start) and
	// cascading effects (beyond max(groundTruth) + 2*sigma).
	maxGT := groundTruthTimestamps[0]
	for _, gt := range groundTruthTimestamps[1:] {
		if gt > maxGT {
			maxGT = gt
		}
	}
	cutoff := float64(maxGT) + 2*sigma

	var predictions []int64
	var numFilteredWarmup, numFilteredCascading int
	for _, period := range output.AnomalyPeriods {
		if baselineStart > 0 && period.PeriodStart < baselineStart {
			numFilteredWarmup++
			continue
		}
		if float64(period.PeriodStart) > cutoff {
			numFilteredCascading++
			continue
		}
		predictions = append(predictions, period.PeriodStart)
	}

	// Count baseline FPs: scored predictions that fire before disruption onset.
	minGT := groundTruthTimestamps[0]
	for _, gt := range groundTruthTimestamps[1:] {
		if gt < minGT {
			minGT = gt
		}
	}
	var numBaselineFPs int
	for _, p := range predictions {
		if p < minGT {
			numBaselineFPs++
		}
	}

	result := ComputeGaussianF1(ScoreInput{
		PredictionTimestamps:  predictions,
		GroundTruthTimestamps: groundTruthTimestamps,
		Sigma:                 sigma,
	})
	result.NumFilteredWarmup = numFilteredWarmup
	result.NumFilteredCascading = numFilteredCascading
	result.NumBaselineFPs = numBaselineFPs

	return &result, nil
}
