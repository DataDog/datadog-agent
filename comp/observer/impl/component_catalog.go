// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import observerdef "github.com/DataDog/datadog-agent/comp/observer/def"

// componentKind distinguishes detectors from correlators in the catalog.
type componentKind int

const (
	componentDetector componentKind = iota
	componentCorrelator
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
func defaultCatalog() *componentCatalog {
	return &componentCatalog{
		entries: []componentEntry{
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
					return NewBOCPDDetector()
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
					})
				},
				defaultEnabled: false,
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
				defaultEnabled: false,
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
				defaultEnabled: false,
			},
			{
				name:        "fanout",
				displayName: "Fanout",
				kind:        componentCorrelator,
				factory: func() any {
					return &FanoutCorrelator{
						windowSeconds: 30,
						active:        make(map[observerdef.MetricName]*observerdef.ActiveCorrelation),
					}
				},
				defaultEnabled: false,
			},
		},
	}
}

// testbenchCatalog returns a catalog customized for the testbench.
// It differs from the default in these ways:
//   - RRCF uses testbench-specific metrics (parquet names instead of DogStatsD names).
//   - cross_signal is disabled (testbench uses time_cluster instead).
//   - time_cluster is enabled by default.
func testbenchCatalog() *componentCatalog {
	cat := defaultCatalog()
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
// Returns the lists of enabled detectors and correlators ready for engine use,
// plus a map of all component instances (enabled or not) for state management.
func (c *componentCatalog) Instantiate(overrides map[string]bool) (
	detectors []observerdef.Detector,
	correlators []observerdef.Correlator,
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
		}
	}
	return detectors, correlators, components
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
