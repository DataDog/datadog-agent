// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/require"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// TestNewComponentReturnsDisabledStubWhenOff verifies the off-by-default fast
// path: with anomaly_detection.enabled=false and no recorder, NewComponent must
// return the zero-allocation disabledObserver stub rather than building the
// engine, storage, catalog, 1000-cap channel, and dispatch goroutine.
func TestNewComponentReturnsDisabledStubWhenOff(t *testing.T) {
	// anomaly_detection.enabled defaults to false; the mock carries that default.
	cfg := configmock.New(t)

	provides, err := NewComponent(Requires{
		Config:   cfg,
		Recorder: option.None[recorderdef.Component](),
	})
	require.NoError(t, err)

	_, ok := provides.Comp.(*disabledObserver)
	require.Truef(t, ok, "expected *disabledObserver when anomaly detection is disabled, got %T", provides.Comp)
}

func TestNewComponentSmartSeverityProfilesForceEnableAnalysisAndScorer(t *testing.T) {
	cfg := configmock.NewFromYAML(t, `
anomaly_detection:
  enabled: false
  anomaly_scorer:
    enabled: false
  logs:
    internal:
      enabled: false
logs_config:
  experimental_adaptive_sampling:
    smart_severity_profiles:
      enabled: true
`)

	lc := &testLifecycle{}
	provides, err := NewComponent(Requires{
		Lifecycle: lc,
		Config:    cfg,
		Log:       &noopLogComponent{},
		Recorder:  option.None[recorderdef.Component](),
	})
	require.NoError(t, err)

	_, disabled := provides.Comp.(*disabledObserver)
	require.False(t, disabled, "smart severity profiles should force-enable anomaly detection")

	sub, err := provides.Comp.SubscribeSeverityEventsReader(severityeventsdef.SeverityEventsConfiguration{})
	require.NoError(t, err, "smart severity profiles should force-enable the anomaly scorer")
	if sub.Unsubscribe != nil {
		sub.Unsubscribe()
	}
}

// TestWarnSmartSeverityOverrides verifies the warning only fires when the
// user explicitly disabled anomaly_detection.enabled and smart severity
// profiles then force-enable it. Leaving the gate unset must not warn, since
// false is already its default.
func TestWarnSmartSeverityOverrides(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantAnyLog bool
	}{
		{
			name: "unset gate, smart severity enabled: no warning",
			yaml: `
logs_config:
  experimental_adaptive_sampling:
    smart_severity_profiles:
      enabled: true
`,
			wantAnyLog: false,
		},
		{
			name: "explicit anomaly_detection.enabled=false: warns",
			yaml: `
anomaly_detection:
  enabled: false
logs_config:
  experimental_adaptive_sampling:
    smart_severity_profiles:
      enabled: true
`,
			wantAnyLog: true,
		},
		{
			name: "explicit anomaly_scorer.enabled=false only: no warning",
			yaml: `
anomaly_detection:
  anomaly_scorer:
    enabled: false
logs_config:
  experimental_adaptive_sampling:
    smart_severity_profiles:
      enabled: true
`,
			wantAnyLog: false,
		},
		{
			name: "explicit anomaly_detection.enabled=true: no warning",
			yaml: `
anomaly_detection:
  enabled: true
logs_config:
  experimental_adaptive_sampling:
    smart_severity_profiles:
      enabled: true
`,
			wantAnyLog: false,
		},
		{
			name: "smart severity profiles disabled: no warning regardless of gates",
			yaml: `
anomaly_detection:
  enabled: false
  anomaly_scorer:
    enabled: false
logs_config:
  experimental_adaptive_sampling:
    smart_severity_profiles:
      enabled: false
`,
			wantAnyLog: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.NewFromYAML(t, tt.yaml)
			rec := &recordingLogComponent{}

			warnSmartSeverityOverrides(cfg, rec)

			if tt.wantAnyLog {
				require.NotEmpty(t, rec.warns, "expected a warning log")
			} else {
				require.Empty(t, rec.warns, "expected no warning log")
			}
		})
	}
}
