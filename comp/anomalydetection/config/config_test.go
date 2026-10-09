// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	compconfig "github.com/DataDog/datadog-agent/comp/core/config"
)

func TestObserverRequired(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{
			name: "disabled by default",
			yaml: ``,
			want: false,
		},
		{
			name: "enabled by smart severity profiles",
			yaml: `
logs_config:
  experimental_adaptive_sampling:
    smart_severity_profiles:
      enabled: true
`,
			want: true,
		},
		{
			name: "enabled by reporting events",
			yaml: `
anomaly_detection:
  reporting:
    events:
      enabled: true
`,
			want: true,
		},
		{
			name: "enabled by scorer dry run",
			yaml: `
anomaly_detection:
  anomaly_scorer:
    dry_run:
      enabled: true
`,
			want: true,
		},
		{
			name: "enabled by recording",
			yaml: `
anomaly_detection:
  recording:
    enabled: true
`,
			want: true,
		},
		{
			name: "stdout alone does not enable observer",
			yaml: `
anomaly_detection:
  reporting:
    stdout:
      enabled: true
`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := compconfig.NewMockFromYAML(t, tt.yaml)
			assert.Equal(t, tt.want, ObserverRequired(cfg))
		})
	}
}

func TestScorerRequired(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{
			name: "disabled by default",
			yaml: ``,
			want: false,
		},
		{
			name: "enabled by smart severity profiles",
			yaml: `
logs_config:
  experimental_adaptive_sampling:
    smart_severity_profiles:
      enabled: true
`,
			want: true,
		},
		{
			name: "enabled by reporting events",
			yaml: `
anomaly_detection:
  reporting:
    events:
      enabled: true
`,
			want: true,
		},
		{
			name: "enabled by scorer dry run",
			yaml: `
anomaly_detection:
  anomaly_scorer:
    dry_run:
      enabled: true
`,
			want: true,
		},
		{
			name: "recording alone does not require scorer",
			yaml: `
anomaly_detection:
  recording:
    enabled: true
`,
			want: false,
		},
		{
			name: "stdout alone does not require scorer",
			yaml: `
anomaly_detection:
  reporting:
    stdout:
      enabled: true
`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := compconfig.NewMockFromYAML(t, tt.yaml)
			assert.Equal(t, tt.want, ScorerRequired(cfg))
		})
	}
}

func TestAnomalyEventsDefaults(t *testing.T) {
	cfg := compconfig.NewMockFromYAML(t, ``)
	got := AnomalyEvents(cfg)
	assert.False(t, got.Enabled)
	assert.Equal(t, DefaultAnomalyEventsEndpoint, got.Endpoint)
	assert.Equal(t, DefaultAnomalyEventsRetryInterval, got.RetryInterval)
}

func TestAnomalyEventsOverrides(t *testing.T) {
	cfg := compconfig.NewMockFromYAML(t, `
anomaly_detection:
  anomaly_events:
    enabled: true
    endpoint: unix:/tmp/events.sock
    connect_retry_interval: 30s
`)
	got := AnomalyEvents(cfg)
	assert.True(t, got.Enabled)
	assert.True(t, AnomalyEventsEnabled(cfg))
	assert.Equal(t, "unix:/tmp/events.sock", got.Endpoint)
	assert.Equal(t, 30*time.Second, got.RetryInterval)
}

func TestAnomalyEventsFallsBackOnEmptyOrInvalidValues(t *testing.T) {
	cfg := compconfig.NewMockFromYAML(t, `
anomaly_detection:
  anomaly_events:
    enabled: true
    endpoint: ""
    connect_retry_interval: 0s
`)
	got := AnomalyEvents(cfg)
	assert.Equal(t, DefaultAnomalyEventsEndpoint, got.Endpoint)
	assert.Equal(t, DefaultAnomalyEventsRetryInterval, got.RetryInterval)
}
