// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package detector provides anomaly detection algorithms and scoring mechanisms.
package detector

// DetectorRunner manages multiple detectors and tracks their best scores
type DetectorRunner struct {
	detectors      []Detector
	bestScores     map[string]float64
	minStepForBest int // minimum step before tracking best scores
}

// DetectorScoreResult contains the computed score and best score information
type DetectorScoreResult struct {
	Score     float64
	BestScore float64
	IsNewBest bool
}

// NewDetectorRunner creates a new detector runner with the given detectors and minimum step threshold
func NewDetectorRunner(detectors []Detector, minStepForBest int) *DetectorRunner {
	bestScores := make(map[string]float64)

	// Initialize best scores based on whether higher or lower scores indicate anomalies
	for _, detector := range detectors {
		if detector.HigherIsAnomalous() {
			// For detectors where higher is anomalous, start with lowest possible
			bestScores[detector.Name()] = float64(-1)
		} else {
			// For detectors where lower is anomalous, start with highest possible
			bestScores[detector.Name()] = float64(1000000)
		}
	}

	return &DetectorRunner{
		detectors:      detectors,
		bestScores:     bestScores,
		minStepForBest: minStepForBest,
	}
}

// ComputeScores computes scores for all detectors and returns results with best score information
func (dr *DetectorRunner) ComputeScores(result TelemetryResult, currentStep int) (map[string]DetectorScoreResult, error) {
	scoreResults := make(map[string]DetectorScoreResult)

	for _, detector := range dr.detectors {
		score, err := detector.ComputeScore(result)
		if err != nil {
			return nil, err
		}

		result := DetectorScoreResult{
			Score:     score,
			BestScore: dr.bestScores[detector.Name()],
			IsNewBest: false,
		}

		// Track best score if we've passed the minimum step threshold
		if currentStep > dr.minStepForBest {
			isNewBest := false

			if detector.HigherIsAnomalous() {
				// Higher scores are more anomalous
				if score > dr.bestScores[detector.Name()] {
					isNewBest = true
					dr.bestScores[detector.Name()] = score
				}
			} else {
				// Lower scores are more anomalous
				if score < dr.bestScores[detector.Name()] {
					isNewBest = true
					dr.bestScores[detector.Name()] = score
				}
			}

			if isNewBest {
				result.IsNewBest = true
				result.BestScore = score
			}
		}

		scoreResults[detector.Name()] = result
	}

	return scoreResults, nil
}

// GetDetectors returns the list of detectors
func (dr *DetectorRunner) GetDetectors() []Detector {
	return dr.detectors
}
