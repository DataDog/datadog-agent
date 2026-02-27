// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// errorAnomalyPattern pairs a log pattern with a severity score.
type errorAnomalyPattern struct {
	pattern  string
	score    float64
	severity string
}

// errorAnomalyPatterns are the patterns that trigger direct anomaly emission (all lowercase).
var errorAnomalyPatterns = []errorAnomalyPattern{
	{"panic", 1.0, "CRITICAL"},
	{"fatal", 0.95, "CRITICAL"},
	{"out of memory", 0.9, "CRITICAL"},
	{" oom ", 0.9, "CRITICAL"},
	{"segfault", 0.85, "CRITICAL"},
	{"stack overflow", 0.85, "CRITICAL"},
	{"null pointer dereference", 0.75, "ERROR"},
	{"unhandled exception", 0.75, "ERROR"},
	{"runtime error", 0.7, "ERROR"},
}

const maxDescriptionLen = 300

// ErrorPatternDetector is a log processor that directly emits anomalies when
// it detects critical patterns in logs (panic, fatal, OOM, segfault, etc.).
// Unlike ConnectionErrorExtractor which emits a metric for TS analysis,
// this processor emits AnomalyOutput directly, bypassing the TS analysis pipeline.
type ErrorPatternDetector struct{}

// Name returns the processor name.
func (e *ErrorPatternDetector) Name() string {
	return "error_pattern_detector"
}

// Process checks if a log contains critical error patterns and emits an anomaly if so.
func (e *ErrorPatternDetector) Process(log observer.LogView) observer.LogProcessorResult {
	score := 0.42
	fmt.Println("Log pattern detected", log.GetTags())

	return observer.LogProcessorResult{
		Anomalies: []observer.AnomalyOutput{
			{
				Source:      "logs",
				Title:       "Log pattern detected",
				Description: "Log pattern detected",
				Tags:        log.GetTags(),
				Score:       &score,
			},
		},
	}
	// content := string(log.GetContent())
	// lowerContent := strings.ToLower(content)

	// for _, p := range errorAnomalyPatterns {
	// 	if strings.Contains(lowerContent, p.pattern) {
	// 		score := p.score
	// 		desc := content
	// 		if len(desc) > maxDescriptionLen {
	// 			desc = desc[:maxDescriptionLen] + "..."
	// 		}
	// 		return observer.LogProcessorResult{
	// 			Anomalies: []observer.AnomalyOutput{
	// 				{
	// 					Source:      "logs",
	// 					Title:       fmt.Sprintf("[%s] Log pattern detected: %q", p.severity, p.pattern),
	// 					Description: desc,
	// 					Tags:        log.GetTags(),
	// 					Score:       &score,
	// 				},
	// 			},
	// 		}
	// 	}
	// }

	// return observer.LogProcessorResult{}
}
