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
	Category       string // "detector", "correlator", or "processing"
	DefaultEnabled bool
	Factory        func(tb *TestBench) interface{}
}

// registeredComponent is a component instance paired with its registration and enabled state.
type registeredComponent struct {
	Registration ComponentRegistration
	Instance     interface{}
	Enabled      bool
}

// ComponentDataProvider is implemented by components that expose extra data
// beyond what their primary interface provides (e.g. edges, clusters, scores).
type ComponentDataProvider interface {
	GetExtraData() interface{}
}

// defaultRegistry defines all available testbench components.
var defaultRegistry = []ComponentRegistration{
	// Detectors
	{
		Name:           "cusum",
		DisplayName:    "CUSUM",
		Category:       "detector",
		DefaultEnabled: true,
		Factory: func(_ *TestBench) interface{} {
			return NewCUSUMDetector()
		},
	},
	{
		Name:           "bocpd",
		DisplayName:    "BOCPD",
		Category:       "detector",
		DefaultEnabled: true,
		Factory: func(tb *TestBench) interface{} {
			return NewBOCPDDetector()
		},
	},
	{
		Name:           "rrcf",
		DisplayName:    "RRCF",
		Category:       "detector",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			config := DefaultRRCFConfig()
			config.Metrics = TestBenchRRCFMetrics()
			return NewRRCFDetector(config)
		},
	},
	{
		Name:           "pelt",
		DisplayName:    "PELT",
		Category:       "detector",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			return NewPELTDetector()
		},
	},
	{
		Name:           "mannwhitney",
		DisplayName:    "Mann-Whitney",
		Category:       "detector",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			return NewMannWhitneyDetector()
		},
	},
	{
		Name:           "cusum_adaptive",
		DisplayName:    "Adaptive CUSUM",
		Category:       "detector",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			return NewAdaptiveCUSUMDetector()
		},
	},
	{
		Name:           "edivisive",
		DisplayName:    "E-Divisive",
		Category:       "detector",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			return NewEDivisiveDetector()
		},
	},
	{
		Name:           "correlation",
		DisplayName:    "Correlation",
		Category:       "detector",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			return NewCorrelationDetector()
		},
	},
	{
		Name:           "topk",
		DisplayName:    "TopK",
		Category:       "detector",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			return NewTopKDetector()
		},
	},
	{
		Name:           "ensemble",
		DisplayName:    "Ensemble",
		Category:       "detector",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			return NewEnsembleDetector()
		},
	},
	{
		Name:           "cusum_hardened",
		DisplayName:    "Hardened CUSUM",
		Category:       "detector",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			return NewHardenedCUSUMDetector()
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
		Name:           "passthrough",
		DisplayName:    "Passthrough",
		Category:       "correlator",
		DefaultEnabled: false,
		Factory: func(_ *TestBench) interface{} {
			return NewDetectorPassthroughCorrelator()
		},
	},
}

// enabledDetectors returns all enabled MetricsDetector instances (per-series detectors).
func (tb *TestBench) enabledDetectors() []observerdef.MetricsDetector {
	var result []observerdef.MetricsDetector
	for _, comp := range tb.components {
		if comp.Enabled && comp.Registration.Category == "detector" {
			if a, ok := comp.Instance.(observerdef.MetricsDetector); ok {
				result = append(result, a)
			}
		}
	}
	return result
}

// enabledMultiDetectors returns all enabled MultiSeriesDetector instances (pull-based).
func (tb *TestBench) enabledMultiDetectors() []observerdef.MultiSeriesDetector {
	var result []observerdef.MultiSeriesDetector
	for _, comp := range tb.components {
		if comp.Enabled && comp.Registration.Category == "detector" {
			if d, ok := comp.Instance.(observerdef.MultiSeriesDetector); ok {
				result = append(result, d)
			}
		}
	}
	return result
}

// enabledCorrelators returns all enabled Correlator instances.
func (tb *TestBench) enabledCorrelators() []observerdef.Correlator {
	var result []observerdef.Correlator
	for _, comp := range tb.components {
		if comp.Enabled && comp.Registration.Category == "correlator" {
			if p, ok := comp.Instance.(observerdef.Correlator); ok {
				result = append(result, p)
			}
		}
	}
	return result
}

// allCorrelators returns all Correlator instances (enabled or not), for reset.
func (tb *TestBench) allCorrelators() []observerdef.Correlator {
	var result []observerdef.Correlator
	for _, comp := range tb.components {
		if comp.Registration.Category == "correlator" {
			if p, ok := comp.Instance.(observerdef.Correlator); ok {
				result = append(result, p)
			}
		}
	}
	return result
}

// GetComponentData returns the extra data and enabled status for a named component.
func (tb *TestBench) GetComponentData(name string) (data interface{}, enabled bool) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	comp, ok := tb.components[name]
	if !ok {
		return nil, false
	}
	if provider, ok := comp.Instance.(ComponentDataProvider); ok {
		return provider.GetExtraData(), comp.Enabled
	}
	return nil, comp.Enabled
}
