// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	config "github.com/DataDog/datadog-agent/comp/core/config"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// ObserverDemoPreset bundles tuned parameters for multiple observer components.
// Activated by the --demo-preset testbench flag or observer.demo_preset: true in config.
// Add new component fields here as the demo configuration grows.
type ObserverDemoPreset struct {
	// BOCPD detector params
	BOCPDWarmupPoints    int
	BOCPDHazard          float64
	BOCPDCPThreshold     float64
	BOCPDCPMassThreshold float64
	// TimeCluster correlator params
	TimeClusterMinClusterSize int
}

// DefaultDemoPreset is the tuned parameter bundle for demo deployments.
// Optimised for the food_delivery_redis benchmark (F1: 0.00 → 0.95).
var DefaultDemoPreset = ObserverDemoPreset{
	BOCPDWarmupPoints:         180,
	BOCPDHazard:               0.001,
	BOCPDCPThreshold:          0.95,
	BOCPDCPMassThreshold:      0.99,
	TimeClusterMinClusterSize: 3,
}

// demoActive returns true when the demo preset should be applied — either via the
// --demo-preset CLI flag or observer.demo_preset: true in the agent config.
func demoActive(cfg config.Component, demoPreset bool) bool {
	return demoPreset || (cfg != nil && cfg.GetBool("observer.demo_preset"))
}

// bocpdFromConfig returns a BOCPDDetector with demo preset and/or individual config overrides.
// Precedence: individual observer.bocpd.* keys > demo preset > code defaults.
// cfg may be nil (testbench without a full agent config).
func bocpdFromConfig(cfg config.Component, demoPreset bool) *BOCPDDetector {
	d := NewBOCPDDetector()

	if demoActive(cfg, demoPreset) {
		d.WarmupPoints = DefaultDemoPreset.BOCPDWarmupPoints
		d.Hazard = DefaultDemoPreset.BOCPDHazard
		d.CPThreshold = DefaultDemoPreset.BOCPDCPThreshold
		d.CPMassThreshold = DefaultDemoPreset.BOCPDCPMassThreshold
	}

	// Individual config overrides always win over the preset.
	if cfg != nil {
		if v := cfg.GetInt("observer.bocpd.warmup_points"); v > 0 {
			d.WarmupPoints = v
		}
		if v := cfg.GetFloat64("observer.bocpd.hazard"); v > 0 {
			d.Hazard = v
		}
		if v := cfg.GetFloat64("observer.bocpd.cp_threshold"); v > 0 {
			d.CPThreshold = v
		}
		if v := cfg.GetFloat64("observer.bocpd.cp_mass_threshold"); v > 0 {
			d.CPMassThreshold = v
		}
		if v := cfg.GetInt("observer.bocpd.short_run_length"); v > 0 {
			d.ShortRunLength = v
		}
		if v := cfg.GetInt("observer.bocpd.recovery_points"); v > 0 {
			d.RecoveryPoints = v
		}
		if v := cfg.GetFloat64("observer.bocpd.prior_variance_scale"); v > 0 {
			d.PriorVarianceScale = v
		}
		if v := cfg.GetFloat64("observer.bocpd.min_variance"); v > 0 {
			d.MinVariance = v
		}
	}

	return d
}

// timeClusterMinSize resolves the MinClusterSize for TimeClusterCorrelator.
// Precedence: observer.time_cluster.min_cluster_size config key > demo preset > 1 (all clusters).
func timeClusterMinSize(cfg config.Component, demoPreset bool) int {
	minSize := 1
	if demoActive(cfg, demoPreset) {
		minSize = DefaultDemoPreset.TimeClusterMinClusterSize
	}
	if cfg != nil {
		if v := cfg.GetInt("observer.time_cluster.min_cluster_size"); v > 0 {
			minSize = v
		}
	}
	return minSize
}

// componentKind distinguishes detectors from correlators in the catalog.
type componentKind int

const (
	componentDetector componentKind = iota
	componentCorrelator
	componentExtractor
)

// componentEntry describes a registered pipeline component (detector or correlator).
type componentEntry struct {
	name           string
	displayName    string
	kind           componentKind
	factory        func() any
	defaultEnabled bool
}

// componentInstance tracks a component entry paired with its runtime instance and enabled state.
type componentInstance struct {
	entry    componentEntry
	instance any
	enabled  bool
}

// componentCatalog is the shared registry of all available pipeline components.
// Both the live observer and the testbench use this to discover and instantiate
// detectors and correlators, eliminating duplicated component assembly logic.
type componentCatalog struct {
	entries []componentEntry
}

// defaultCatalog returns the standard component catalog used by both live and testbench.
// All known detectors and correlators are registered here. Consumers use enable
// overrides to select which subset is active.
func defaultCatalog(cfg config.Component, demoPreset bool) *componentCatalog {
	return &componentCatalog{
		entries: []componentEntry{
			// ---- Extractors ----
			{
				name:        "log_metrics_extractor",
				displayName: "Log Metrics Extractor",
				kind:        componentExtractor,
				factory: func() any {
					return &LogMetricsExtractor{
						MaxEvalBytes: 4096,
						ExcludeFields: map[string]struct{}{
							"timestamp": {}, "ts": {}, "time": {},
							"pid": {}, "ppid": {}, "uid": {}, "gid": {},
						},
					}
				},
				defaultEnabled: true,
			},
			{
				name:        "connection_error_extractor",
				displayName: "Connection Error Extractor",
				kind:        componentExtractor,
				factory: func() any {
					return &ConnectionErrorExtractor{}
				},
				defaultEnabled: true,
			},
			{
				name:        "log_pattern_extractor",
				displayName: "Log Pattern Extractor",
				kind:        componentExtractor,
				factory: func() any {
					return NewLogPatternExtractor()
				},
				defaultEnabled: true,
			},
			// ---- Detectors ----
			{
				name:        "cusum",
				displayName: "CUSUM",
				kind:        componentDetector,
				factory: func() any {
					return NewCUSUMDetector()
				},
				defaultEnabled: false,
			},
			{
				name:        "bocpd",
				displayName: "BOCPD",
				kind:        componentDetector,
				factory: func() any {
					return bocpdFromConfig(cfg, demoPreset)
				},
				defaultEnabled: true,
			},
			{
				name:        "rrcf",
				displayName: "RRCF",
				kind:        componentDetector,
				factory: func() any {
					return NewRRCFDetector(DefaultRRCFConfig())
				},
				defaultEnabled: true,
			},
			// ---- Correlators ----
			{
				name:        "cross_signal",
				displayName: "CrossSignal",
				kind:        componentCorrelator,
				factory: func() any {
					return NewCorrelator(CorrelatorConfig{})
				},
				defaultEnabled: true,
			},
			{
				name:        "time_cluster",
				displayName: "TimeCluster",
				kind:        componentCorrelator,
				factory: func() any {
					return NewTimeClusterCorrelator(TimeClusterConfig{
						ProximitySeconds: 10,
						WindowSeconds:    120,
						MinClusterSize:   timeClusterMinSize(cfg, demoPreset),
					})
				},
				defaultEnabled: true,
			},
			{
				name:        "lead_lag",
				displayName: "Lead-Lag",
				kind:        componentCorrelator,
				factory: func() any {
					return NewLeadLagCorrelator(LeadLagConfig{
						MaxLagSeconds:       30,
						MinObservations:     3,
						ConfidenceThreshold: 0.6,
						WindowSeconds:       120,
					})
				},
				defaultEnabled: true,
			},
			{
				name:        "surprise",
				displayName: "Surprise",
				kind:        componentCorrelator,
				factory: func() any {
					return NewSurpriseCorrelator(SurpriseConfig{
						WindowSizeSeconds: 10,
						MinLift:           2.0,
						MinSupport:        2,
					})
				},
				defaultEnabled: true,
			},
		},
	}
}

// testbenchCatalog returns a catalog customized for the testbench.
// It differs from the default in these ways:
//   - RRCF uses testbench-specific metrics (parquet names instead of DogStatsD names).
//   - cross_signal is disabled (testbench uses time_cluster instead).
//   - time_cluster is enabled by default.
func testbenchCatalog(cfg config.Component, demoPreset bool) *componentCatalog {
	cat := defaultCatalog(cfg, demoPreset)
	cat = cat.WithOverride("rrcf", func() any {
		config := DefaultRRCFConfig()
		config.Metrics = TestBenchRRCFMetrics()
		return NewRRCFDetector(config)
	})
	cat = cat.WithDefaultEnabled("cross_signal", false)
	cat = cat.WithDefaultEnabled("time_cluster", true)
	return cat
}

// WithOverride returns a copy of the catalog with the named component's factory replaced.
func (c *componentCatalog) WithOverride(name string, factory func() any) *componentCatalog {
	newEntries := make([]componentEntry, len(c.entries))
	copy(newEntries, c.entries)
	for i, e := range newEntries {
		if e.name == name {
			newEntries[i].factory = factory
			break
		}
	}
	return &componentCatalog{entries: newEntries}
}

// WithDefaultEnabled returns a copy of the catalog with the named component's defaultEnabled changed.
func (c *componentCatalog) WithDefaultEnabled(name string, enabled bool) *componentCatalog {
	newEntries := make([]componentEntry, len(c.entries))
	copy(newEntries, c.entries)
	for i, e := range newEntries {
		if e.name == name {
			newEntries[i].defaultEnabled = enabled
			break
		}
	}
	return &componentCatalog{entries: newEntries}
}

// Instantiate creates component instances. The overrides map controls which
// components are enabled; keys not present in overrides use the catalog default.
// Returns the lists of enabled detectors, correlators, and extractors ready for
// engine use, plus a map of all component instances (enabled or not) for state
// management.
func (c *componentCatalog) Instantiate(overrides map[string]bool) (
	detectors []observerdef.Detector,
	correlators []observerdef.Correlator,
	extractors []observerdef.LogMetricsExtractor,
	components map[string]*componentInstance,
) {
	components = make(map[string]*componentInstance, len(c.entries))

	for _, entry := range c.entries {
		enabled := entry.defaultEnabled
		if overrides != nil {
			if override, ok := overrides[entry.name]; ok {
				enabled = override
			}
		}

		instance := entry.factory()
		ci := &componentInstance{
			entry:    entry,
			instance: instance,
			enabled:  enabled,
		}
		components[entry.name] = ci

		if !enabled {
			continue
		}

		switch entry.kind {
		case componentDetector:
			if d, ok := instance.(observerdef.Detector); ok {
				detectors = append(detectors, d)
			} else if sd, ok := instance.(observerdef.SeriesDetector); ok {
				detectors = append(detectors, newSeriesDetectorAdapter(sd, defaultAggregations))
			}
		case componentCorrelator:
			if cor, ok := instance.(observerdef.Correlator); ok {
				correlators = append(correlators, cor)
			}
		case componentExtractor:
			if ext, ok := instance.(observerdef.LogMetricsExtractor); ok {
				extractors = append(extractors, ext)
			}
		}
	}
	return detectors, correlators, extractors, components
}

// Entries returns a copy of all catalog entries (for UI/API use).
func (c *componentCatalog) Entries() []componentEntry {
	result := make([]componentEntry, len(c.entries))
	copy(result, c.entries)
	return result
}

// enabledDetectors returns the enabled Detector instances from a components map.
// SeriesDetector implementations are wrapped with seriesDetectorAdapter.
func catalogEnabledDetectors(components map[string]*componentInstance, catalog *componentCatalog) []observerdef.Detector {
	var result []observerdef.Detector
	// Iterate in catalog order for deterministic ordering
	for _, entry := range catalog.entries {
		ci, ok := components[entry.name]
		if !ok || !ci.enabled || ci.entry.kind != componentDetector {
			continue
		}
		if d, ok := ci.instance.(observerdef.Detector); ok {
			result = append(result, d)
		} else if sd, ok := ci.instance.(observerdef.SeriesDetector); ok {
			result = append(result, newSeriesDetectorAdapter(sd, defaultAggregations))
		}
	}
	return result
}

// catalogEnabledExtractors returns the enabled LogMetricsExtractor instances from a components map.
func catalogEnabledExtractors(components map[string]*componentInstance, catalog *componentCatalog) []observerdef.LogMetricsExtractor {
	var result []observerdef.LogMetricsExtractor
	for _, entry := range catalog.entries {
		ci, ok := components[entry.name]
		if !ok || !ci.enabled || ci.entry.kind != componentExtractor {
			continue
		}
		if ext, ok := ci.instance.(observerdef.LogMetricsExtractor); ok {
			result = append(result, ext)
		}
	}
	return result
}

// enabledCorrelators returns the enabled Correlator instances from a components map.
func catalogEnabledCorrelators(components map[string]*componentInstance, catalog *componentCatalog) []observerdef.Correlator {
	var result []observerdef.Correlator
	for _, entry := range catalog.entries {
		ci, ok := components[entry.name]
		if !ok || !ci.enabled || ci.entry.kind != componentCorrelator {
			continue
		}
		if cor, ok := ci.instance.(observerdef.Correlator); ok {
			result = append(result, cor)
		}
	}
	return result
}
