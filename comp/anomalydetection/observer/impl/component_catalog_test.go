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
	require.Equal(t, []string{"scanmw"}, detectorNames(detectors))

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
			wantDetectors: []string{"scanmw"},
		},
		{
			name:          "pipeline ignores enabled legacy detectors",
			pipeline:      true,
			enabled:       map[string]bool{"bocpd": true, "holt_residual": true, "tukey_biweight": true},
			wantDetectors: []string{"scanmw"},
		},
		{
			name:          "pipeline ignores scanmw disable",
			pipeline:      true,
			enabled:       map[string]bool{"scanmw": false},
			wantDetectors: []string{"scanmw"},
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
			if len(mode.wantDetectors) > 0 && slices.Contains(mode.wantDetectors, "scanmw") {
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
  use_legacy_detectors: true
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
  use_legacy_detectors: false
`)
	settings = settingsFromAgentConfig(defaultCatalog(), cfg)
	require.True(t, settings.UseMetricPipeline)
	detectors, _, _, _, _ := instantiateForTest(t, defaultCatalog(), settings)
	require.Equal(t, []string{"scanmw"}, detectorNames(detectors))
	pipeline := detectors[0].detector.(*MetricPipeline)
	require.Equal(t, observerdef.DetectorPointWindow{MinPoints: 30, MaxPoints: 120}, pipeline.DetectorPointWindow())
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
	holt := settings.configs["holt_residual"].(holtResidualCatalogConfig)
	require.Equal(t, 15, holt.WarmupPoints)
	require.Equal(t, 25, holt.ResidualWindow)
	tukey := settings.configs["tukey_biweight"].(tukeyBiweightCatalogConfig)
	require.Equal(t, 40, tukey.WindowSize)
	require.Equal(t, 40, tukey.MinPoints)
	require.True(t, settings.Enabled["anomaly_scorer"])
	require.NotContains(t, settings.Enabled, "time_cluster")
	scorer := settings.configs["anomaly_scorer"].(AnomalyScorerConfig)
	require.True(t, scorer.CorrelationEvents)
	require.Equal(t, DefaultAnomalyScorerConfig().CooldownSecs, scorer.CooldownSecs)
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

func TestCatalogAggregationFilters(t *testing.T) {
	for _, name := range []string{"holt_residual", "tukey_biweight"} {
		for _, tc := range []struct {
			name string
			json string
			want []observerdef.Aggregate
		}{
			{"omitted", `{}`, defaultAggregations},
			{"empty", `{"aggregations":[]}`, nil},
			{"null", `{"aggregations":null}`, nil},
			{"unknown", `{"aggregations":["invalid"]}`, nil},
			{"sum", `{"aggregations":["sum"]}`, []observerdef.Aggregate{observerdef.AggregateSum}},
			{"ordered", `{"aggregations":["count","avg"]}`, []observerdef.Aggregate{observerdef.AggregateCount, observerdef.AggregateAverage}},
			{"mixed", `{"aggregations":["invalid","sum","avg"]}`, []observerdef.Aggregate{observerdef.AggregateSum, observerdef.AggregateAverage}},
			{"duplicates", `{"aggregations":["sum","sum"]}`, []observerdef.Aggregate{observerdef.AggregateSum, observerdef.AggregateSum}},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				settings, err := ParseSettingsFromJSON(map[string]json.RawMessage{name: json.RawMessage(tc.json)})
				require.NoError(t, err)
				settings.Enabled = map[string]bool{"bocpd": false, name: true}
				settings = ApplyTestbenchDefaults(settings)
				detectors, _, _, _, components := instantiateForTest(t, defaultCatalog(), settings)
				raw, err := json.Marshal(components[name].activeConfig)
				require.NoError(t, err)
				var flat map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(raw, &flat))
				require.NotContains(t, flat, "HoltResidualConfig")
				require.NotContains(t, flat, "TukeyBiweightConfig")
				roundtrip, err := ParseSettingsFromJSON(map[string]json.RawMessage{name: raw})
				require.NoError(t, err)
				roundtrip.Enabled = settings.Enabled
				decoded, _, _, _, _ := instantiateForTest(t, defaultCatalog(), roundtrip)
				wantValues, wantCount := detectorAggregations(detectors[0].filter.Aggregations)
				gotValues, gotCount := detectorAggregations(decoded[0].filter.Aggregations)
				require.Equal(t, wantValues, gotValues)
				require.Equal(t, wantCount, gotCount)
				require.Len(t, detectors, 1)
				require.Equal(t, tc.want, append([]observerdef.Aggregate(nil), detectors[0].filter.Aggregations...))
				storage := newDetectorTestStorage()
				for ts := int64(1); ts <= 120; ts++ {
					storage.Add("work", "metric", 100, ts, nil)
				}
				e := newEngine(engineConfig{storage: storage, detectors: detectors})
				check := func() {
					var got []observerdef.Aggregate
					for key := range filterTestState(detectors[0].detector) {
						got = append(got, key.Aggregate)
					}
					want, n := detectorAggregations(tc.want)
					require.ElementsMatch(t, want[:n], got)
				}
				e.Advance(120)
				check()
				e.Reset()
				e.Advance(120)
				check()
				e.SetDetectors(detectors)
				e.ReplayStoredData()
				check()
				settings.Enabled[name] = false
				disabled, _, _, _, _ := instantiateForTest(t, defaultCatalog(), settings)
				require.Empty(t, disabled)
				e.SetDetectors(disabled)
				settings.Enabled[name] = true
				replacement, _, _, _, _ := instantiateForTest(t, defaultCatalog(), settings)
				require.Equal(t, detectors[0].filter, replacement[0].filter)
				require.NotSame(t, detectors[0].detector, replacement[0].detector)
				e.ResetForReplay(replacement, nil, nil, nil, DefaultStorageConfig(), BaselineConfig{})
				detectors = replacement
				for ts := int64(1); ts <= 120; ts++ {
					e.Storage().Add("work", "metric", 100, ts, nil)
				}
				e.ReplayStoredData()
				check()
			})
		}
	}
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
