// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package providers

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestSetupConfigs(t *testing.T) {
	tests := []struct {
		name       string
		config     []*PrometheusCheck
		wantChecks []*PrometheusCheck
		wantErr    bool
	}{
		{
			name:   "empty config, default check",
			config: []*PrometheusCheck{},
			wantChecks: []*PrometheusCheck{
				{
					Instances: []*OpenmetricsInstance{
						{
							Metrics:   []string{"*"},
							Namespace: "",
						},
					},
					AD: &ADConfig{
						ExcludeAutoconf: boolPointer(true),
						KubeAnnotations: &InclExcl{
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
			config: []*PrometheusCheck{
				{
					Instances: []*OpenmetricsInstance{
						{
							Timeout: 20,
						},
					},
				},
			},
			wantChecks: []*PrometheusCheck{
				{
					Instances: []*OpenmetricsInstance{
						{
							Metrics:   []string{"*"},
							Namespace: "",
							Timeout:   20,
						},
					},
					AD: &ADConfig{
						ExcludeAutoconf: boolPointer(true),
						KubeAnnotations: &InclExcl{
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
			config: []*PrometheusCheck{
				{
					AD: &ADConfig{
						KubeAnnotations: &InclExcl{
							Excl: map[string]string{"custom/annotation": "exclude"},
						},
						KubeContainerNames: []string{"foo*"},
					},
				},
			},
			wantChecks: []*PrometheusCheck{
				{
					Instances: []*OpenmetricsInstance{
						{
							Metrics:   []string{"*"},
							Namespace: "",
						},
					},
					AD: &ADConfig{
						ExcludeAutoconf: boolPointer(true),
						KubeAnnotations: &InclExcl{
							Excl: map[string]string{"custom/annotation": "exclude"},
							Incl: map[string]string{"prometheus.io/scrape": "true"},
						},
						KubeContainerNames: []string{"foo*"},
						containersRe:       regexp.MustCompile("foo*"),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "custom instances and AD",
			config: []*PrometheusCheck{
				{
					Instances: []*OpenmetricsInstance{
						{
							Metrics:       []string{"foo", "bar"},
							Namespace:     "custom_ns",
							IgnoreMetrics: []string{"baz"},
						},
					},
					AD: &ADConfig{
						ExcludeAutoconf: boolPointer(false),
						KubeAnnotations: &InclExcl{
							Incl: map[string]string{"custom/annotation": "include"},
							Excl: map[string]string{"custom/annotation": "exclude"},
						},
						KubeContainerNames: []string{},
					},
				},
			},
			wantChecks: []*PrometheusCheck{
				{
					Instances: []*OpenmetricsInstance{
						{
							Metrics:       []string{"foo", "bar"},
							Namespace:     "custom_ns",
							IgnoreMetrics: []string{"baz"},
						},
					},
					AD: &ADConfig{
						ExcludeAutoconf: boolPointer(false),
						KubeAnnotations: &InclExcl{
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
			config: []*PrometheusCheck{
				{
					AD: &ADConfig{
						KubeContainerNames: []string{"*"},
					},
				},
			},
			wantChecks: nil,
			wantErr:    false,
		},
		{
			name: "two checks, one invalid check",
			config: []*PrometheusCheck{
				{
					AD: &ADConfig{
						KubeContainerNames: []string{"*"},
					},
				},
				{
					AD: &ADConfig{
						KubeContainerNames: []string{"foo", "bar"},
					},
				},
			},
			wantChecks: []*PrometheusCheck{
				{
					Instances: []*OpenmetricsInstance{
						{
							Metrics:   []string{"*"},
							Namespace: "",
						},
					},
					AD: &ADConfig{
						ExcludeAutoconf: boolPointer(true),
						KubeAnnotations: &InclExcl{
							Excl: map[string]string{"prometheus.io/scrape": "false"},
							Incl: map[string]string{"prometheus.io/scrape": "true"},
						},
						KubeContainerNames: []string{"foo", "bar"},
						containersRe:       regexp.MustCompile("foo|bar"),
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

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        string
	}{
		{
			name: "nominal case",
			annotations: map[string]string{
				"foo": "bar",
			},
			want: "http://%%host%%:%%port%%/metrics",
		},
		{
			name: "custom port",
			annotations: map[string]string{
				"foo":                "bar",
				"prometheus.io/port": "1337",
			},
			want: "http://%%host%%:1337/metrics",
		},
		{
			name: "custom path",
			annotations: map[string]string{
				"foo":                "bar",
				"prometheus.io/path": "/metrix",
			},
			want: "http://%%host%%:%%port%%/metrix",
		},
		{
			name: "custom port and path",
			annotations: map[string]string{
				"foo":                "bar",
				"prometheus.io/port": "1337",
				"prometheus.io/path": "/metrix",
			},
			want: "http://%%host%%:1337/metrix",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildURL(tt.annotations); got != tt.want {
				t.Errorf("buildURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainerRegex(t *testing.T) {
	ad := &ADConfig{}
	ad.setContainersRegex()
	assert.Nil(t, ad.containersRe)
	assert.True(t, ad.matchContainer("a-random-string"))

	ad = &ADConfig{KubeContainerNames: []string{"foo"}}
	ad.setContainersRegex()
	assert.NotNil(t, ad.containersRe)
	assert.True(t, ad.matchContainer("foo"))
	assert.False(t, ad.matchContainer("bar"))

	ad = &ADConfig{KubeContainerNames: []string{"foo", "b*"}}
	ad.setContainersRegex()
	assert.NotNil(t, ad.containersRe)
	assert.True(t, ad.matchContainer("foo"))
	assert.True(t, ad.matchContainer("bar"))
}
