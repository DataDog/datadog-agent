// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"fmt"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// componentKind distinguishes detectors from correlators and extractors in the catalog.
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
	// and the key prefix "anomaly_detection.detectors.<name>.". It returns the
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
	IsConfigured(key string) bool
}

// ComponentSettings holds per-component configuration provided by the consumer.
// Both the live observer and the testbench build this from their respective
// config sources, giving a single path through instantiation.
type ComponentSettings struct {
	// Enabled maps component name to whether it should be active.
	// Components not listed use their catalog default.
	Enabled map[string]bool

	// Baseline controls the baseline analysis window.
	Baseline BaselineConfig

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
//	detectors, correlators, scorer, extractors, components := catalog.Instantiate(settings)
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
				defaultEnabled: false,
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
						return nil, fmt.Errorf("log_pattern_extractor: failed to parse JSON config: %w", err)
					}
					return cfg, nil
				},
			},
			{
				name:           LogTokenizerExtractorName,
				displayName:    "Log Tokenizer Extractor (Exact Hash)",
				kind:           componentExtractor,
				defaultConfig:  DefaultLogTokenizerExtractorConfig(),
				factory:        func(cfg any) any { return NewLogTokenizerExtractor(cfg.(LogTokenizerExtractorConfig)) },
				defaultEnabled: false,
				parseJSON: func(defaults any, raw []byte) (any, error) {
					cfg := defaults.(LogTokenizerExtractorConfig)
					if err := json.Unmarshal(raw, &cfg); err != nil {
						return nil, fmt.Errorf("%s: failed to parse JSON config: %w", LogTokenizerExtractorName, err)
					}
					return cfg, nil
				},
			},
			{
				name:           LogTokenizerFuzzyExtractorName,
				displayName:    "Log Tokenizer Extractor (Fuzzy Match)",
				kind:           componentExtractor,
				defaultConfig:  DefaultLogTokenizerFuzzyExtractorConfig(),
				factory:        func(cfg any) any { return NewLogTokenizerFuzzyExtractor(cfg.(LogTokenizerFuzzyExtractorConfig)) },
				defaultEnabled: false,
				parseJSON: func(defaults any, raw []byte) (any, error) {
					cfg := defaults.(LogTokenizerFuzzyExtractorConfig)
					if err := json.Unmarshal(raw, &cfg); err != nil {
						return nil, fmt.Errorf("%s: failed to parse JSON config: %w", LogTokenizerFuzzyExtractorName, err)
					}
					return cfg, nil
				},
			},
			{
				name:          LogSemanticTokenizerAblationExtractorName,
				displayName:   "Semantic Tokenizer Ablation Extractor",
				kind:          componentExtractor,
				defaultConfig: DefaultLogSemanticTokenizerAblationExtractorConfig(),
				factory: func(cfg any) any {
					return NewLogSemanticTokenizerAblationExtractor(cfg.(LogSemanticTokenizerAblationExtractorConfig))
				},
				defaultEnabled: false,
				parseJSON: func(defaults any, raw []byte) (any, error) {
					cfg := defaults.(LogSemanticTokenizerAblationExtractorConfig)
					if err := json.Unmarshal(raw, &cfg); err != nil {
						return nil, fmt.Errorf("%s: failed to parse JSON config: %w", LogSemanticTokenizerAblationExtractorName, err)
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
						return nil, fmt.Errorf("cusum: failed to parse JSON config: %w", err)
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
				readConfig: func(reader ConfigReader, prefix string) any {
					cfg := DefaultBOCPDConfig()
					if key := prefix + "warmup_points"; reader.IsConfigured(key) {
						cfg.WarmupPoints = reader.GetInt(key)
					}
					return cfg
				},
				parseJSON: func(defaults any, raw []byte) (any, error) {
					cfg := defaults.(BOCPDConfig)
					if err := json.Unmarshal(raw, &cfg); err != nil {
						return nil, fmt.Errorf("bocpd: failed to parse JSON config: %w", err)
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
						return nil, fmt.Errorf("rrcf: failed to parse JSON config: %w", err)
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
			{
				name:           "holt_residual",
				displayName:    "HoltResidual",
				kind:           componentDetector,
				defaultConfig:  DefaultHoltResidualConfig(),
				factory:        func(cfg any) any { return NewHoltResidualDetectorWithConfig(cfg.(HoltResidualConfig)) },
				defaultEnabled: false,
				parseJSON: func(defaults any, raw []byte) (any, error) {
					cfg := defaults.(HoltResidualConfig)
					if err := json.Unmarshal(raw, &cfg); err != nil {
						return nil, err
					}
					return cfg, nil
				},
			},
			{
				name:           "tukey_biweight",
				displayName:    "TukeyBiweight",
				kind:           componentDetector,
				defaultConfig:  DefaultTukeyBiweightConfig(),
				factory:        func(cfg any) any { return NewTukeyBiweightDetectorWithConfig(cfg.(TukeyBiweightConfig)) },
				defaultEnabled: false,
				parseJSON: func(defaults any, raw []byte) (any, error) {
					cfg := defaults.(TukeyBiweightConfig)
					if err := json.Unmarshal(raw, &cfg); err != nil {
						return nil, err
					}
					return cfg, nil
				},
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
						return nil, fmt.Errorf("cross_signal: failed to parse JSON config: %w", err)
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
						return nil, fmt.Errorf("time_cluster: failed to parse JSON config: %w", err)
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
			// ---- Anomaly Scorer (treated as a Correlator by the engine) ----
			{
				name:           "anomaly_scorer",
				displayName:    "AnomalyScorer",
				kind:           componentCorrelator,
				defaultConfig:  DefaultAnomalyScorerConfig(),
				factory:        func(cfg any) any { return NewAnomalyScorer(cfg.(AnomalyScorerConfig)) },
				defaultEnabled: false,
				parseJSON: func(defaults any, raw []byte) (any, error) {
					cfg := defaults.(AnomalyScorerConfig)
					if err := json.Unmarshal(raw, &cfg); err != nil {
						return nil, fmt.Errorf("anomaly_scorer: failed to parse JSON config: %w", err)
					}
					return cfg, nil
				},
			},
		},
	}
}

// Instantiate creates component instances. Settings provides per-component
// config and enabled values; anything not specified falls back to catalog
// defaults.
//
// scorer is the typed anomaly scorer pointer (may be nil when disabled or absent).
// It is NOT included in correlators — the engine handles that separately so it
// can set the typed engine.scorer pointer at the same time.
func (c *componentCatalog) Instantiate(settings ComponentSettings) (
	detectors []observerdef.Detector,
	correlators []observerdef.Correlator,
	scorer *anomalyScorer,
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
			// The anomaly_scorer entry is a componentCorrelator but returns an
			// *anomalyScorer, so we capture it separately instead of adding it to
			// correlators here. The engine/observer wires it in with telemetry.
			if sc, ok := instance.(*anomalyScorer); ok {
				scorer = sc
			} else if cor, ok := instance.(observerdef.Correlator); ok {
				correlators = append(correlators, cor)
			}
		case componentExtractor:
			if ext, ok := instance.(observerdef.LogMetricsExtractor); ok {
				extractors = append(extractors, ext)
			}
		}
	}
	return detectors, correlators, scorer, extractors, components
}

// CatalogEntry is a public view of a catalog component.
type CatalogEntry struct {
	Name           string
	DisplayName    string
	Kind           string // "detector", "correlator", or "extractor"
	DefaultEnabled bool
}

// ParseSettingsFromJSON builds ComponentSettings from a map of JSON-encoded
// per-component overrides (e.g. from a --config params file). Each value may
// contain an optional "enabled" bool plus component-specific hyperparameters.
// Unknown component names are rejected.
func ParseSettingsFromJSON(overrides map[string]json.RawMessage) (ComponentSettings, error) {
	cat := defaultCatalog()
	settings := ComponentSettings{
		Enabled: make(map[string]bool),
		configs: make(map[string]any),
	}
	for name, raw := range overrides {
		var entry *componentEntry
		for i := range cat.entries {
			if cat.entries[i].name == name {
				entry = &cat.entries[i]
				break
			}
		}
		if entry == nil {
			return ComponentSettings{}, fmt.Errorf("unknown component %q in params file", name)
		}
		var wrapper struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			return ComponentSettings{}, fmt.Errorf("parsing enabled for %q: %w", name, err)
		}
		if wrapper.Enabled != nil {
			settings.Enabled[name] = *wrapper.Enabled
		}
		if entry.parseJSON != nil {
			cfg, err := entry.parseJSON(entry.defaultConfig, raw)
			if err != nil {
				return ComponentSettings{}, fmt.Errorf("parsing config for %q: %w", name, err)
			}
			settings.configs[name] = cfg
		}
	}
	return settings, nil
}

// TestbenchCatalogEntries returns all component names and kinds from the testbench catalog.
// Used by the CLI to implement --only without hardcoding component lists.
func TestbenchCatalogEntries() []CatalogEntry {
	cat := defaultCatalog()
	result := make([]CatalogEntry, len(cat.entries))
	for i, e := range cat.entries {
		result[i] = CatalogEntry{
			Name:           e.name,
			DisplayName:    e.displayName,
			Kind:           kindString(e.kind),
			DefaultEnabled: e.defaultEnabled,
		}
	}
	return result
}

// Entries returns a copy of all catalog entries (for UI/API use).
func (c *componentCatalog) Entries() []componentEntry {
	result := make([]componentEntry, len(c.entries))
	copy(result, c.entries)
	return result
}

func kindString(k componentKind) string {
	switch k {
	case componentDetector:
		return "detector"
	case componentCorrelator:
		return "correlator"
	case componentExtractor:
		return "extractor"
	default:
		return "unknown"
	}
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
// Intended use: a unit test calls this against defaultCatalog() so any new
// detector added to the catalog without a teardown story fails CI before it
// can leak memory in production. SeriesDetector entries are validated against
// the SeriesRemover interface on the wrapping adapter (newSeriesDetectorAdapter
// always returns a *seriesDetectorAdapter, which implements SeriesRemover),
// matching what Instantiate produces at runtime.
func (c *componentCatalog) validateDetectorTeardownContract() error {
	for _, entry := range c.entries {
		if entry.kind != componentDetector {
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
				return &detectorTeardownContractError{name: entry.name, reason: "seriesDetectorAdapter no longer implements SeriesRemover — the wrapping invariant has regressed"}
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
	return fmt.Sprintf("detector %q violates teardown contract: %s", e.name, e.reason)
}
