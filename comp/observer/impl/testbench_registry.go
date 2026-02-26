// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import observerdef "github.com/DataDog/datadog-agent/comp/observer/def"

// ComponentRegistration describes how to create and identify a testbench component.
type ComponentRegistration struct {
	Name           string // unique identifier (e.g. "cusum", "lead_lag")
	DisplayName    string // human-readable name (e.g. "CUSUM", "Lead-Lag")
	Category       string // "analyzer", "correlator", or "processing"
	DefaultEnabled bool
	Factory        func(tb *TestBench) interface{}
}

// registeredComponent is a component instance paired with its registration and enabled state.
type registeredComponent struct {
	Registration ComponentRegistration
	Instance     interface{}
	Enabled      bool
}

// CorrelatorDataProvider is implemented by correlators that expose extra data
// beyond what the AnomalyProcessor interface provides (e.g. edges, clusters).
type CorrelatorDataProvider interface {
	GetExtraData() interface{}
}

// defaultRegistry defines all available testbench components.
var defaultRegistry = []ComponentRegistration{
	// Analyzers
	{
		Name:           "cusum",
		DisplayName:    "CUSUM",
		Category:       "analyzer",
		DefaultEnabled: true,
		Factory: func(tb *TestBench) interface{} {
			cusum := NewCUSUMDetector()
			cusum.SkipCountMetrics = !tb.config.CUSUMIncludeCount
			return cusum
		},
	},
	{
		Name:           "zscore",
		DisplayName:    "Z-Score",
		Category:       "analyzer",
		DefaultEnabled: true,
		Factory: func(tb *TestBench) interface{} {
			return NewRobustZScoreDetector()
		},
	},
	{
		Name:           "bocpd",
		DisplayName:    "BOCPD",
		Category:       "analyzer",
		DefaultEnabled: true,
		Factory: func(tb *TestBench) interface{} {
			return NewBOCPDDetector()
		},
	},
	{
		Name:           "lightesd",
		DisplayName:    "LightESD",
		Category:       "analyzer",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			return NewLightESDEmitter(DefaultLightESDConfig())
		},
	},
	{
		Name:           "graphsketch_analyzer",
		DisplayName:    "GraphSketch-Analyzer",
		Category:       "analyzer",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			return NewGraphSketchEmitter(DefaultGraphSketchConfig())
		},
	},
	// Correlators
	{
		Name:           "time_cluster",
		DisplayName:    "TimeCluster",
		Category:       "correlator",
		DefaultEnabled: true,
		Factory: func(_ *TestBench) interface{} {
			return NewTimeClusterCorrelator(TimeClusterConfig{
				ProximitySeconds: 10,
				MinClusterSize:   2,
				WindowSeconds:    120,
			})
		},
	},
	{
		Name:           "lead_lag",
		DisplayName:    "Lead-Lag",
		Category:       "correlator",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			return NewLeadLagCorrelator(LeadLagConfig{
				MaxLagSeconds:       30,
				MinObservations:     3,
				ConfidenceThreshold: 0.6,
				WindowSeconds:       120,
			})
		},
	},
	{
		Name:           "surprise",
		DisplayName:    "Surprise",
		Category:       "correlator",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			return NewSurpriseCorrelator(SurpriseConfig{
				WindowSizeSeconds: 10,
				MinLift:           2.0,
				MinSupport:        2,
			})
		},
	},
	{
		Name:           "graph_sketch",
		DisplayName:    "GraphSketch",
		Category:       "correlator",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			return NewGraphSketchCorrelator(DefaultGraphSketchCorrelatorConfig())
		},
	},
	// Processing
	{
		Name:           "dedup",
		DisplayName:    "Deduplication",
		Category:       "processing",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			return NewAnomalyDeduplicator(AnomalyDedupConfig{
				BucketSizeSeconds: 5,
			})
		},
	},
}

// enabledAnalyzers returns all enabled TimeSeriesAnalysis instances.
func (tb *TestBench) enabledAnalyzers() []observerdef.TimeSeriesAnalysis {
	var result []observerdef.TimeSeriesAnalysis
	for _, comp := range tb.components {
		if comp.Enabled && comp.Registration.Category == "analyzer" {
			if a, ok := comp.Instance.(observerdef.TimeSeriesAnalysis); ok {
				result = append(result, a)
			}
		}
	}
	return result
}

// enabledCorrelators returns all enabled AnomalyProcessor instances.
func (tb *TestBench) enabledCorrelators() []observerdef.AnomalyProcessor {
	var result []observerdef.AnomalyProcessor
	for _, comp := range tb.components {
		if comp.Enabled && comp.Registration.Category == "correlator" {
			if p, ok := comp.Instance.(observerdef.AnomalyProcessor); ok {
				result = append(result, p)
			}
		}
	}
	return result
}

// allCorrelators returns all AnomalyProcessor instances (enabled or not), for reset.
func (tb *TestBench) allCorrelators() []observerdef.AnomalyProcessor {
	var result []observerdef.AnomalyProcessor
	for _, comp := range tb.components {
		if comp.Registration.Category == "correlator" {
			if p, ok := comp.Instance.(observerdef.AnomalyProcessor); ok {
				result = append(result, p)
			}
		}
	}
	return result
}

// getDeduplicator returns the deduplicator if it is enabled, or nil.
func (tb *TestBench) getDeduplicator() *AnomalyDeduplicator {
	comp, ok := tb.components["dedup"]
	if !ok || !comp.Enabled {
		return nil
	}
	if d, ok := comp.Instance.(*AnomalyDeduplicator); ok {
		return d
	}
	return nil
}

// GetCorrelatorData returns the extra data and enabled status for a named correlator.
func (tb *TestBench) GetCorrelatorData(name string) (data interface{}, enabled bool) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	comp, ok := tb.components[name]
	if !ok {
		return nil, false
	}
	if provider, ok := comp.Instance.(CorrelatorDataProvider); ok {
		return provider.GetExtraData(), comp.Enabled
	}
	return nil, comp.Enabled
}
