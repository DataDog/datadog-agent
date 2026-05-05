// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
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
			{
				name:           "hellinger",
				displayName:    "Hellinger",
				kind:           componentDetector,
				factory:        func(any) any { return NewHellingerDetector() },
				defaultEnabled: true,
			},
			{
				name:           "hellingercp",
				displayName:    "HellingerCP",
				kind:           componentDetector,
				factory:        func(any) any { return NewHellingerCPDetector() },
				defaultEnabled: true,
			},
			{
				name:           "acorrshift",
				displayName:    "AcorrShift",
				kind:           componentDetector,
				factory:        func(any) any { return NewAcorrShiftDetector() },
				defaultEnabled: false,
			},
			{
				name:           "burgar",
				displayName:    "BurgAR",
				kind:           componentDetector,
				factory:        func(any) any { return NewBurgarDetector() },
				defaultEnabled: false,
			},
			{
				name:           "cross_decorrelate",
				displayName:    "CrossDecorrelate",
				kind:           componentDetector,
				factory:        func(any) any { return NewCrossDecorrelateDetector() },
				defaultEnabled: false,
			},
			{
				name:           "denratio",
				displayName:    "DensityRatio",
				kind:           componentDetector,
				factory:        func(any) any { return NewDenRatioDetector() },
				defaultEnabled: false,
			},
			{
				name:           "dfa_hurst",
				displayName:    "DFAHurst",
				kind:           componentDetector,
				factory:        func(any) any { return NewDFAHurstDetector() },
				defaultEnabled: false,
			},
			{
				name:           "dirbf",
				displayName:    "DirichletBF",
				kind:           componentDetector,
				factory:        func(any) any { return NewDIRBFDetector() },
				defaultEnabled: false,
			},
			{
				name:           "energydist",
				displayName:    "EnergyDistance",
				kind:           componentDetector,
				factory:        func(any) any { return NewEnergyDistDetector() },
				defaultEnabled: false,
			},
			{
				name:           "esn",
				displayName:    "ESN",
				kind:           componentDetector,
				factory:        func(any) any { return NewESNDetector() },
				defaultEnabled: false,
			},
			{
				name:           "evt_spot",
				displayName:    "EVTSpot",
				kind:           componentDetector,
				factory:        func(any) any { return NewEVTSpotDetector() },
				defaultEnabled: false,
			},
			{
				name:           "garch_volatility",
				displayName:    "GARCHVolatility",
				kind:           componentDetector,
				factory:        func(any) any { return NewGarchVolatilityDetector() },
				defaultEnabled: false,
			},
			{
				name:           "glr_mean_variance",
				displayName:    "GLRMeanVariance",
				kind:           componentDetector,
				factory:        func(any) any { return NewGLRMeanVarianceDetector() },
				defaultEnabled: false,
			},
			{
				name:           "grubbs_loo",
				displayName:    "GrubbsLOO",
				kind:           componentDetector,
				factory:        func(any) any { return NewGrubbsLOODetector() },
				defaultEnabled: false,
			},
			{
				name:           "hi_moments",
				displayName:    "HiMoments",
				kind:           componentDetector,
				factory:        func(any) any { return NewHiMomentsDetector() },
				defaultEnabled: false,
			},
			{
				name:           "hl_shift",
				displayName:    "HodgesLehmannShift",
				kind:           componentDetector,
				factory:        func(any) any { return NewHLShiftDetector() },
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
				name:           "hot_sax",
				displayName:    "HotSAX",
				kind:           componentDetector,
				factory:        func(any) any { return NewHotSAXDetector() },
				defaultEnabled: false,
			},
			{
				name:           "hwres",
				displayName:    "HoltWintersResidual",
				kind:           componentDetector,
				factory:        func(any) any { return NewHWResDetector() },
				defaultEnabled: false,
			},
			{
				name:           "kl_divergence",
				displayName:    "KLDivergence",
				kind:           componentDetector,
				factory:        func(any) any { return NewKLDivergenceDetector() },
				defaultEnabled: false,
			},
			{
				name:           "ks_drift",
				displayName:    "KSDrift",
				kind:           componentDetector,
				factory:        func(any) any { return NewKSDriftDetector() },
				defaultEnabled: false,
			},
			{
				name:           "mannkendall",
				displayName:    "MannKendall",
				kind:           componentDetector,
				factory:        func(any) any { return NewMannKendallDetector() },
				defaultEnabled: false,
			},
			{
				name:           "matrix_profile",
				displayName:    "MatrixProfile",
				kind:           componentDetector,
				factory:        func(any) any { return NewMatrixProfileDetector() },
				defaultEnabled: false,
			},
			{
				name:           "mmd_rff",
				displayName:    "MMDRFF",
				kind:           componentDetector,
				factory:        func(any) any { return NewMMDRFFDetector() },
				defaultEnabled: false,
			},
			{
				name:           "mmdrff",
				displayName:    "MMDRFFTwoSample",
				kind:           componentDetector,
				factory:        func(any) any { return NewMMDRFFTwoSampleDetector() },
				defaultEnabled: false,
			},
			{
				name:           "permentropy",
				displayName:    "PermutationEntropy",
				kind:           componentDetector,
				factory:        func(any) any { return NewPermEntropyDetector() },
				defaultEnabled: false,
			},
			{
				name:           "pht",
				displayName:    "PageHinkley",
				kind:           componentDetector,
				factory:        func(any) any { return NewPHTDetector() },
				defaultEnabled: false,
			},
			{
				name:           "shapediscord",
				displayName:    "ShapeDiscord",
				kind:           componentDetector,
				factory:        func(any) any { return NewShapeDiscordDetector() },
				defaultEnabled: false,
			},
			{
				name:           "spectral_residual",
				displayName:    "SpectralResidual",
				kind:           componentDetector,
				factory:        func(any) any { return NewSpectralResidualDetector() },
				defaultEnabled: false,
			},
			{
				name:           "stl_seasonal",
				displayName:    "STLSeasonal",
				kind:           componentDetector,
				factory:        func(any) any { return NewSTLSeasonalDetector() },
				defaultEnabled: false,
			},
			{
				name:           "trendresid",
				displayName:    "TrendResidual",
				kind:           componentDetector,
				factory:        func(any) any { return NewTrendResidDetector() },
				defaultEnabled: false,
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
			{
				name:           "varshift",
				displayName:    "VarianceShift",
				kind:           componentDetector,
				factory:        func(any) any { return NewVarShiftDetector() },
				defaultEnabled: false,
			},
			{
				name:           "wasserstein",
				displayName:    "Wasserstein",
				kind:           componentDetector,
				factory:        func(any) any { return NewWassersteinDetector() },
				defaultEnabled: false,
			},
			{
				name:           "wsr",
				displayName:    "WilcoxonSignedRank",
				kind:           componentDetector,
				factory:        func(any) any { return NewWSRDetector() },
				defaultEnabled: false,
			},
			{
				// LODA (Lightweight On-line Detector of Anomalies, Pevný 2016)
				// — sparse-projection ensemble over a 5-D synthetic feature
				// vector, scored against discounted equi-width histograms.
				// Disabled by default: the detector is on the same
				// "earn-your-place" track as scanmw/scanwelch — flip to true
				// only after a positive evaluation, and never as part of an
				// unrelated change. TestLODA_DefaultEnabledIsFalse guards
				// this default.
				name:           "loda",
				displayName:    "LODA",
				kind:           componentDetector,
				factory:        func(any) any { return NewLODADetector() },
				defaultEnabled: false,
			},
			{
				// Dempster-Shafer evidence-combination correlator. Treats each
				// detector's anomaly as a Basic Probability Assignment over the
				// frame {Anomalous, Normal}, fuses BPAs on the same series via
				// Dempster's rule, and emits correlations when fused belief
				// in {Anomalous} clears a threshold AND conflict K is below
				// a ceiling. Unlike K-of-N consensus this does NOT suppress
				// single high-confidence detector fires, which is the explicit
				// anti-pattern from exp-0070.
				//
				// Disabled in favor of lord_fdr_correlator; kept registered
				// for --only eval comparison.
				name:           "dempster_shafer_correlator",
				displayName:    "Dempster-Shafer Correlator",
				kind:           componentCorrelator,
				factory:        func(any) any { return NewDempsterShaferCorrelator() },
				defaultEnabled: false,
			},
			// ---- Correlators ----
			{
				name:           "anomaly_rank",
				displayName:    "AnomalyRank",
				kind:           componentCorrelator,
				defaultConfig:  DefaultAnomalyRankConfig(),
				factory:        func(cfg any) any { return NewAnomalyRankCorrelator(cfg.(AnomalyRankConfig)) },
				defaultEnabled: true,
				parseJSON: func(defaults any, raw []byte) (any, error) {
					cfg := defaults.(AnomalyRankConfig)
					if err := json.Unmarshal(raw, &cfg); err != nil {
						return nil, err
					}
					return cfg, nil
				},
			},
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
			{
				// LORD-1 online-FDR correlator (Javanmard & Montanari 2018,
				// "Online rules for control of false discovery rate"). Converts
				// each anomaly's Score to a synthetic p-value and applies the
				// LORD-1 wealth dynamics to decide whether to emit a
				// corresponding ActiveCorrelation, capping the long-run FDR at
				// Alpha=0.10.
				//
				// Ships defaultEnabled=true to replace dempster_shafer_correlator
				// (now flipped to defaultEnabled=false), so the count of default-
				// enabled correlators is unchanged at 2 (time_cluster + lord_fdr).
				name:           "lord_fdr_correlator",
				displayName:    "LORD-FDR Correlator",
				kind:           componentCorrelator,
				factory:        func(any) any { return NewLORDFDRCorrelator() },
				defaultEnabled: true,
			},
			{
				name:           "rankflip_correlator",
				displayName:    "RankFlipCorrelator",
				kind:           componentCorrelator,
				factory:        func(any) any { return NewRankFlipCorrelator() },
				defaultEnabled: false,
			},
			{
				name:           "affinity_cluster_correlator",
				displayName:    "AffinityClusterCorrelator",
				kind:           componentCorrelator,
				factory:        func(any) any { return NewAffinityClusterCorrelator(DefaultAffinityClusterConfig()) },
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
