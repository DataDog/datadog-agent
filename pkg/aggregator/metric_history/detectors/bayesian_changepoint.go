// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package detectors

import (
	"fmt"
	"math"

	mh "github.com/DataDog/datadog-agent/pkg/aggregator/metric_history"
)

// bocpdSuffStats holds sufficient statistics for the Normal-Gamma conjugate posterior.
// Used by BOCPD to track each run-length hypothesis efficiently.
type bocpdSuffStats struct {
	n     float64 // number of observations
	mean  float64 // running mean
	sumSq float64 // sum of squared deviations
}

// BayesianChangepointDetector implements Bayesian Online Changepoint Detection (BOCPD).
//
// This algorithm maintains a distribution over "run lengths" (time since last changepoint)
// and uses Bayesian inference to update this distribution as new data arrives.
// When the probability of a very short run length (indicating a recent changepoint) exceeds
// a threshold, an anomaly is reported.
//
// The implementation uses a Gaussian predictive model with conjugate Normal-Gamma priors,
// which allows efficient online updates.
//
// Reference: Adams & MacKay (2007) "Bayesian Online Changepoint Detection"
// https://arxiv.org/abs/0710.3742
type BayesianChangepointDetector struct {
	// Hazard is the constant hazard rate (probability of changepoint at each step).
	// Lower values = expect longer segments between changes.
	// Typical values: 1/100 to 1/250 (expect changepoint every 100-250 observations)
	// Default: 0.01 (expect changepoint every ~100 points)
	Hazard float64

	// Threshold is the minimum probability of changepoint to trigger an anomaly.
	// Higher = fewer, more confident detections. Lower = more sensitive but noisier.
	// Default: 0.5
	Threshold float64

	// MinDataPoints is the minimum number of points required before detection.
	// Default: 15
	MinDataPoints int

	// ReportWindow is the number of recent points to report changepoints for.
	// This prevents re-reporting old changepoints on each detection cycle.
	// Should match DetectionIntervalFlushes from config.
	// Default: 1 (only report changepoints in the most recent point)
	ReportWindow int

	// Prior parameters for Normal-Gamma distribution
	// These control how quickly the detector adapts to new data
	PriorMu    float64 // prior mean (default: 0, will be overridden by first observation)
	PriorKappa float64 // prior strength for mean (default: 0.1, weak prior)
	PriorAlpha float64 // prior shape for variance (default: 1)
	PriorBeta  float64 // prior scale for variance (default: 1)
}

// NewBayesianChangepointDetector creates a new detector with default parameters.
func NewBayesianChangepointDetector() *BayesianChangepointDetector {
	return &BayesianChangepointDetector{
		Hazard:        0.01,  // expect changepoint roughly every 100 observations
		Threshold:     0.5,   // report when >50% probability of recent changepoint
		MinDataPoints: 15,    // need some history before detecting
		ReportWindow:  1,     // only report changepoints from most recent point
		PriorMu:       0,     // will be set from data
		PriorKappa:    0.1,   // weak prior on mean
		PriorAlpha:    1.0,   // weakly informative prior on variance
		PriorBeta:     1.0,   // weakly informative prior on variance
	}
}

// Name returns the detector's unique identifier.
func (d *BayesianChangepointDetector) Name() string {
	return "bayesian_changepoint"
}

// Analyze examines a series and detects changepoints using BOCPD.
func (d *BayesianChangepointDetector) Analyze(key mh.SeriesKey, history *mh.MetricHistory) []mh.Anomaly {
	points := history.Recent.ToSlice()
	if len(points) < d.MinDataPoints {
		return nil
	}

	// Extract values and compute deltas (same as z-score, for consistency)
	values := make([]float64, len(points))
	for i, p := range points {
		values[i] = p.Stats.Mean()
	}

	// Compute deltas
	deltas := make([]float64, len(values)-1)
	for i := 0; i < len(values)-1; i++ {
		deltas[i] = values[i+1] - values[i]
	}

	if len(deltas) < d.MinDataPoints {
		return nil
	}

	// Run BOCPD on deltas - this computes changepoint probability for EVERY point
	changepointProbs := d.runBOCPD(deltas)

	// Only report changepoints from the recent ReportWindow points.
	// We compute probabilities for the full history (needed for accurate Bayesian inference),
	// but only report new changepoints to avoid duplicate alerts.
	// ReportWindow should match DetectionIntervalFlushes (how often we run detection).
	var anomalies []mh.Anomaly

	reportWindow := d.ReportWindow
	if reportWindow <= 0 {
		reportWindow = 1
	}
	startIdx := len(changepointProbs) - reportWindow
	if startIdx < 0 {
		startIdx = 0
	}

	for i := startIdx; i < len(changepointProbs); i++ {
		prob := changepointProbs[i]
		if prob >= d.Threshold {
			// Delta corresponds to transition from points[i] to points[i+1]
			timestamp := points[i+1].Timestamp
			delta := deltas[i]

			// Determine direction
			anomalyType := "changepoint_up"
			direction := "increase"
			if delta < 0 {
				anomalyType = "changepoint_down"
				direction = "decrease"
			}

			// Severity based on probability
			severity := math.Min(prob, 1.0)

			anomalies = append(anomalies, mh.Anomaly{
				SeriesKey:    key,
				DetectorName: d.Name(),
				Timestamp:    timestamp,
				Type:         anomalyType,
				Severity:     severity,
				Message: fmt.Sprintf("Changepoint detected (Bayesian): delta=%.2f, direction=%s, probability=%.2f",
					delta, direction, prob),
			})
		}
	}

	return anomalies
}

// RunBOCPDExposed exposes runBOCPD for testing/diagnostics.
func (d *BayesianChangepointDetector) RunBOCPDExposed(data []float64) []float64 {
	return d.runBOCPD(data)
}

// runBOCPD runs the Bayesian Online Changepoint Detection algorithm.
// Returns the probability of a changepoint at each time step.
func (d *BayesianChangepointDetector) runBOCPD(data []float64) []float64 {
	n := len(data)
	if n == 0 {
		return nil
	}

	// Initialize prior from first few data points
	priorMu := d.PriorMu
	if priorMu == 0 && n > 5 {
		// Use median of first few points as prior mean
		priorMu = computeMedian(data[:min(10, n)])
	}

	// Run length probabilities: R[t][r] = P(run length = r at time t)
	// We use a simplified approach storing only the current distribution
	// and the max run length we track
	maxRunLength := n + 1

	// Current run length distribution (log probabilities for numerical stability)
	logR := make([]float64, maxRunLength)
	logR[0] = 0 // P(r=0) = 1 initially (log(1) = 0)
	for i := 1; i < maxRunLength; i++ {
		logR[i] = math.Inf(-1) // log(0) = -inf
	}

	// Sufficient statistics for each run length hypothesis
	// These allow us to compute predictive probabilities efficiently
	stats := make([]bocpdSuffStats, maxRunLength)
	for i := range stats {
		stats[i] = bocpdSuffStats{n: 0, mean: priorMu, sumSq: 0}
	}

	// Hazard function: constant probability of changepoint
	logH := math.Log(d.Hazard)
	log1mH := math.Log(1 - d.Hazard)

	// Store changepoint probabilities at each step
	changepointProbs := make([]float64, n)

	for t := 0; t < n; t++ {
		x := data[t]

		// Compute predictive probabilities for each run length
		predProbs := make([]float64, t+2)
		for r := 0; r <= t; r++ {
			if logR[r] == math.Inf(-1) {
				predProbs[r] = math.Inf(-1)
				continue
			}

			// Student-t predictive distribution from Normal-Gamma posterior
			predProbs[r] = d.logStudentTPredictive(x, stats[r], priorMu)
		}

		// Growth probabilities: P(r_t = r+1 | r_{t-1} = r)
		newLogR := make([]float64, maxRunLength)
		for i := range newLogR {
			newLogR[i] = math.Inf(-1)
		}

		// Changepoint probability at this step (sum of all probabilities flowing into r=0)
		logCPProb := math.Inf(-1)

		for r := 0; r <= t; r++ {
			if logR[r] == math.Inf(-1) {
				continue
			}

			// Probability of this run length continuing (no changepoint)
			logGrowth := logR[r] + predProbs[r] + log1mH
			newLogR[r+1] = logSumExp(newLogR[r+1], logGrowth)

			// Probability of changepoint (new run starting)
			logCP := logR[r] + predProbs[r] + logH
			logCPProb = logSumExp(logCPProb, logCP)
		}

		// New run starts with probability summed from all possible previous run lengths
		newLogR[0] = logCPProb

		// Normalize
		logSum := math.Inf(-1)
		for r := 0; r <= t+1; r++ {
			logSum = logSumExp(logSum, newLogR[r])
		}
		for r := 0; r <= t+1; r++ {
			newLogR[r] -= logSum
		}

		// Store changepoint probability (probability of run length 0 or 1)
		// P(changepoint) = P(r=0) + P(r=1) represents recent changepoint
		changepointProbs[t] = math.Exp(newLogR[0]) + math.Exp(newLogR[1])

		// Update sufficient statistics for next iteration
		newStats := make([]bocpdSuffStats, maxRunLength)
		newStats[0] = bocpdSuffStats{n: 0, mean: priorMu, sumSq: 0} // fresh start after changepoint

		for r := 0; r <= t; r++ {
			// Update stats for run length r+1
			s := stats[r]
			newN := s.n + 1
			delta := x - s.mean
			newMean := s.mean + delta/newN
			newSumSq := s.sumSq + delta*(x-newMean)

			newStats[r+1] = bocpdSuffStats{n: newN, mean: newMean, sumSq: newSumSq}
		}

		logR = newLogR
		stats = newStats
	}

	return changepointProbs
}

// logStudentTPredictive computes log probability of x under Student-t predictive distribution.
// This is derived from the Normal-Gamma posterior after observing the sufficient statistics.
func (d *BayesianChangepointDetector) logStudentTPredictive(x float64, s bocpdSuffStats, priorMu float64) float64 {
	// Posterior parameters
	kappa := d.PriorKappa + s.n
	alpha := d.PriorAlpha + s.n/2
	mu := (d.PriorKappa*priorMu + s.n*s.mean) / kappa

	// Posterior beta (scale for variance)
	beta := d.PriorBeta + s.sumSq/2
	if s.n > 0 {
		beta += (d.PriorKappa * s.n * (s.mean - priorMu) * (s.mean - priorMu)) / (2 * kappa)
	}

	// Student-t parameters
	nu := 2 * alpha                          // degrees of freedom
	sigma := math.Sqrt(beta * (kappa + 1) / (alpha * kappa)) // scale

	// Handle edge case of zero variance
	if sigma < 1e-10 {
		sigma = 1e-10
	}

	// Log probability of Student-t distribution
	// log p(x | nu, mu, sigma) = log(Gamma((nu+1)/2)) - log(Gamma(nu/2))
	//                           - 0.5*log(nu*pi*sigma^2)
	//                           - ((nu+1)/2)*log(1 + ((x-mu)/sigma)^2/nu)
	z := (x - mu) / sigma
	logP := lgamma((nu+1)/2) - lgamma(nu/2) -
		0.5*math.Log(nu*math.Pi*sigma*sigma) -
		((nu+1)/2)*math.Log(1+z*z/nu)

	return logP
}

// lgamma computes log(Gamma(x)) using Go's math.Lgamma
func lgamma(x float64) float64 {
	lg, _ := math.Lgamma(x)
	return lg
}

// logSumExp computes log(exp(a) + exp(b)) in a numerically stable way
func logSumExp(a, b float64) float64 {
	if math.IsInf(a, -1) {
		return b
	}
	if math.IsInf(b, -1) {
		return a
	}
	if a > b {
		return a + math.Log(1+math.Exp(b-a))
	}
	return b + math.Log(1+math.Exp(a-b))
}
