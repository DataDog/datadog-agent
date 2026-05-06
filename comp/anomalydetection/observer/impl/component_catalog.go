// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// ComponentKind distinguishes detectors from correlators in the catalog.
type ComponentKind int

const (
	ComponentDetector ComponentKind = iota
	ComponentCorrelator
	ComponentExtractor
)

// ComponentEntry describes a registered pipeline component.
//
// Each entry pairs a default config with a factory function. The factory
// accepts the config and returns the constructed component. This separation
// lets consumers provide config from any source (agent config, testbench UI)
// without replacing the factory itself.
type ComponentEntry struct {
	name           string
	displayName    string
	kind           ComponentKind
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

// ComponentInstance tracks a component entry paired with its runtime instance and enabled state.
type ComponentInstance struct {
	entry        ComponentEntry
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

// ComponentCatalog is the shared registry of all available pipeline components.
// Both the live observer and the testbench use this to discover and instantiate
// detectors, correlators, and extractors.
//
// Usage:
//
//	catalog := DefaultCatalog()
//	settings := ComponentSettings{ ... } // from agent config, testbench UI, etc.
//	detectors, correlators, extractors, components := catalog.Instantiate(settings)
type ComponentCatalog struct {
	entries []ComponentEntry
}

// DefaultCatalog returns the component catalog with all known components and
// their default configs. This is the single starting point for both the live
// observer and the testbench — they diverge only in what ComponentSettings
// they pass to Instantiate.
func DefaultCatalog() *ComponentCatalog {
	return &ComponentCatalog{
		entries: []ComponentEntry{
			// ---- Extractors ----
			{
				name:           "log_metrics_extractor",
				displayName:    "Log Metrics Extractor",
				kind:           ComponentExtractor,
				defaultConfig:  DefaultLogMetricsExtractorConfig(),
				factory:        func(cfg any) any { return NewLogMetricsExtractor(cfg.(LogMetricsExtractorConfig)) },
				defaultEnabled: true,
			},
			{
				name:           "connection_error_extractor",
				displayName:    "Connection Error Extractor",
				kind:           ComponentExtractor,
				defaultConfig:  DefaultConnectionErrorExtractorConfig(),
				factory:        func(any) any { return &ConnectionErrorExtractor{} },
				defaultEnabled: true,
			},
			{
				name:           "log_pattern_extractor",
				displayName:    "Log Pattern Extractor",
				kind:           ComponentExtractor,
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
				kind:           ComponentDetector,
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
				kind:           ComponentDetector,
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
				kind:           ComponentDetector,
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
				kind:           ComponentDetector,
				factory:        func(any) any { return NewScanMWDetector() },
				defaultEnabled: false,
			},
			{
				name:           "scanwelch",
				displayName:    "ScanWelch",
				kind:           ComponentDetector,
				factory:        func(any) any { return NewScanWelchDetector() },
				defaultEnabled: false,
			},
			// ---- Correlators ----
			{
				name:           "cross_signal",
				displayName:    "CrossSignal",
				kind:           ComponentCorrelator,
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
				kind:           ComponentCorrelator,
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
				kind:           ComponentCorrelator,
				factory:        func(any) any { return NewDetectorPassthroughCorrelator() },
				defaultEnabled: false,
			},
		},
	}
}

// Instantiate creates component instances. Settings provides per-component
// config and enabled values; anything not specified falls back to catalog
// defaults.
func (c *ComponentCatalog) Instantiate(settings ComponentSettings) (
	detectors []observerdef.Detector,
	correlators []observerdef.Correlator,
	extractors []observerdef.LogMetricsExtractor,
	components map[string]*ComponentInstance,
) {
	components = make(map[string]*ComponentInstance, len(c.entries))

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
		ci := &ComponentInstance{
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
		case ComponentDetector:
			if d, ok := instance.(observerdef.Detector); ok {
				detectors = append(detectors, d)
			} else if sd, ok := instance.(observerdef.SeriesDetector); ok {
				detectors = append(detectors, newSeriesDetectorAdapter(sd, defaultAggregations))
			}
		case ComponentCorrelator:
			if cor, ok := instance.(observerdef.Correlator); ok {
				correlators = append(correlators, cor)
			}
		case ComponentExtractor:
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
	cat := DefaultCatalog()
	result := make([]CatalogEntry, len(cat.entries))
	for i, e := range cat.entries {
		kind := "unknown"
		switch e.kind {
		case ComponentDetector:
			kind = "detector"
		case ComponentCorrelator:
			kind = "correlator"
		case ComponentExtractor:
			kind = "extractor"
		}
		result[i] = CatalogEntry{Name: e.name, Kind: kind}
	}
	return result
}

// Entries returns a copy of all catalog entries (for UI/API use).
func (c *ComponentCatalog) Entries() []ComponentEntry {
	result := make([]ComponentEntry, len(c.entries))
	copy(result, c.entries)
	return result
}

// ParseComponentConfig parses hyperparameters from a JSON blob into settings for
// the given component entry. It starts from the entry's default config and
// overlays JSON values. Returns an error if the component has no JSON parser or
// if parsing fails.
//
// This is provided so callers outside the package can implement functionality
// equivalent to LoadTestbenchParams without needing access to unexported fields.
func (c *ComponentCatalog) ParseComponentConfig(settings *ComponentSettings, entry ComponentEntry, raw []byte) error {
	if entry.parseJSON == nil || entry.defaultConfig == nil {
		return nil // component has no tunable hyperparameters
	}
	cfg, err := entry.parseJSON(entry.defaultConfig, raw)
	if err != nil {
		return err
	}
	if settings.configs == nil {
		settings.configs = make(map[string]any)
	}
	settings.configs[entry.name] = cfg
	return nil
}

// Name returns the component entry's name (e.g. "bocpd", "time_cluster").
func (e ComponentEntry) Name() string { return e.name }

// DisplayName returns the human-readable display name for the component entry.
func (e ComponentEntry) DisplayName() string { return e.displayName }

// EntryKind returns the component kind (detector, correlator, or extractor).
func (e ComponentEntry) EntryKind() ComponentKind { return e.kind }

// Instance returns the runtime component instance created by the factory.
func (ci *ComponentInstance) Instance() any { return ci.instance }

// Kind returns the component kind (detector, correlator, or extractor).
func (ci *ComponentInstance) Kind() ComponentKind { return ci.entry.kind }

// Enabled returns whether this component instance is currently enabled.
func (ci *ComponentInstance) Enabled() bool { return ci.enabled }

// Toggle flips the enabled state of the component instance and returns the new state.
func (ci *ComponentInstance) Toggle() bool {
	ci.enabled = !ci.enabled
	return ci.enabled
}

// ActiveConfig returns the config actually passed to the factory (nil for
// parameterless components).
func (ci *ComponentInstance) ActiveConfig() any { return ci.activeConfig }

// NewComponentInstanceForTest creates a ComponentInstance with the given name,
// kind, and enabled state. Intended for use in tests outside this package.
func NewComponentInstanceForTest(name string, kind ComponentKind, enabled bool) *ComponentInstance {
	return &ComponentInstance{
		entry:   ComponentEntry{name: name, kind: kind},
		enabled: enabled,
	}
}

// CatalogEnabledDetectors returns the enabled Detector instances from a components map.
// SeriesDetector implementations are wrapped with seriesDetectorAdapter.
func CatalogEnabledDetectors(components map[string]*ComponentInstance, catalog *ComponentCatalog) []observerdef.Detector {
	var result []observerdef.Detector
	// Iterate in catalog order for deterministic ordering
	for _, entry := range catalog.entries {
		ci, ok := components[entry.name]
		if !ok || !ci.enabled || ci.entry.kind != ComponentDetector {
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

// CatalogEnabledExtractors returns the enabled LogMetricsExtractor instances from a components map.
func CatalogEnabledExtractors(components map[string]*ComponentInstance, catalog *ComponentCatalog) []observerdef.LogMetricsExtractor {
	var result []observerdef.LogMetricsExtractor
	for _, entry := range catalog.entries {
		ci, ok := components[entry.name]
		if !ok || !ci.enabled || ci.entry.kind != ComponentExtractor {
			continue
		}
		if ext, ok := ci.instance.(observerdef.LogMetricsExtractor); ok {
			result = append(result, ext)
		}
	}
	return result
}

// CatalogEnabledCorrelators returns the enabled Correlator instances from a components map.
func CatalogEnabledCorrelators(components map[string]*ComponentInstance, catalog *ComponentCatalog) []observerdef.Correlator {
	var result []observerdef.Correlator
	for _, entry := range catalog.entries {
		ci, ok := components[entry.name]
		if !ok || !ci.enabled || ci.entry.kind != ComponentCorrelator {
			continue
		}
		if cor, ok := ci.instance.(observerdef.Correlator); ok {
			result = append(result, cor)
		}
	}
	return result
}

// statelessDetectorAllowlist enumerates catalog detectors that are explicitly
// permitted to NOT implement observerdef.SeriesRemover. A stateless detector
// keeps no per-series state (no posterior maps, no segment trackers, no
// visible-count tracking) and therefore needs nothing freed when storage
// evicts a series; the engine's fanOutSeriesRemoval safely no-ops on it.
//
// Any new entry added here is asserting "this detector is genuinely stateless
// across detect calls". If a detector ever grows per-series memory (cache,
// tracker, accumulator keyed by SeriesRef), it must implement SeriesRemover
// and be removed from this list — otherwise its memory grows with the
// cumulative number of series ever observed even after storage evicts them.
var statelessDetectorAllowlist = map[string]struct{}{
	// SeriesDetector implementations are wrapped at instantiation time by
	// seriesDetectorAdapter, which itself implements SeriesRemover — so the
	// raw SeriesDetector struct doesn't need to. The adapter handles teardown
	// of its own lastVisibleCount cache and forwards to the wrapped detector
	// only if it also satisfies SeriesRemover.
	//
	// Truly-stateless catalog Detectors are listed below.

	// RRCF tracks a FIXED set of metric definitions configured at construction
	// (RRCFConfig.Metrics, with DefaultRRCFMetrics() as the fallback). Its
	// resolvedKeys / cursors maps are keyed by cursorKey (a metric definition
	// identifier), not by ingested SeriesRef — so the map size is bounded by
	// the configured metrics, not by storage cardinality. Adding storage-eviction
	// fan-out would not free anything because RRCF state isn't keyed by SeriesRef.
	// If RRCF is ever extended to track per-tag-combination state keyed by
	// SeriesRef, this entry must be removed and RRCF must implement
	// SeriesRemover.
	"rrcf": {},
}

// validateDetectorTeardownContract checks that every detector entry in the
// catalog either implements observerdef.SeriesRemover (so engine eviction
// fan-out can free its per-series state) or is explicitly listed in
// statelessDetectorAllowlist. Returns nil on success and a descriptive error
// on the first violator.
//
// Intended use: a unit test calls this against DefaultCatalog() so any new
// detector added to the catalog without a teardown story fails CI before it
// can leak memory in production. SeriesDetector entries are validated against
// the SeriesRemover interface on the wrapping adapter (newSeriesDetectorAdapter
// always returns a *seriesDetectorAdapter, which implements SeriesRemover),
// matching what Instantiate produces at runtime.
func (c *ComponentCatalog) validateDetectorTeardownContract() error {
	for _, entry := range c.entries {
		if entry.kind != ComponentDetector {
			continue
		}
		if _, allowed := statelessDetectorAllowlist[entry.name]; allowed {
			continue
		}
		// Build the same instance Instantiate would. We use defaultConfig
		// because the contract under test is structural, not config-dependent.
		instance := entry.factory(entry.defaultConfig)

		// Mirror Instantiate's wrapping logic: SeriesDetector is wrapped
		// in seriesDetectorAdapter, which is a SeriesRemover. A direct
		// Detector implementation must be a SeriesRemover itself.
		if sd, ok := instance.(observerdef.SeriesDetector); ok {
			wrapped := newSeriesDetectorAdapter(sd, defaultAggregations)
			if _, ok := any(wrapped).(observerdef.SeriesRemover); !ok {
				return &detectorTeardownContractError{name: entry.name, reason: "seriesDetectorAdapter no longer implements SeriesRemover \u2014 the wrapping invariant has regressed"}
			}
			continue
		}
		if d, ok := instance.(observerdef.Detector); ok {
			if _, ok := d.(observerdef.SeriesRemover); ok {
				continue
			}
			return &detectorTeardownContractError{name: entry.name, reason: "detector neither implements observerdef.SeriesRemover nor is listed in statelessDetectorAllowlist"}
		}
		return &detectorTeardownContractError{name: entry.name, reason: "factory product is neither observerdef.Detector nor observerdef.SeriesDetector"}
	}
	return nil
}

// detectorTeardownContractError marks a catalog entry that violates the
// SeriesRemover contract.
type detectorTeardownContractError struct {
	name   string
	reason string
}

func (e *detectorTeardownContractError) Error() string {
	return "detector \"" + e.name + "\" violates teardown contract: " + e.reason
}
