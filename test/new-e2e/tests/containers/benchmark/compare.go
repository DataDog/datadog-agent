// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import (
	"errors"
	"fmt"
	"math"
	"time"
)

const (
	// SignificanceLevel is the p-value under which a difference between the versions is
	// significant.
	SignificanceLevel = 0.05
	// MinBlocks is the minimum number of blocks of the window of a phase: the significance
	// test is computed from the block means.
	MinBlocks = 3
)

// Point is a point of a timeseries.
type Point struct {
	Timestamp time.Time
	Value     float64
}

// Window is the measurement window [From, To) of a phase, split into consecutive blocks of
// Block (the last one may be shorter). The significance test relies on the means of the
// blocks rather than on the points: consecutive points of a timeseries are autocorrelated,
// the means of long enough blocks much less.
type Window struct {
	From  time.Time     `json:"from"`
	To    time.Time     `json:"to"`
	Block time.Duration `json:"block_ns"`
}

// Thresholds are the minimum changes of the mean of a metric to report it as an
// improvement or a regression.
type Thresholds struct {
	// Relative is the minimum change relative to the baseline, e.g. 0.05 for 5%.
	Relative float64 `json:"relative"`
	// Absolute is the minimum change in the unit of the metric: a smaller change is never
	// significant.
	Absolute float64 `json:"absolute"`
	// Significance is the p-value under which a change is significant, SignificanceLevel if
	// zero. A benchmark comparing many metrics lowers it to keep the probability of a false
	// verdict among all of them low.
	Significance float64 `json:"significance,omitempty"`
}

// significance returns the p-value under which a change is significant.
func (t Thresholds) significance() float64 {
	if t.Significance > 0 {
		return t.Significance
	}
	return SignificanceLevel
}

// Verdict is the conclusion of the comparison of a metric.
type Verdict string

const (
	// VerdictImprovement is a significant decrease of the metric.
	VerdictImprovement Verdict = "improvement"
	// VerdictRegression is a significant increase of the metric.
	VerdictRegression Verdict = "regression"
	// VerdictUnchanged is no significant change of the metric.
	VerdictUnchanged Verdict = "unchanged"
	// VerdictError is a metric that couldn't be compared.
	VerdictError Verdict = "error"
)

// Phase holds the points of a metric over a measurement phase, by image rather than by
// installation: the two installations, each on node pools of its own, swap their images
// between the two phases of a measurement.
type Phase struct {
	Window Window
	// Baseline are the points of the installation running the baseline images.
	Baseline []Point
	// Comparison are the points of the installation running the comparison images.
	Comparison []Point
}

// PhaseComparison is the comparison of a metric over a phase.
type PhaseComparison struct {
	Window         Window  `json:"window"`
	BaselineMean   float64 `json:"baseline_mean"`
	ComparisonMean float64 `json:"comparison_mean"`
	// LogRatio is the mean over the blocks of ln(comparison mean / baseline mean). It
	// includes the difference between the node pools of the installations.
	LogRatio float64 `json:"log_ratio"`
	Blocks   int     `json:"blocks"`
}

// Comparison is the comparison of a metric between the baseline and the comparison images.
type Comparison struct {
	// BaselineMean and ComparisonMean are the means of the images over both phases: the
	// arithmetic means of their two phase means, one on each node pool.
	BaselineMean   float64 `json:"baseline_mean"`
	ComparisonMean float64 `json:"comparison_mean"`
	// RelativeChange is exp(e) - 1, e being the estimate of ln(comparison / baseline): the
	// mean of the log ratios of both phases, in which the difference between the node pools
	// cancels out.
	RelativeChange float64 `json:"relative_change"`
	// PValue is the two-sided p-value of the t-test of e = 0.
	PValue  float64           `json:"p_value"`
	Verdict Verdict           `json:"verdict"`
	Phases  []PhaseComparison `json:"phases,omitempty"`
}

// Compare compares a metric of the baseline and comparison images over the first phase of a
// measurement and the swapped one, in which the installations swapped their images.
//
// The ratio of the comparison to the baseline in a block of a phase is the effect of the
// images times the ratio β of the speed of the node pools of the installations running them,
// or 1/β once swapped. So the mean of the block log ratios over a phase estimates
// ln(effect) ± ln(β), and the average of both phases ln(effect). Its variance is the sum of
// the variances of both phase means divided by 4, with Welch–Satterthwaite degrees of freedom.
//
// The change is an improvement or a regression when it is significant, and at least as large
// as both thresholds. Every block must hold points of both images: missing data is an error,
// never a lack of change.
func Compare(first, swapped Phase, thresholds Thresholds) (Comparison, error) {
	var c Comparison
	var logRatios [2][]float64
	for i, phase := range []Phase{first, swapped} {
		phaseComparison, ratios, err := comparePhase(phase)
		if err != nil {
			return Comparison{}, fmt.Errorf("phase %d: %w", i+1, err)
		}
		c.Phases = append(c.Phases, phaseComparison)
		logRatios[i] = ratios
	}

	c.BaselineMean = (c.Phases[0].BaselineMean + c.Phases[1].BaselineMean) / 2
	c.ComparisonMean = (c.Phases[0].ComparisonMean + c.Phases[1].ComparisonMean) / 2
	c.RelativeChange = math.Expm1((c.Phases[0].LogRatio + c.Phases[1].LogRatio) / 2)
	c.PValue = meanOfMeansTTest(logRatios[0], logRatios[1])
	c.Verdict = c.verdict(thresholds)
	return c, nil
}

func (c Comparison) verdict(thresholds Thresholds) Verdict {
	if c.PValue >= thresholds.significance() || math.Abs(c.ComparisonMean-c.BaselineMean) < thresholds.Absolute {
		return VerdictUnchanged
	}
	switch {
	case c.RelativeChange >= thresholds.Relative:
		return VerdictRegression
	case c.RelativeChange <= -thresholds.Relative:
		return VerdictImprovement
	}
	return VerdictUnchanged
}

// comparePhase returns the comparison of the images over a phase, and the log ratios of
// their block means.
func comparePhase(phase Phase) (PhaseComparison, []float64, error) {
	baselineMean, baselineBlocks, err := summarize(phase.Baseline, phase.Window)
	if err != nil {
		return PhaseComparison{}, nil, fmt.Errorf("baseline: %w", err)
	}
	comparisonMean, comparisonBlocks, err := summarize(phase.Comparison, phase.Window)
	if err != nil {
		return PhaseComparison{}, nil, fmt.Errorf("comparison: %w", err)
	}

	logRatios := make([]float64, len(baselineBlocks))
	for block := range baselineBlocks {
		// Also rejects NaN, and infinities, which would make the report unmarshalable to JSON.
		if !positiveFinite(baselineBlocks[block]) || !positiveFinite(comparisonBlocks[block]) {
			return PhaseComparison{}, nil, fmt.Errorf("block %d: the means of the baseline (%g) and the comparison (%g) must be positive and finite", block+1, baselineBlocks[block], comparisonBlocks[block])
		}
		logRatios[block] = math.Log(comparisonBlocks[block] / baselineBlocks[block])
	}
	return PhaseComparison{
		Window:         phase.Window,
		BaselineMean:   baselineMean,
		ComparisonMean: comparisonMean,
		LogRatio:       mean(logRatios),
		Blocks:         len(logRatios),
	}, logRatios, nil
}

func positiveFinite(v float64) bool {
	return v > 0 && !math.IsInf(v, 1)
}

// summarize returns the mean of the points within window, and the means of its blocks.
func summarize(points []Point, window Window) (float64, []float64, error) {
	if window.Block <= 0 || !window.From.Before(window.To) {
		return 0, nil, fmt.Errorf("invalid window [%s, %s) of blocks of %s", window.From, window.To, window.Block)
	}
	blocks := int((window.To.Sub(window.From) + window.Block - 1) / window.Block)
	if blocks < MinBlocks {
		return 0, nil, fmt.Errorf("the window holds %d block(s) of %s, at least %d are needed", blocks, window.Block, MinBlocks)
	}

	sums := make([]float64, blocks)
	counts := make([]int, blocks)
	for _, p := range points {
		if p.Timestamp.Before(window.From) || !p.Timestamp.Before(window.To) {
			continue
		}
		block := int(p.Timestamp.Sub(window.From) / window.Block)
		sums[block] += p.Value
		counts[block]++
	}

	var sum float64
	var count int
	means := make([]float64, blocks)
	var errs []error
	for block := range blocks {
		if counts[block] == 0 {
			start := window.From.Add(time.Duration(block) * window.Block)
			end := start.Add(window.Block)
			if end.After(window.To) {
				end = window.To
			}
			errs = append(errs, fmt.Errorf("no data between %s and %s", start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339)))
			continue
		}
		sum += sums[block]
		count += counts[block]
		means[block] = sums[block] / float64(counts[block])
	}
	if len(errs) > 0 {
		return 0, nil, errors.Join(errs...)
	}
	return sum / float64(count), means, nil
}
