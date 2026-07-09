// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"strings"
	"testing"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/require"
)

type testLifecycle struct {
	hooks []compdef.Hook
}

func (l *testLifecycle) Append(h compdef.Hook) {
	l.hooks = append(l.hooks, h)
}

type noopLogComponent struct{}

func (noopLogComponent) Trace(...interface{})                   {}
func (noopLogComponent) Tracef(string, ...interface{})          {}
func (noopLogComponent) Debug(...interface{})                   {}
func (noopLogComponent) Debugf(string, ...interface{})          {}
func (noopLogComponent) Info(...interface{})                    {}
func (noopLogComponent) Infof(string, ...interface{})           {}
func (noopLogComponent) Warn(...interface{}) error              { return nil }
func (noopLogComponent) Warnf(string, ...interface{}) error     { return nil }
func (noopLogComponent) Error(...interface{}) error             { return nil }
func (noopLogComponent) Errorf(string, ...interface{}) error    { return nil }
func (noopLogComponent) Critical(...interface{}) error          { return nil }
func (noopLogComponent) Criticalf(string, ...interface{}) error { return nil }
func (noopLogComponent) Flush()                                 {}

var _ log.Component = noopLogComponent{}

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
			telComp := telemetryimpl.GetCompatComponent()
			telComp.Reset()
			t.Cleanup(telComp.Reset)

			_, err := NewComponent(Requires{
				Lifecycle: lc,
				Config:    cfg,
				Log:       noopLogComponent{},
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
	telComp := telemetryimpl.GetCompatComponent()
	telComp.Reset()
	t.Cleanup(telComp.Reset)

	provides, err := NewComponent(Requires{
		Lifecycle: lc,
		Config:    cfg,
		Log:       noopLogComponent{},
		Telemetry: telComp,
	})
	require.NoError(t, err)

	handle := provides.Comp.GetHandle("dogstatsd")
	_, ok := handle.(*noopObserveHandle)
	require.Truef(t, ok, `GetHandle("dogstatsd") returned %T, want *noopObserveHandle`, handle)

	requireNoObserverMetricFamilies(t, telComp)
}
