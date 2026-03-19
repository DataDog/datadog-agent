// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// contextAwareStorage wraps a StorageReader with namespace-keyed ContextProviders,
// adding GetContext resolution to the storage interface. Detectors receive
// this instead of bare storage so they can query context by namespace + name.
type contextAwareStorage struct {
	observer.StorageReader
	providers map[string]observer.ContextProvider // namespace → provider
}

// GetContext looks up the provider for the given namespace, then queries it
// with the bare metric name. Returns false if no provider exists for the
// namespace or if the provider has no context for the name.
func (s *contextAwareStorage) GetContext(namespace, name string) (observer.MetricContext, bool) {
	cp, ok := s.providers[namespace]
	if !ok {
		return observer.MetricContext{}, false
	}
	return cp.GetContext(name)
}

// collectContextProviders discovers ContextProvider implementations among
// instantiated extractors via type assertion. Returns a map keyed by the
// extractor's component name (which is used as the storage namespace for
// its metrics), enabling O(1) lookup during anomaly enrichment.
func collectContextProviders(extractors []observer.LogMetricsExtractor) map[string]observer.ContextProvider {
	providers := make(map[string]observer.ContextProvider)
	for _, ext := range extractors {
		if cp, ok := ext.(observer.ContextProvider); ok {
			providers[ext.Name()] = cp
		}
	}
	return providers
}

// truncate shortens s to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
