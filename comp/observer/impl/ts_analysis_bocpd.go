// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// BOCPDDetector detects changepoints using Bayesian Online Changepoint Detection.
// This implementation uses a constant hazard function and a Gaussian predictive
// model with known observation variance and Normal prior over the mean.
type BOCPDDetector struct {
	// MinPoints is the minimum number of points required before emitting.
	// Default: 10
	MinPoints int

	// BaselineFraction is the fraction of points used for baseline variance estimation.
	// Default: 0.25
	BaselineFraction float64

	// Hazard is the constant changepoint hazard probability.
	// Default: 0.05
	Hazard float64

	// CPThreshold is the posterior P(changepoint at t) threshold to emit.
	// Default: 0.6
	CPThreshold float64

	// MaxRunLength caps tracked run-length hypotheses for bounded compute.
	// Default: 200
	MaxRunLength int

	// PriorVarianceScale controls prior variance over the mean relative to observed variance.
	// Default: 10.0
	PriorVarianceScale float64

	// SkipCountMetrics skips :count metrics, which are often noisy under scaling.
	// Default: true
	SkipCountMetrics bool
}

// NewBOCPDDetector creates a BOCPD detector with defaults tuned for testbench use.
func NewBOCPDDetector() *BOCPDDetector {
	return &BOCPDDetector{
		MinPoints:          10,
		BaselineFraction:   0.25,
		Hazard:             0.05,
		CPThreshold:        0.6,
		MaxRunLength:       200,
		PriorVarianceScale: 10.0,
		SkipCountMetrics:   true,
	}
}

// Name returns the analyzer name.
func (b *BOCPDDetector) Name() string {
	return "bocpd_detector"
}

// Analyze runs BOCPD and emits the first changepoint crossing CPThreshold.
func (b *BOCPDDetector) Analyze(series observer.Series) observer.TimeSeriesAnalysisResult {
	if b.SkipCountMetrics && strings.HasSuffix(series.Name, ":count") {
		return observer.TimeSeriesAnalysisResult{}
	}

	minPoints := b.MinPoints
	if minPoints <= 0 {
		minPoints = 10
	}
	baselineFrac := b.BaselineFraction
	if baselineFrac <= 0 {
		baselineFrac = 0.25
	}
	hazard := b.Hazard
	if hazard <= 0 || hazard >= 1 {
		hazard = 0.05
	}
	threshold := b.CPThreshold
	if threshold <= 0 || threshold >= 1 {
		threshold = 0.6
	}
	maxRunLength := b.MaxRunLength
	if maxRunLength <= 0 {
		maxRunLength = 200
	}
	priorVarScale := b.PriorVarianceScale
	if priorVarScale <= 0 {
		priorVarScale = 10.0
	}

	n := len(series.Points)
	if n < minPoints {
		return observer.TimeSeriesAnalysisResult{}
	}

	baselineEnd := int(float64(n) * baselineFrac)
	if baselineEnd < 3 {
		baselineEnd = 3
	}
	if baselineEnd >= n {
		baselineEnd = n - 1
	}

	baselinePoints := series.Points[:baselineEnd]
	baselineMean := mean(baselinePoints)
	baselineStddev := sampleStddev(baselinePoints, baselineMean)
	const epsilon = 1e-10
	if baselineStddev < epsilon {
		if math.Abs(baselineMean) > epsilon {
			baselineStddev = math.Abs(baselineMean) * 0.1
		} else {
			return observer.TimeSeriesAnalysisResult{}
		}
	}

	obsVar := baselineStddev * baselineStddev
	priorMean := baselineMean
	priorPrecision := 1.0 / (obsVar * priorVarScale)

	runProbs := []float64{1.0}
	means := []float64{priorMean}
	precisions := []float64{priorPrecision}

	for i, p := range series.Points {
		x := p.Value
		predPrior := gaussianPDF(x, priorMean, obsVar+1.0/priorPrecision)

		newRunProbs := make([]float64, len(runProbs)+1)
		newRunProbs[0] = hazard * predPrior
		for r := range runProbs {
			pred := gaussianPDF(x, means[r], obsVar+1.0/precisions[r])
			newRunProbs[r+1] = runProbs[r] * (1.0 - hazard) * pred
		}

		normalizeProbs(newRunProbs)
		cpProb := newRunProbs[0]

		if i >= minPoints-1 && cpProb >= threshold {
			deviation := (x - baselineMean) / baselineStddev
			anomaly := observer.AnomalyOutput{
				Source: series.Name,
				Title:  fmt.Sprintf("BOCPD changepoint detected: %s", series.Name),
				Description: fmt.Sprintf("%s changepoint probability %.2f exceeded threshold %.2f",
					series.Name, cpProb, threshold),
				Tags:      series.Tags,
				Timestamp: p.Timestamp,
				DebugInfo: &observer.AnomalyDebugInfo{
					BaselineStart:  series.Points[0].Timestamp,
					BaselineEnd:    series.Points[baselineEnd-1].Timestamp,
					BaselineMean:   baselineMean,
					BaselineStddev: baselineStddev,
					Threshold:      threshold,
					CurrentValue:   x,
					DeviationSigma: deviation,
				},
			}
			return observer.TimeSeriesAnalysisResult{Anomalies: []observer.AnomalyOutput{anomaly}}
		}

		newMeans := make([]float64, len(newRunProbs))
		newPrecisions := make([]float64, len(newRunProbs))

		// Run length 0 hypothesis: changepoint at current time, restart from prior.
		newMeans[0], newPrecisions[0] = normalPosterior(priorMean, priorPrecision, x, obsVar)
		// Growth hypotheses.
		for r := range means {
			newMeans[r+1], newPrecisions[r+1] = normalPosterior(means[r], precisions[r], x, obsVar)
		}

		if len(newRunProbs) > maxRunLength+1 {
			newRunProbs = newRunProbs[:maxRunLength+1]
			newMeans = newMeans[:maxRunLength+1]
			newPrecisions = newPrecisions[:maxRunLength+1]
			normalizeProbs(newRunProbs)
		}

		runProbs = newRunProbs
		means = newMeans
		precisions = newPrecisions
	}

	return observer.TimeSeriesAnalysisResult{}
}

func normalPosterior(priorMean, priorPrecision, x, obsVar float64) (mean, precision float64) {
	obsPrecision := 1.0 / obsVar
	precision = priorPrecision + obsPrecision
	mean = (priorPrecision*priorMean + obsPrecision*x) / precision
	return mean, precision
}

func normalizeProbs(probs []float64) {
	var total float64
	for _, p := range probs {
		total += p
	}
	if total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
		uniform := 1.0 / float64(len(probs))
		for i := range probs {
			probs[i] = uniform
		}
		return
	}
	for i := range probs {
		probs[i] /= total
	}
}

func gaussianPDF(x, mean, variance float64) float64 {
	const minVariance = 1e-12
	if variance < minVariance {
		variance = minVariance
	}
	z := x - mean
	denom := math.Sqrt(2 * math.Pi * variance)
	return math.Exp(-(z*z)/(2*variance)) / denom
}
