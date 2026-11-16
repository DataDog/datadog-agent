// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// TestDefaultCatalog_DetectorTeardownContract is the structural guard that
// every catalog detector implements observerdef.SeriesRemover. Without this, a new
// detector with per-series state can be added to the catalog and silently
// leak memory in production: storage eviction will free the series, but the
// detector's per-series map will never shrink.
func TestDefaultCatalog_DetectorTeardownContract(t *testing.T) {
	require.NoError(t, defaultCatalog().validateDetectorTeardownContract(),
		"every catalog detector must implement SeriesRemover")
}

func TestDefaultCatalog_EnabledDetectors(t *testing.T) {
	settings := ComponentSettings{UseMetricPipeline: true}
	detectors, _, _, _, _ := instantiateForTest(t, defaultCatalog(), settings)
	require.Equal(t, []string{"stable_pipeline:scanmw"}, detectorNames(detectors))
	pipeline := detectors[0].detector.(*MetricPipeline)
	stable := pipeline.registrations[0].detector.(*StablePipeline)
	require.NotNil(t, stable.gate, "default stable pipeline must gate at 5%")
	require.InDelta(t, 0.05, stable.gate.minChange, 1e-12)

	detectors, _, _, _, _ = instantiateForTest(t, defaultCatalog(), ComponentSettings{})
	require.Equal(t, []string{"bocpd"}, detectorNames(detectors))
}

func TestDetectorWorkflowSelection(t *testing.T) {
	for _, mode := range []struct {
		name          string
		pipeline      bool
		enabled       map[string]bool
		wantDetectors []string
	}{
		{
			name:          "pipeline defaults",
			pipeline:      true,
			wantDetectors: []string{"stable_pipeline:scanmw"},
		},
		{
			name:          "pipeline ignores enabled legacy detectors",
			pipeline:      true,
			enabled:       map[string]bool{"bocpd": true, "holt_residual": true, "tukey_biweight": true},
			wantDetectors: []string{"stable_pipeline:scanmw"},
		},
		{
			name:          "pipeline ignores scanmw disable",
			pipeline:      true,
			enabled:       map[string]bool{"scanmw": false},
			wantDetectors: []string{"stable_pipeline:scanmw"},
		},
		{
			name:          "legacy keeps catalog defaults",
			wantDetectors: []string{"bocpd"},
		},
		{
			name:          "legacy honors explicit selections",
			enabled:       map[string]bool{"scanmw": true, "scanwelch": true},
			wantDetectors: []string{"bocpd", "scanmw", "scanwelch"},
		},
	} {
		t.Run(mode.name, func(t *testing.T) {
			settings := ComponentSettings{
				Enabled:           mode.enabled,
				UseMetricPipeline: mode.pipeline,
			}
			detectors, _, _, _, components := instantiateForTest(t, defaultCatalog(), settings)
			require.Equal(t, mode.wantDetectors, detectorNames(detectors))
			if len(mode.wantDetectors) > 0 &&
				(slices.Contains(mode.wantDetectors, "scanmw") || slices.Contains(mode.wantDetectors, "stable_pipeline:scanmw")) {
				if !mode.pipeline {
					scanmw := components["scanmw"]
					require.True(t, scanmw.enabled)
					require.IsType(t, &ScanMWDetector{}, scanmw.instance)
				} else {
					require.IsType(t, &MetricPipeline{}, detectors[0].detector)
				}
			}
		})
	}
}

func TestSettingsFromAgentConfigWorkflow(t *testing.T) {
	cfg := configmock.NewFromYAML(t, `
anomaly_detection:
  detectors:
    scanmw:
      min_points: 42
      max_points: 84
`)
	settings := settingsFromAgentConfig(defaultCatalog(), cfg)
	require.False(t, settings.UseMetricPipeline)

	cfg = configmock.NewFromYAML(t, `
anomaly_detection:
  detectors:
    scanmw:
      min_points: 42
      max_points: 84
  metric_pipeline:
    enabled: true
`)
	settings = settingsFromAgentConfig(defaultCatalog(), cfg)
	require.True(t, settings.UseMetricPipeline)
	require.Nil(t, settings.StablePipeline, "absent setting must select defaults")
	require.Nil(t, settings.MetricPipelineAnomalyCutoff)
	detectors, _, _, _, _ := instantiateForTest(t, defaultCatalog(), settings)
	require.Equal(t, []string{"stable_pipeline:scanmw"}, detectorNames(detectors))
	pipeline := detectors[0].detector.(*MetricPipeline)
	stable := pipeline.registrations[0].detector.(*StablePipeline)
	require.Equal(t, observerdef.DetectorPointWindow{MinPoints: 30, MaxPoints: 120}, pipeline.DetectorPointWindow())
	require.NotNil(t, stable.gate, "absent stable config selects the default gate")
	require.True(t, pipeline.anomalyCutoff)
}

func TestSettingsFromAgentConfigStablePipeline(t *testing.T) {
	cfg := configmock.NewFromYAML(t, `
anomaly_detection:
  metric_pipeline:
    enabled: true
    stable:
      min_relative_change: 0
`)
	settings := settingsFromAgentConfig(defaultCatalog(), cfg)
	require.NotNil(t, settings.StablePipeline)
	require.Zero(t, settings.StablePipeline.MinRelativeChange)
	detectors, _, _, _, _ := instantiateForTest(t, defaultCatalog(), settings)
	pipeline := detectors[0].detector.(*MetricPipeline)
	stable := pipeline.registrations[0].detector.(*StablePipeline)
	require.Nil(t, stable.gate, "explicit zero must construct no gate")
	require.True(t, pipeline.anomalyCutoff)

	cfg = configmock.NewFromYAML(t, `
anomaly_detection:
  metric_pipeline:
    enabled: true
    stable:
      min_relative_change: 0.2
`)
	settings = settingsFromAgentConfig(defaultCatalog(), cfg)
	require.NotNil(t, settings.StablePipeline)
	require.InDelta(t, 0.2, settings.StablePipeline.MinRelativeChange, 1e-12)
	detectors, _, _, _, _ = instantiateForTest(t, defaultCatalog(), settings)
	pipeline = detectors[0].detector.(*MetricPipeline)
	stable = pipeline.registrations[0].detector.(*StablePipeline)
	require.NotNil(t, stable.gate)
	require.InDelta(t, 0.2, stable.gate.minChange, 1e-12)
}

func TestScanDetectors_CatalogConfiguration(t *testing.T) {
	for _, name := range []string{"scanmw", "scanwelch"} {
		for _, source := range []string{"agent", "json"} {
			for _, enabled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/enabled=%t", name, source, enabled), func(t *testing.T) {
					var settings ComponentSettings
					if source == "agent" {
						cfg := configmock.NewFromYAML(t, fmt.Sprintf(`
anomaly_detection:
  detectors:
    %s:
      enabled: %t
      min_points: 42
      max_points: 84
`, name, enabled))
						settings = settingsFromAgentConfig(defaultCatalog(), cfg)
					} else {
						var err error
						settings, err = ParseSettingsFromJSON(map[string]json.RawMessage{
							name: json.RawMessage(fmt.Sprintf(
								`{"enabled":%t,"min_points":42,"max_points":84}`,
								enabled)),
						})
						require.NoError(t, err)
					}
					detectors, _, _, _, components := instantiateForTest(t, defaultCatalog(), settings)
					var want, got any
					switch d := components[name].instance.(type) {
					case *ScanMWDetector:
						cfg := DefaultScanMWConfig()
						cfg.MinPoints, cfg.MaxPoints = 42, 84
						want, got = cfg, d.config
					case *ScanWelchDetector:
						cfg := DefaultScanWelchConfig()
						cfg.MinPoints, cfg.MaxPoints = 42, 84
						want, got = cfg, d.config
					default:
						t.Fatalf("unexpected detector: %T", d)
					}
					require.Equal(t, want, components[name].activeConfig)
					require.Equal(t, want, got)
					require.Equal(t, enabled, components[name].enabled)
					if enabled {
						require.Contains(t, detectorNames(detectors), name)
					} else {
						require.NotContains(t, detectorNames(detectors), name)
					}
				})
			}
		}
	}
}

// Every catalog detector can be enabled through anomaly_detection.detectors.*.
// Check all of them, including those disabled by default, so a new detector
// cannot silently use FormatAnomaly's generic fallback when it starts emitting.
func TestDefaultCatalog_EmittingDetectorsHaveAnomalyFormatter(t *testing.T) {
	for _, entry := range defaultCatalog().Entries() {
		if entry.kind != componentDetector {
			continue
		}
		t.Run(entry.name, func(t *testing.T) {
			detector, ok := entry.factory(entry.defaultConfig).(interface{ Name() string })
			require.True(t, ok, "catalog detector must expose its runtime name")
			require.Equal(t, entry.name, detector.Name())

			anomaly := observerdef.Anomaly{
				Source:       observerdef.SeriesDescriptor{Name: "metric", Aggregate: observerdef.AggregateAverage},
				DetectorName: detector.Name(),
				DebugInfo: &observerdef.AnomalyDebugInfo{
					BOCPDTrigger: observerdef.BOCPDTriggerChangePointProbability,
				},
			}
			title, description := observerdef.FormatAnomaly(anomaly)
			require.NotEqual(t, "Anomaly detected: "+detector.Name()+": metric:avg", title,
				"configurable detector is missing an anomaly formatter")
			require.NotEmpty(t, description, "configurable detector is missing an anomaly description")
		})
	}
}

func TestTestbenchCatalogAndSettingsIncludePassthrough(t *testing.T) {
	found := false
	for _, entry := range TestbenchCatalogEntries() {
		if entry.Name == TestbenchPassthroughComponentName {
			found = true
			require.Equal(t, "correlator", entry.Kind)
			require.False(t, entry.DefaultEnabled)
		}
	}
	require.True(t, found)

	settings, err := ParseSettingsFromJSON(map[string]json.RawMessage{
		TestbenchPassthroughComponentName: json.RawMessage(`{"enabled":true}`),
	})
	require.NoError(t, err)
	require.True(t, settings.Enabled[TestbenchPassthroughComponentName])
}

// TestValidateDetectorTeardownContract_FlagsBareDetector confirms the
// validator rejects a Detector that doesn't implement SeriesRemover.
func TestValidateDetectorTeardownContract_FlagsBareDetector(t *testing.T) {
	cat := &componentCatalog{
		entries: []componentEntry{
			{
				name:           "bare-detector",
				kind:           componentDetector,
				factory:        func(any) any { return &bareDetectorForValidator{} },
				defaultEnabled: true,
			},
		},
	}
	err := cat.validateDetectorTeardownContract()
	require.Error(t, err)
	var contractErr *detectorTeardownContractError
	require.True(t, errors.As(err, &contractErr), "error must be detectorTeardownContractError")
	require.Equal(t, "bare-detector", contractErr.name)
}

func TestApplyTestbenchDefaults(t *testing.T) {
	settings := ApplyTestbenchDefaults(ComponentSettings{})

	require.Equal(t, 40, settings.configs["bocpd"].(BOCPDConfig).WarmupPoints)
	holt := settings.configs["holt_residual"].(HoltResidualConfig)
	require.Equal(t, 15, holt.WarmupPoints)
	require.Equal(t, 25, holt.ResidualWindow)
	tukey := settings.configs["tukey_biweight"].(TukeyBiweightConfig)
	require.Equal(t, 40, tukey.WindowSize)
	require.Equal(t, 40, tukey.MinPoints)
	require.True(t, settings.Enabled["anomaly_scorer"])
	require.NotContains(t, settings.Enabled, "time_cluster")
	scorer := settings.configs["anomaly_scorer"].(AnomalyScorerConfig)
	require.True(t, scorer.CorrelationEvents)
	require.Equal(t, DefaultAnomalyScorerConfig().CooldownSecs, scorer.CooldownSecs)
	require.NotNil(t, settings.StablePipeline)
	require.InDelta(t, defaultStableMinRelativeChange, settings.StablePipeline.MinRelativeChange, 1e-12)
}

func TestApplyTestbenchDefaults_PreservesExplicitConfig(t *testing.T) {
	settings, err := ParseSettingsFromJSON(map[string]json.RawMessage{
		"bocpd":          json.RawMessage(`{"warmup_points": 42}`),
		"scanmw":         json.RawMessage(`{"min_points": 42, "max_points": 84}`),
		"scanwelch":      json.RawMessage(`{"min_points": 42, "max_points": 84}`),
		"anomaly_scorer": json.RawMessage(`{"enabled":false,"cooldown_secs":0}`),
		"time_cluster":   json.RawMessage(`{"enabled":true}`),
	})
	require.NoError(t, err)

	settings = ApplyTestbenchDefaults(settings)
	require.Equal(t, 42, settings.configs["bocpd"].(BOCPDConfig).WarmupPoints)
	scanMW := settings.configs["scanmw"].(ScanMWConfig)
	require.Equal(t, 42, scanMW.MinPoints)
	require.Equal(t, 84, scanMW.MaxPoints)
	scanWelch := settings.configs["scanwelch"].(ScanWelchConfig)
	require.Equal(t, 42, scanWelch.MinPoints)
	require.Equal(t, 84, scanWelch.MaxPoints)
	require.False(t, settings.Enabled["anomaly_scorer"])
	require.Zero(t, settings.configs["anomaly_scorer"].(AnomalyScorerConfig).CooldownSecs)
	require.True(t, settings.Enabled["time_cluster"])
}

// bareDetectorForValidator is a minimal observerdef.Detector that
// intentionally does NOT implement SeriesRemover — used to drive the
// negative cases of validateDetectorTeardownContract.
type bareDetectorForValidator struct{}

func (*bareDetectorForValidator) Name() string { return "bare-detector" }
func (*bareDetectorForValidator) Ready() bool  { return true }
func (*bareDetectorForValidator) Detect(_ observerdef.StorageReader, _ int64, _ observerdef.DetectionFilter) observerdef.DetectionResult {
	return observerdef.DetectionResult{}
}

// JSON toggles and partial hyperparameter overrides must use the same warmup
// as --only, while explicit values and the generic parser remain authoritative.
func TestParseTestbenchSettingsFromJSON(t *testing.T) {
	overrides := map[string]json.RawMessage{
		"bocpd":          json.RawMessage(`{"enabled":true,"hazard":0.08}`),
		"holt_residual":  json.RawMessage(`{"enabled":true,"alpha":0.4}`),
		"tukey_biweight": json.RawMessage(`{"enabled":true,"z_threshold":6}`),
		"scanmw":         json.RawMessage(`{"enabled":true,"max_points":96}`),
		"scanwelch":      json.RawMessage(`{"enabled":true}`),
		"anomaly_scorer": json.RawMessage(`{"enabled":true}`),
	}
	settings, err := ParseTestbenchSettingsFromJSON(overrides)
	require.NoError(t, err)
	bocpd := settings.configs["bocpd"].(BOCPDConfig)
	require.Equal(t, 40, bocpd.WarmupPoints)
	require.Equal(t, 0.08, bocpd.Hazard)
	holt := settings.configs["holt_residual"].(HoltResidualConfig)
	require.Equal(t, 15, holt.WarmupPoints)
	require.Equal(t, 25, holt.ResidualWindow)
	require.Equal(t, 0.4, holt.Alpha)
	tukey := settings.configs["tukey_biweight"].(TukeyBiweightConfig)
	require.Equal(t, 40, tukey.MinPoints)
	require.Equal(t, 40, tukey.WindowSize)
	require.Equal(t, 6.0, tukey.ZThreshold)
	require.Equal(t, 30, settings.configs["scanmw"].(ScanMWConfig).MinPoints)
	require.Equal(t, 96, settings.configs["scanmw"].(ScanMWConfig).MaxPoints)
	require.Equal(t, 30, settings.configs["scanwelch"].(ScanWelchConfig).MinPoints)
	scorer := settings.configs["anomaly_scorer"].(AnomalyScorerConfig)
	require.True(t, scorer.CorrelationEvents)
	require.Equal(t, DefaultAnomalyScorerConfig().CooldownSecs, scorer.CooldownSecs)

	production, err := ParseSettingsFromJSON(overrides)
	require.NoError(t, err)
	require.Equal(t, 60, production.configs["bocpd"].(BOCPDConfig).WarmupPoints)
	require.Equal(t, 24, production.configs["holt_residual"].(HoltResidualConfig).WarmupPoints)
	require.Equal(t, 60, production.configs["holt_residual"].(HoltResidualConfig).ResidualWindow)
	require.Equal(t, 80, production.configs["tukey_biweight"].(TukeyBiweightConfig).MinPoints)
	require.Equal(t, 30, production.configs["scanmw"].(ScanMWConfig).MinPoints)

	explicit, err := ParseTestbenchSettingsFromJSON(map[string]json.RawMessage{
		"bocpd":          json.RawMessage(`{"warmup_points":42}`),
		"holt_residual":  json.RawMessage(`{"warmup_points":6,"residual_window":18}`),
		"tukey_biweight": json.RawMessage(`{"min_points":20,"window_size":60}`),
		"scanmw":         json.RawMessage(`{"min_points":42}`),
		"scanwelch":      json.RawMessage(`{"min_points":36}`),
		"anomaly_scorer": json.RawMessage(`{"enabled":false,"cooldown_secs":0,"correlation_events":false}`),
	})
	require.NoError(t, err)
	require.Equal(t, 42, explicit.configs["bocpd"].(BOCPDConfig).WarmupPoints)
	require.Equal(t, 6, explicit.configs["holt_residual"].(HoltResidualConfig).WarmupPoints)
	require.Equal(t, 18, explicit.configs["holt_residual"].(HoltResidualConfig).ResidualWindow)
	require.Equal(t, 20, explicit.configs["tukey_biweight"].(TukeyBiweightConfig).MinPoints)
	require.Equal(t, 60, explicit.configs["tukey_biweight"].(TukeyBiweightConfig).WindowSize)
	require.Equal(t, 42, explicit.configs["scanmw"].(ScanMWConfig).MinPoints)
	require.Equal(t, 36, explicit.configs["scanwelch"].(ScanWelchConfig).MinPoints)
	require.False(t, explicit.Enabled["anomaly_scorer"])
	require.Zero(t, explicit.configs["anomaly_scorer"].(AnomalyScorerConfig).CooldownSecs)
	require.False(t, explicit.configs["anomaly_scorer"].(AnomalyScorerConfig).CorrelationEvents)
}

func instantiateForTest(t testing.TB, catalog *componentCatalog, settings ComponentSettings) (
	[]detectorRegistration,
	[]observerdef.Correlator,
	*anomalyScorer,
	[]observerdef.LogMetricsExtractor,
	map[string]*componentInstance,
) {
	detectors, correlators, scorer, extractors, components, err := catalog.Instantiate(settings)
	require.NoError(t, err)
	return detectors, correlators, scorer, extractors, components
}
