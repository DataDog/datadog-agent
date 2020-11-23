// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package providers

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common"
	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/stretchr/testify/assert"
)

func TestSetupConfigs(t *testing.T) {
	trueVar := true
	falseVar := false
	tests := []struct {
		name       string
		config     []*common.PrometheusCheck
		wantChecks []*common.PrometheusCheck
		wantErr    bool
	}{
		{
			name:   "empty config, default check",
			config: []*common.PrometheusCheck{},
			wantChecks: []*common.PrometheusCheck{
				{
					Instances: []*common.OpenmetricsInstance{
						{
							Metrics:   []string{"*"},
							Namespace: "",
						},
					},
					AD: &common.ADConfig{
						ExcludeAutoconf: &trueVar,
						KubeAnnotations: &common.InclExcl{
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
			config: []*common.PrometheusCheck{
				{
					Instances: []*common.OpenmetricsInstance{
						{
							Timeout: 20,
						},
					},
				},
			},
			wantChecks: []*common.PrometheusCheck{
				{
					Instances: []*common.OpenmetricsInstance{
						{
							Metrics:   []string{"*"},
							Namespace: "",
							Timeout:   20,
						},
					},
					AD: &common.ADConfig{
						ExcludeAutoconf: &trueVar,
						KubeAnnotations: &common.InclExcl{
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
			config: []*common.PrometheusCheck{
				{
					AD: &common.ADConfig{
						KubeAnnotations: &common.InclExcl{
							Excl: map[string]string{"custom/annotation": "exclude"},
						},
						KubeContainerNames: []string{"foo*"},
					},
				},
			},
			wantChecks: []*common.PrometheusCheck{
				{
					Instances: []*common.OpenmetricsInstance{
						{
							Metrics:   []string{"*"},
							Namespace: "",
						},
					},
					AD: &common.ADConfig{
						ExcludeAutoconf: &trueVar,
						KubeAnnotations: &common.InclExcl{
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
			config: []*common.PrometheusCheck{
				{
					Instances: []*common.OpenmetricsInstance{
						{
							Metrics:       []string{"foo", "bar"},
							Namespace:     "custom_ns",
							IgnoreMetrics: []string{"baz"},
						},
					},
					AD: &common.ADConfig{
						ExcludeAutoconf: &falseVar,
						KubeAnnotations: &common.InclExcl{
							Incl: map[string]string{"custom/annotation": "include"},
							Excl: map[string]string{"custom/annotation": "exclude"},
						},
						KubeContainerNames: []string{},
					},
				},
			},
			wantChecks: []*common.PrometheusCheck{
				{
					Instances: []*common.OpenmetricsInstance{
						{
							Metrics:       []string{"foo", "bar"},
							Namespace:     "custom_ns",
							IgnoreMetrics: []string{"baz"},
						},
					},
					AD: &common.ADConfig{
						ExcludeAutoconf: &falseVar,
						KubeAnnotations: &common.InclExcl{
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
			config: []*common.PrometheusCheck{
				{
					AD: &common.ADConfig{
						KubeContainerNames: []string{"*"},
					},
				},
			},
			wantChecks: nil,
			wantErr:    false,
		},
		{
			name: "two checks, one invalid check",
			config: []*common.PrometheusCheck{
				{
					AD: &common.ADConfig{
						KubeContainerNames: []string{"*"},
					},
				},
				{
					AD: &common.ADConfig{
						KubeContainerNames: []string{"foo", "bar"},
					},
				},
			},
			wantChecks: []*common.PrometheusCheck{
				{
					Instances: []*common.OpenmetricsInstance{
						{
							Metrics:   []string{"*"},
							Namespace: "",
						},
					},
					AD: &common.ADConfig{
						ExcludeAutoconf: &trueVar,
						KubeAnnotations: &common.InclExcl{
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
			p := &PrometheusConfigProvider{}
			config.Datadog.Set("prometheus_scrape.checks", tt.config)
			if err := p.setupConfigs(); (err != nil) != tt.wantErr {
				t.Errorf("PrometheusConfigProvider.setupConfigs() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.EqualValues(t, tt.wantChecks, p.checks)
		})
	}
}
