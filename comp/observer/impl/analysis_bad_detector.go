// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// BadDetector is a simple example log processor that looks for "this is bad" in logs.
// It serves as a template for implementing real log processors.
type BadDetector struct{}

// Name returns the processor name.
func (b *BadDetector) Name() string {
	return "bad_detector"
}

// Process checks if a log contains "this is bad" and returns metrics/anomalies if so.
func (b *BadDetector) Process(log observer.LogView) observer.LogProcessorResult {
	content := string(log.GetContent())
	if !strings.Contains(content, "this is bad") {
		return observer.LogProcessorResult{}
	}

	return observer.LogProcessorResult{
		Metrics: []observer.MetricOutput{{
			Name:  "observer.bad_logs.count",
			Value: 1,
			Tags:  log.GetTags(),
		}},
		Anomalies: []observer.AnomalyOutput{{
			Source:      "observer.bad_logs.count",
			Title:       "Bad log detected",
			Description: content,
			Tags:        log.GetTags(),
		}},
	}
}
