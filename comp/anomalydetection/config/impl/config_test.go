// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package anomalydetectionconfigimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	compconfig "github.com/DataDog/datadog-agent/comp/core/config"
)

func TestEffectiveAnalysisEnabled(t *testing.T) {
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
			name: "enabled explicitly",
			yaml: `
anomaly_detection:
  enabled: true
`,
			want: true,
		},
		{
			name: "enabled implicitly by smart severity profiles",
			yaml: `
anomaly_detection:
  enabled: false
logs_config:
  experimental_adaptive_sampling:
    smart_severity_profiles:
      enabled: true
`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := compconfig.NewMockFromYAML(t, tt.yaml)
			assert.Equal(t, tt.want, EffectiveAnalysisEnabled(cfg))
		})
	}
}

func TestEffectiveAnomalyScorerEnabled(t *testing.T) {
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
			name: "explicit scorer flag enables scorer",
			yaml: `
anomaly_detection:
  anomaly_scorer:
    enabled: true
`,
			want: true,
		},
		{
			name: "smart severity profiles override disabled scorer flag",
			yaml: `
anomaly_detection:
  anomaly_scorer:
    enabled: false
logs_config:
  experimental_adaptive_sampling:
    smart_severity_profiles:
      enabled: true
`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := compconfig.NewMockFromYAML(t, tt.yaml)
			assert.Equal(t, tt.want, EffectiveAnomalyScorerEnabled(cfg))
		})
	}
}
