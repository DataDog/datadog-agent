// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// validateUniqueExtractorNames rejects duplicate runtime extractor names since
// they are used as namespaces in storage and context lookup.
func validateUniqueExtractorNames(extractors []observer.LogMetricsExtractor) {
	seen := make(map[string]struct{}, len(extractors))
	for _, ext := range extractors {
		name := ext.Name()
		if _, ok := seen[name]; ok {
			panic(fmt.Sprintf("duplicate log extractor name: %q", name))
		}
		seen[name] = struct{}{}
	}
}

// truncate shortens s to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
