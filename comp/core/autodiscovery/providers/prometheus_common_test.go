// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/pkg/config/mock"

	"github.com/stretchr/testify/assert"
)

func TestGetPrometheusConfigs(t *testing.T) {
	tests := []struct {
		name       string
		config     []*types.PrometheusCheck
		wantChecks []*types.PrometheusCheck
		wantErr    bool
	}{
		{
			name:   "empty config, default check",
			config: []*types.PrometheusCheck{},
			wantChecks: []*types.PrometheusCheck{
				{
					Instances: []*types.OpenmetricsInstance{
						{
							Metrics:   []interface{}{"PLACEHOLDER"},
							Namespace: "",
						},
					},
					AD: &types.ADConfig{
						KubeAnnotations: &types.InclExcl{
							Excl: map[string]string{"prometheus.io/scrape": "false"},
							Incl: map[string]string{"prometheus.io/scrape": "true"},
						},
						KubeContainerNames: []string{},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "custom instance, set required fields",
			config: []*types.PrometheusCheck{
				{
					Instances: []*types.OpenmetricsInstance{
						{
							Timeout: 20,
						},
					},
				},
			},
			wantChecks: []*types.PrometheusCheck{
				{
					Instances: []*types.OpenmetricsInstance{
						{
							Metrics:   []interface{}{"*"},
							Namespace: "",
							Timeout:   20,
						},
					},
					AD: &types.ADConfig{
						KubeAnnotations: &types.InclExcl{
							Excl: map[string]string{"prometheus.io/scrape": "false"},
							Incl: map[string]string{"prometheus.io/scrape": "true"},
						},
						KubeContainerNames: []string{},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "custom AD, set required fields",
			config: []*types.PrometheusCheck{
				{
					AD: &types.ADConfig{
						KubeAnnotations: &types.InclExcl{
							Excl: map[string]string{"custom/annotation": "exclude"},
						},
						KubeContainerNames: []string{"foo*"},
					},
				},
			},
			wantChecks: []*types.PrometheusCheck{
				{
					Instances: []*types.OpenmetricsInstance{
						{
							Metrics:   []interface{}{"*"},
							Namespace: "",
						},
					},
					AD: &types.ADConfig{
						KubeAnnotations: &types.InclExcl{
							Excl: map[string]string{"custom/annotation": "exclude"},
							Incl: map[string]string{"prometheus.io/scrape": "true"},
						},
						KubeContainerNames: []string{"foo*"},
						ContainersRe:       regexp.MustCompile("foo*"),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "custom instances and AD",
			config: []*types.PrometheusCheck{
				{
					Instances: []*types.OpenmetricsInstance{
						{
							Metrics:       []interface{}{"foo", "bar"},
							Namespace:     "custom_ns",
							IgnoreMetrics: []string{"baz"},
						},
					},
					AD: &types.ADConfig{
						KubeAnnotations: &types.InclExcl{
							Incl: map[string]string{"custom/annotation": "include"},
							Excl: map[string]string{"custom/annotation": "exclude"},
						},
						KubeContainerNames: []string{},
					},
				},
			},
			wantChecks: []*types.PrometheusCheck{
				{
					Instances: []*types.OpenmetricsInstance{
						{
							Metrics:       []interface{}{"foo", "bar"},
							Namespace:     "custom_ns",
							IgnoreMetrics: []string{"baz"},
						},
					},
					AD: &types.ADConfig{
						KubeAnnotations: &types.InclExcl{
							Incl: map[string]string{"custom/annotation": "include"},
							Excl: map[string]string{"custom/annotation": "exclude"},
						},
						KubeContainerNames: []string{},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid check",
			config: []*types.PrometheusCheck{
				{
					AD: &types.ADConfig{
						KubeContainerNames: []string{"*"},
					},
				},
			},
			wantChecks: []*types.PrometheusCheck{},
			wantErr:    false,
		},
		{
			name: "two checks, one invalid check",
			config: []*types.PrometheusCheck{
				{
					AD: &types.ADConfig{
						KubeContainerNames: []string{"*"},
					},
				},
				{
					AD: &types.ADConfig{
						KubeContainerNames: []string{"foo", "bar"},
					},
				},
			},
			wantChecks: []*types.PrometheusCheck{
				{
					Instances: []*types.OpenmetricsInstance{
						{
							Metrics:   []interface{}{"*"},
							Namespace: "",
						},
					},
					AD: &types.ADConfig{
						KubeAnnotations: &types.InclExcl{
							Excl: map[string]string{"prometheus.io/scrape": "false"},
							Incl: map[string]string{"prometheus.io/scrape": "true"},
						},
						KubeContainerNames: []string{"foo", "bar"},
						ContainersRe:       regexp.MustCompile("foo|bar"),
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mock.New(t)
			confBytes, _ := json.Marshal(tt.config)
			cfg.SetWithoutSource("prometheus_scrape.checks", string(confBytes))
			checks, err := getPrometheusConfigs()
			if (err != nil) != tt.wantErr {
				t.Errorf("getPrometheusConfigs() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.EqualValues(t, tt.wantChecks, checks)
		})
	}
}

func TestPromAnnotationsDiffer(t *testing.T) {
	tests := []struct {
		name   string
		checks []*types.PrometheusCheck
		first  map[string]string
		second map[string]string
		want   bool
	}{
		{
			name:   "scrape annotation changed",
			checks: []*types.PrometheusCheck{types.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/scrape": "true"},
			second: map[string]string{"prometheus.io/scrape": "false"},
			want:   true,
		},
		{
			name:   "scrape annotation unchanged",
			checks: []*types.PrometheusCheck{types.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/scrape": "true"},
			second: map[string]string{"prometheus.io/scrape": "true"},
			want:   false,
		},
		{
			name:   "scrape annotation removed",
			checks: []*types.PrometheusCheck{types.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/scrape": "true"},
			second: map[string]string{"foo": "bar"},
			want:   true,
		},
		{
			name:   "path annotation changed",
			checks: []*types.PrometheusCheck{types.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/path": "/metrics"},
			second: map[string]string{"prometheus.io/path": "/metrics_custom"},
			want:   true,
		},
		{
			name:   "path annotation unchanged",
			checks: []*types.PrometheusCheck{types.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/path": "/metrics"},
			second: map[string]string{"prometheus.io/path": "/metrics"},
			want:   false,
		},
		{
			name:   "port annotation changed",
			checks: []*types.PrometheusCheck{types.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/port": "1234"},
			second: map[string]string{"prometheus.io/port": "4321"},
			want:   true,
		},
		{
			name:   "port annotation unchanged",
			checks: []*types.PrometheusCheck{types.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/port": "1234"},
			second: map[string]string{"prometheus.io/port": "1234"},
			want:   false,
		},
		{
			name:   "include annotation changed",
			checks: []*types.PrometheusCheck{{AD: &types.ADConfig{KubeAnnotations: &types.InclExcl{Incl: map[string]string{"include": "true"}}}}},
			first:  map[string]string{"include": "true"},
			second: map[string]string{"include": "foo"},
			want:   true,
		},
		{
			name:   "include annotation unchanged",
			checks: []*types.PrometheusCheck{{AD: &types.ADConfig{KubeAnnotations: &types.InclExcl{Incl: map[string]string{"include": "true"}}}}},
			first:  map[string]string{"include": "true"},
			second: map[string]string{"include": "true"},
			want:   false,
		},
		{
			name:   "exclude annotation changed",
			checks: []*types.PrometheusCheck{{AD: &types.ADConfig{KubeAnnotations: &types.InclExcl{Excl: map[string]string{"exclude": "true"}}}}},
			first:  map[string]string{"exclude": "true"},
			second: map[string]string{"exclude": "foo"},
			want:   true,
		},
		{
			name:   "exclude annotation unchanged",
			checks: []*types.PrometheusCheck{{AD: &types.ADConfig{KubeAnnotations: &types.InclExcl{Excl: map[string]string{"exclude": "true"}}}}},
			first:  map[string]string{"exclude": "true"},
			second: map[string]string{"exclude": "true"},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, promAnnotationsDiffer(tt.checks, tt.first, tt.second))
		})
	}
}
