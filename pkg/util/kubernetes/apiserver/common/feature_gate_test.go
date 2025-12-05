// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFeatureGatesFromMetrics(t *testing.T) {
	tests := []struct {
		name        string
		metricsData string
		want        map[string]FeatureGate
		wantErr     bool
	}{
		{
			name: "single feature gate",
			metricsData: `# HELP kubernetes_feature_enabled [ALPHA] Feature gate status
# TYPE kubernetes_feature_enabled gauge
kubernetes_feature_enabled{name="SomeFeature",stage="ALPHA"} 1
`,
			want: map[string]FeatureGate{
				"SomeFeature": {
					Name:    "SomeFeature",
					Stage:   "ALPHA",
					Enabled: true,
				},
			},
			wantErr: false,
		},
		{
			name: "multiple feature gates",
			metricsData: `# HELP kubernetes_feature_enabled [ALPHA] Feature gate status
# TYPE kubernetes_feature_enabled gauge
kubernetes_feature_enabled{name="FeatureA",stage="ALPHA"} 1
kubernetes_feature_enabled{name="FeatureB",stage="BETA"} 0
kubernetes_feature_enabled{name="FeatureC",stage="GA"} 1
`,
			want: map[string]FeatureGate{
				"FeatureA": {
					Name:    "FeatureA",
					Stage:   "ALPHA",
					Enabled: true,
				},
				"FeatureB": {
					Name:    "FeatureB",
					Stage:   "BETA",
					Enabled: false,
				},
				"FeatureC": {
					Name:    "FeatureC",
					Stage:   "GA",
					Enabled: true,
				},
			},
			wantErr: false,
		},
		{
			name: "empty stage",
			metricsData: `kubernetes_feature_enabled{name="NoStageFeature",stage=""} 1
`,
			want: map[string]FeatureGate{
				"NoStageFeature": {
					Name:    "NoStageFeature",
					Stage:   "",
					Enabled: true,
				},
			},
			wantErr: false,
		},
		{
			name: "mixed with comments and other metrics",
			metricsData: `# HELP some_other_metric Some other metric
# TYPE some_other_metric gauge
some_other_metric 42
# HELP kubernetes_feature_enabled [ALPHA] Feature gate status
# TYPE kubernetes_feature_enabled gauge
kubernetes_feature_enabled{name="RealFeature",stage="ALPHA"} 1
other_metric{label="value"} 123
`,
			want: map[string]FeatureGate{
				"RealFeature": {
					Name:    "RealFeature",
					Stage:   "ALPHA",
					Enabled: true,
				},
			},
			wantErr: false,
		},
		{
			name:        "empty metrics",
			metricsData: ``,
			want:        map[string]FeatureGate{},
			wantErr:     false,
		},
		{
			name: "only comments",
			metricsData: `# HELP kubernetes_feature_enabled [ALPHA] Feature gate status
# TYPE kubernetes_feature_enabled gauge
`,
			want:    map[string]FeatureGate{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFeatureGatesFromMetrics([]byte(tt.metricsData))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
