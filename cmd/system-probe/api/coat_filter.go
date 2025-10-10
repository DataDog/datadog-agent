// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"strings"
)

// filterCOATMetrics extracts only COAT metrics (system_probe_*) from Prometheus text format
// we do assume all COAT metrics starts with "system_probe_"
// this will avoid sending full metrics duplicates to core agent and emitting them twice
// NOTE(mb) we may want to move this to another pkg
// NOTE(mb) we may want to simplify the logic and keep a simple list of all metrics we want to expose for COAT
func filterCOATMetrics(promText string) string {
	lines := strings.Split(promText, "\n")
	var coatLines []string

	inCOATMetric := false
	for _, line := range lines {
		// Check if this line starts a COAT metric (# HELP or # TYPE system_probe_*)
		if strings.HasPrefix(line, "# HELP system_probe_") || strings.HasPrefix(line, "# TYPE system_probe_") {
			inCOATMetric = true
			coatLines = append(coatLines, line)
		} else if strings.HasPrefix(line, "system_probe_") {
			// Metric value line for COAT metric
			coatLines = append(coatLines, line)
			inCOATMetric = false
		} else if strings.HasPrefix(line, "# ") {
			// Different metric's help/type - no longer in COAT metric
			inCOATMetric = false
		} else if inCOATMetric {
			// Continuation line for COAT metric (multiline help text)
			coatLines = append(coatLines, line)
		}
		// Skip all other lines (non-COAT metrics)
	}

	result := strings.Join(coatLines, "\n")
	if len(result) > 0 {
		result += "\n"
	}
	return result
}
