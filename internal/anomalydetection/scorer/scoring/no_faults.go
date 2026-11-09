// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package scoring contains evaluation rules shared by the scorer and testbench.
package scoring

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// Window is a labelled interval in the original recording metadata.
type Window struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// HealthyEpisode is the subset of episode.json required for no-fault evaluation.
type HealthyEpisode struct {
	Mode             string          `json:"mode"`
	Expectation      string          `json:"expectation"`
	EvaluationWindow *Window         `json:"evaluation_window,omitempty"`
	Disruption       json.RawMessage `json:"disruption,omitempty"`
	Disruptions      json.RawMessage `json:"disruptions,omitempty"`
}

// Period identifies an emitted episode, rather than each second spent at High.
type Period struct {
	Pattern string  `json:"pattern"`
	Start   float64 `json:"period_start"`
	End     float64 `json:"period_end"`
}

// NoFaultResult deliberately has no F1, precision or recall fields.
type NoFaultResult struct {
	Mode                       string   `json:"mode"`
	ScoringVersion             string   `json:"scoring_version"`
	FalsePositives             int      `json:"false_positives"`
	EvaluationDurationSeconds  float64  `json:"evaluation_duration_seconds"`
	EvaluationWindow           *Window  `json:"evaluation_window"`
	NumFilteredWarmup          int      `json:"num_filtered_warmup"`
	NumFilteredOutsideScenario int      `json:"num_filtered_outside_scenario"`
	NumFilteredNonHigh         int      `json:"num_filtered_non_high"`
	Episodes                   []Period `json:"episodes"`
}

// HealthyWindow refuses to infer a healthy run from missing disruption timing.
func HealthyWindow(episode HealthyEpisode) (float64, float64, error) {
	if episode.Mode != "healthy-control" || episode.Expectation != "healthy" {
		return 0, 0, fmt.Errorf("no-fault scoring requires mode=healthy-control and expectation=healthy")
	}
	for _, raw := range []json.RawMessage{episode.Disruption, episode.Disruptions} {
		var compact bytes.Buffer
		if len(raw) > 0 {
			if err := json.Compact(&compact, raw); err != nil {
				return 0, 0, fmt.Errorf("invalid disruption metadata: %w", err)
			}
		}
		value := compact.String()
		if value != "" && value != "null" && value != "{}" && value != "[]" {
			return 0, 0, fmt.Errorf("healthy episode must not contain disruptions")
		}
	}
	if episode.EvaluationWindow == nil {
		return 0, 0, fmt.Errorf("healthy episode requires evaluation_window.start and end")
	}
	start, err := time.Parse(time.RFC3339Nano, episode.EvaluationWindow.Start)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid evaluation_window.start: %w", err)
	}
	end, err := time.Parse(time.RFC3339Nano, episode.EvaluationWindow.End)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid evaluation_window.end: %w", err)
	}
	if !end.After(start) {
		return 0, 0, fmt.Errorf("evaluation_window.end must be after start")
	}
	return float64(start.Unix()) + float64(start.Nanosecond())/1e9, float64(end.Unix()) + float64(end.Nanosecond())/1e9, nil
}

// ScoreNoFaults counts unique High episode starts in the half-open healthy window.
func ScoreNoFaults(periods []Period, episode HealthyEpisode) (*NoFaultResult, error) {
	start, end, err := HealthyWindow(episode)
	if err != nil {
		return nil, err
	}
	result := &NoFaultResult{
		Mode:                      "no_faults",
		ScoringVersion:            "high-severity-nofault-v1",
		EvaluationDurationSeconds: end - start,
		EvaluationWindow:          episode.EvaluationWindow,
		Episodes:                  []Period{},
	}
	seen := make(map[struct {
		pattern string
		start   float64
	}]bool)
	for _, period := range periods {
		if !strings.HasPrefix(period.Pattern, "anomaly_scorer_high:") {
			result.NumFilteredNonHigh++
			continue
		}
		if math.IsNaN(period.Start) || math.IsInf(period.Start, 0) {
			return nil, fmt.Errorf("episode start must be finite")
		}
		key := struct {
			pattern string
			start   float64
		}{period.Pattern, period.Start}
		if seen[key] {
			continue
		}
		seen[key] = true
		if period.Start < start {
			result.NumFilteredWarmup++
		} else if period.Start >= end {
			result.NumFilteredOutsideScenario++
		} else {
			result.FalsePositives++
			result.Episodes = append(result.Episodes, period)
		}
	}
	return result, nil
}
