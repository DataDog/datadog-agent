// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"strings"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/require"
)

type testLifecycle struct {
	hooks []compdef.Hook
}

func TestStorageConfigFromAgentConfigDerivesRetentionFromDetectorWindows(t *testing.T) {
	detectors := []observerdef.Detector{NewBOCPDDetector(DefaultBOCPDConfig())}

	for name, test := range map[string]struct {
		yaml string
		want int64
	}{
		"unset":      {want: 1816},
		"zero":       {yaml: "anomaly_detection:\n  storage:\n    point_retention: 0s\n", want: 1816},
		"short":      {yaml: "anomaly_detection:\n  storage:\n    point_retention: 30s\n", want: 1816},
		"sufficient": {yaml: "anomaly_detection:\n  storage:\n    point_retention: 3200s\n", want: 3200},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := configmock.NewFromYAML(t, test.yaml)
			storageCfg := storageConfigFromAgentConfig(cfg, detectors)
			require.Equal(t, test.want, storageCfg.PointRetentionSecs)
			require.Equal(t, 120, storageCfg.MaxPointsPerSeries)
		})
	}
}

func TestMaxDetectorPoints(t *testing.T) {
	bocpdConfig := DefaultBOCPDConfig()
	bocpdConfig.WarmupPoints = 40
	detectors := []observerdef.Detector{
		NewBOCPDDetector(bocpdConfig),
		NewHoltResidualDetector(),
		NewTukeyBiweightDetector(),
		NewScanMWDetector(),
		NewScanWelchDetector(),
	}
	require.Equal(t, 120, maxDetectorPoints(detectors))
}

func (l *testLifecycle) Append(h compdef.Hook) {
	l.hooks = append(l.hooks, h)
}

func requireNoObserverMetricFamilies(t *testing.T, telemetryComp telemetry.Component) {
	t.Helper()

	metricFamilies, err := telemetryComp.Gather(false)
	require.NoError(t, err)

	for _, family := range metricFamilies {
		if strings.HasPrefix(family.GetName(), "observer__") {
			t.Fatalf("unexpected observer metric family initialized: %s", family.GetName())
		}
	}
}

func TestNewComponentReturnsErrorForInvalidMetricProcessingRulesConfig(t *testing.T) {
	testCases := []struct {
		name        string
		yaml        string
		errContains string
	}{
		{
			name: "invalid rule type",
			yaml: `
anomaly_detection:
  reporting:
    events:
      enabled: true
  metrics:
    enabled: true
    processing_rules:
      - type: invalid_type
        name: bad_rule
`,
			errContains: `anomaly_detection.metrics.processing_rules: rule "bad_rule": unsupported type "invalid_type"`,
		},
		{
			name: "invalid name pattern",
			yaml: `
anomaly_detection:
  reporting:
    events:
      enabled: true
  metrics:
    enabled: true
    processing_rules:
      - type: exclude_at_match
        name: bad_pattern
        name_pattern: kubernetes.*.cpu
`,
			errContains: "anomaly_detection.metrics.processing_rules: rule \"bad_pattern\": name_pattern must be a prefix with an optional trailing *",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.NewFromYAML(t, tc.yaml)
			lc := &testLifecycle{}
			telComp := telemetryimpl.NewMock(t)

			_, err := NewComponent(Requires{
				Lifecycle: lc,
				Config:    cfg,
				Telemetry: telComp,
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errContains)
		})
	}
}

func TestNewComponentWithAnalysisDisabledUsesNoopHandleAndDoesNotInitializeObserverMetrics(t *testing.T) {
	cfg := configmock.NewFromYAML(t, `
anomaly_detection:
  metrics:
    enabled: true
    processing_rules:
      - type: exclude_at_match
        name: drop_dogstatsd
        source: dogstatsd
`)
	lc := &testLifecycle{}
	telComp := telemetryimpl.NewMock(t)

	provides, err := NewComponent(Requires{
		Lifecycle: lc,
		Config:    cfg,
		Telemetry: telComp,
	})
	require.NoError(t, err)

	handle := provides.Comp.GetHandle("dogstatsd")
	_, ok := handle.(*noopObserveHandle)
	require.Truef(t, ok, `GetHandle("dogstatsd") returned %T, want *noopObserveHandle`, handle)

	requireNoObserverMetricFamilies(t, telComp)
}

func TestNewComponentReadsInactiveSeriesEvictionStorageConfig(t *testing.T) {
	tt := []struct {
		name          string
		storageConfig string
		wantTTL       int64
		wantInterval  int64
	}{
		{
			name: "configured",
			storageConfig: `
    inactive_series_ttl: 30m
    inactive_series_check_interval: 10m`,
			wantTTL:      30 * 60,
			wantInterval: 10 * 60,
		},
		{
			name: "disabled with zero",
			storageConfig: `
    inactive_series_ttl: 0s
    inactive_series_check_interval: 0s`,
			wantTTL:      0,
			wantInterval: 0,
		},
		{
			name: "negative values retain defaults",
			storageConfig: `
    inactive_series_ttl: -1s
    inactive_series_check_interval: -1s`,
			wantTTL:      storageInactiveSeriesTTLSeconds,
			wantInterval: storageInactiveSeriesCheckIntervalSeconds,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.NewFromYAML(t, `
anomaly_detection:
  reporting:
    events:
      enabled: true
  storage:
`+tc.storageConfig)

			provides, err := NewComponent(Requires{
				Lifecycle: &testLifecycle{},
				Config:    cfg,
				Telemetry: telemetryimpl.NewMock(t),
			})
			require.NoError(t, err)
			obs, ok := provides.Comp.(*observerImpl)
			require.True(t, ok)
			require.Equal(t, tc.wantTTL, obs.engine.storage.cfg.InactiveSeriesTTLSeconds)
			require.Equal(t, tc.wantInterval, obs.engine.storage.cfg.InactiveSeriesCheckIntervalSeconds)
		})
	}
}
