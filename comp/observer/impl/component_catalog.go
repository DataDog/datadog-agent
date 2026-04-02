// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// componentKind distinguishes detectors from correlators in the catalog.
type componentKind int

const (
	componentDetector componentKind = iota
	componentCorrelator
	componentExtractor
)

// componentEntry describes a registered pipeline component.
//
// Each entry pairs a default config with a factory function. The factory
// accepts the config and returns the constructed component. This separation
// lets consumers provide config from any source (agent config, testbench UI)
// without replacing the factory itself.
type componentEntry struct {
	name           string
	displayName    string
	kind           componentKind
	defaultConfig  any           // typed config value (e.g. CUSUMConfig, RRCFConfig)
	factory        func(any) any // accepts the config, returns the component
	defaultEnabled bool

	// readConfig optionally reads component config from the agent config
	// system. When set, settingsFromAgentConfig calls it with a ConfigReader
	// and the key prefix "observer.components.<name>.". It returns the
	// populated config struct. Only components that need agent-config
	// tuning set this; others leave it nil.
	readConfig func(ConfigReader, string) any

	// parseJSON optionally parses component hyperparameters from a JSON object
	// (the component's sub-object from a --config params file, with "enabled"
	// already stripped). It starts from the provided defaults and overlays JSON
	// values, so unspecified fields keep their default. Returns the populated
	// typed config. Nil means the component has no tunable hyperparameters.
	parseJSON func(defaults any, raw []byte) (any, error)
}

// componentInstance tracks a component entry paired with its runtime instance and enabled state.
type componentInstance struct {
	entry        componentEntry
	instance     any
	enabled      bool
	activeConfig any // config actually passed to factory (nil for parameterless components)
}

// ConfigReader provides read access to a key-value configuration source.
// This is a minimal interface satisfied by the agent's config.Component,
// allowing component configs to read values without depending on the full
// agent config package.
type ConfigReader interface {
	GetBool(key string) bool
	GetInt(key string) int
	GetFloat64(key string) float64
	GetString(key string) string
	IsKnown(key string) bool
}

// ComponentSettings holds per-component configuration provided by the consumer.
// Both the live observer and the testbench build this from their respective
// config sources, giving a single path through instantiation.
type ComponentSettings struct {
	// Enabled maps component name to whether it should be active.
	// Components not listed use their catalog default.
	Enabled map[string]bool

	// configs is populated internally by readConfig functions on catalog
	// entries (e.g. from agent config). It is not exported because the
	// values must match the typed config expected by each component's
	// factory — a wrong type would panic at instantiation time.
	configs map[string]any
}

// componentCatalog is the shared registry of all available pipeline components.
// Both the live observer and the testbench use this to discover and instantiate
// detectors, correlators, and extractors.
//
// Usage:
//
//	catalog := defaultCatalog()
//	settings := ComponentSettings{ ... } // from agent config, testbench UI, etc.
//	detectors, correlators, extractors, components := catalog.Instantiate(settings)
type componentCatalog struct {
	entries []componentEntry
}

// defaultCatalog returns the component catalog with all known components and
// their default configs. This is the single starting point for both the live
// observer and the testbench — they diverge only in what ComponentSettings
// they pass to Instantiate.
func defaultCatalog() *componentCatalog {
	return &componentCatalog{
		entries: []componentEntry{
			// ---- Extractors ----
			{
				name:           "log_metrics_extractor",
				displayName:    "Log Metrics Extractor",
				kind:           componentExtractor,
				defaultConfig:  DefaultLogMetricsExtractorConfig(),
				factory:        func(cfg any) any { return NewLogMetricsExtractor(cfg.(LogMetricsExtractorConfig)) },
				defaultEnabled: true,
			},
			{
				name:           "connection_error_extractor",
				displayName:    "Connection Error Extractor",
				kind:           componentExtractor,
				defaultConfig:  DefaultConnectionErrorExtractorConfig(),
				factory:        func(any) any { return &ConnectionErrorExtractor{} },
				defaultEnabled: true,
			},
			{
				name:           "log_pattern_extractor",
				displayName:    "Log Pattern Extractor",
				kind:           componentExtractor,
				defaultConfig:  DefaultLogPatternExtractorConfig(),
				factory:        func(cfg any) any { return NewLogPatternExtractor(cfg.(LogPatternExtractorConfig)) },
				defaultEnabled: true,
				parseJSON: func(defaults any, raw []byte) (any, error) {
					cfg := defaults.(LogPatternExtractorConfig)
					if err := json.Unmarshal(raw, &cfg); err != nil {
						return nil, err
					}
					return cfg, nil
				},
			},
			// ---- Detectors ----
			{
				name:           "cusum",
				displayName:    "CUSUM",
				kind:           componentDetector,
				defaultConfig:  DefaultCUSUMConfig(),
				factory:        func(cfg any) any { return NewCUSUMDetector(cfg.(CUSUMConfig)) },
				defaultEnabled: false,
				parseJSON: func(defaults any, raw []byte) (any, error) {
					cfg := defaults.(CUSUMConfig)
					if err := json.Unmarshal(raw, &cfg); err != nil {
						return nil, err
					}
					return cfg, nil
				},
			},
			{
				name:           "bocpd",
				displayName:    "BOCPD",
				kind:           componentDetector,
				defaultConfig:  DefaultBOCPDConfig(),
				factory:        func(cfg any) any { return NewBOCPDDetector(cfg.(BOCPDConfig)) },
				defaultEnabled: true,
				parseJSON: func(defaults any, raw []byte) (any, error) {
					cfg := defaults.(BOCPDConfig)
					if err := json.Unmarshal(raw, &cfg); err != nil {
						return nil, err
					}
					return cfg, nil
				},
			},
			{
				name:           "rrcf",
				displayName:    "RRCF",
				kind:           componentDetector,
				defaultConfig:  DefaultRRCFConfig(),
				factory:        func(cfg any) any { return NewRRCFDetector(cfg.(RRCFConfig)) },
				defaultEnabled: true,
				parseJSON: func(defaults any, raw []byte) (any, error) {
					cfg := defaults.(RRCFConfig)
					if err := json.Unmarshal(raw, &cfg); err != nil {
						return nil, err
					}
					return cfg, nil
				},
			},
			{
				name:           "scanmw",
				displayName:    "ScanMW",
				kind:           componentDetector,
				factory:        func(any) any { return NewScanMWDetector() },
				defaultEnabled: false,
			},
			{
				name:           "scanwelch",
				displayName:    "ScanWelch",
				kind:           componentDetector,
				factory:        func(any) any { return NewScanWelchDetector() },
				defaultEnabled: false,
			},
			// ---- Correlators ----
			{
				name:           "cross_signal",
				displayName:    "CrossSignal",
				kind:           componentCorrelator,
				defaultConfig:  DefaultCorrelatorConfig(),
				factory:        func(cfg any) any { return NewCorrelator(cfg.(CorrelatorConfig)) },
				defaultEnabled: false,
				parseJSON: func(defaults any, raw []byte) (any, error) {
					cfg := defaults.(CorrelatorConfig)
					if err := json.Unmarshal(raw, &cfg); err != nil {
						return nil, err
					}
					return cfg, nil
				},
			},
			{
				name:           "time_cluster",
				displayName:    "TimeCluster",
				kind:           componentCorrelator,
				defaultConfig:  DefaultTimeClusterConfig(),
				factory:        func(cfg any) any { return NewTimeClusterCorrelator(cfg.(TimeClusterConfig)) },
				defaultEnabled: true,
				readConfig:     readTimeClusterConfig,
				parseJSON: func(defaults any, raw []byte) (any, error) {
					cfg := defaults.(TimeClusterConfig)
					if err := json.Unmarshal(raw, &cfg); err != nil {
						return nil, err
					}
					return cfg, nil
				},
			},
			{
				name:           "passthrough",
				displayName:    "Passthrough",
				kind:           componentCorrelator,
				factory:        func(any) any { return NewDetectorPassthroughCorrelator() },
				defaultEnabled: false,
			},
		},
	}
}

// Instantiate creates component instances. Settings provides per-component
// config and enabled values; anything not specified falls back to catalog
// defaults.
func (c *componentCatalog) Instantiate(settings ComponentSettings) (
	detectors []observerdef.Detector,
	correlators []observerdef.Correlator,
	extractors []observerdef.LogMetricsExtractor,
	components map[string]*componentInstance,
) {
	components = make(map[string]*componentInstance, len(c.entries))

	for _, entry := range c.entries {
		cfg := entry.defaultConfig
		if override, ok := settings.configs[entry.name]; ok {
			cfg = override
		}

		enabled := entry.defaultEnabled
		if override, ok := settings.Enabled[entry.name]; ok {
			enabled = override
		}

		instance := entry.factory(cfg)
		ci := &componentInstance{
			entry:        entry,
			instance:     instance,
			enabled:      enabled,
			activeConfig: cfg,
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

// CatalogEntry is a public view of a catalog component for CLI use.
type CatalogEntry struct {
	Name string
	Kind string // "detector", "correlator", or "extractor"
}

// TestbenchCatalogEntries returns all component names and kinds from the testbench catalog.
// Used by the CLI to implement --only without hardcoding component lists.
func TestbenchCatalogEntries() []CatalogEntry {
	cat := defaultCatalog()
	result := make([]CatalogEntry, len(cat.entries))
	for i, e := range cat.entries {
		kind := "unknown"
		switch e.kind {
		case componentDetector:
			kind = "detector"
		case componentCorrelator:
			kind = "correlator"
		case componentExtractor:
			kind = "extractor"
		}
		result[i] = CatalogEntry{Name: e.name, Kind: kind}
	}
	return result
}

// Entries returns a copy of all catalog entries (for UI/API use).
func (c *componentCatalog) Entries() []componentEntry {
	result := make([]componentEntry, len(c.entries))
	copy(result, c.entries)
	return result
}

// catalogEnabledDetectors returns the enabled Detector instances from a components map.
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

// catalogEnabledCorrelators returns the enabled Correlator instances from a components map.
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
