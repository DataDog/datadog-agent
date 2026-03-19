// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// contextAwareStorage wraps a StorageReader with a set of ContextProviders,
// adding GetContext resolution to the storage interface. Detectors receive
// this instead of bare storage so they can enrich anomalies with origin info.
type contextAwareStorage struct {
	observer.StorageReader
	providers []observer.ContextProvider
}

// GetContext queries registered ContextProviders in order, returning the first match.
func (s *contextAwareStorage) GetContext(metricName string) (observer.MetricContext, bool) {
	for _, cp := range s.providers {
		if info, ok := cp.GetContext(metricName); ok {
			return info, true
		}
	}
	return observer.MetricContext{}, false
}

// collectContextProviders discovers ContextProvider implementations among
// instantiated extractors via type assertion. This is called during catalog
// instantiation so the engine receives providers without any schema changes.
func collectContextProviders(extractors []observer.LogMetricsExtractor) []observer.ContextProvider {
	var providers []observer.ContextProvider
	for _, ext := range extractors {
		if cp, ok := ext.(observer.ContextProvider); ok {
			providers = append(providers, cp)
		}
	}
	return providers
}
