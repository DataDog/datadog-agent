// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubelet

package providers

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"

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
			p := &PrometheusPodsConfigProvider{}
			config.Datadog.Set("prometheus_scrape.checks", tt.config)
			if err := p.setupConfigs(); (err != nil) != tt.wantErr {
				t.Errorf("PrometheusPodsConfigProvider.setupConfigs() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.EqualValues(t, tt.wantChecks, p.checks)
		})
	}
}

func TestConfigsForPod(t *testing.T) {
	tests := []struct {
		name    string
		check   *PrometheusCheck
		pod     *kubelet.Pod
		want    []integration.Config
		matched bool
	}{
		{
			name:  "nominal case",
			check: defaultCheck,
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:        "foo-pod",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr",
							ID:   "foo-ctr-id",
						},
					},
					AllContainers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr",
							ID:   "foo-ctr-id",
						},
					},
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"prometheus_url\":\"http://%%host%%:%%port%%/metrics\",\"namespace\":\"\",\"metrics\":[\"*\"]}")},
					Provider:      names.Prometheus,
					Source:        "kubelet:foo-ctr-id",
					ADIdentifiers: []string{"foo-ctr-id"},
				},
			},
		},
		{
			name: "custom prometheus_url",
			check: &PrometheusCheck{
				Instances: []*OpenmetricsInstance{
					{
						URL:       "foo/bar",
						Metrics:   []string{"*"},
						Namespace: "",
					},
				},
			},
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:        "foo-pod",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr",
							ID:   "foo-ctr-id",
						},
					},
					AllContainers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr",
							ID:   "foo-ctr-id",
						},
					},
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"prometheus_url\":\"foo/bar\",\"namespace\":\"\",\"metrics\":[\"*\"]}")},
					Provider:      names.Prometheus,
					Source:        "kubelet:foo-ctr-id",
					ADIdentifiers: []string{"foo-ctr-id"},
				},
			},
		},
		{
			name:  "excluded",
			check: defaultCheck,
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:        "foo-pod",
					Annotations: map[string]string{"prometheus.io/scrape": "false"},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr",
							ID:   "foo-ctr-id",
						},
					},
					AllContainers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr",
							ID:   "foo-ctr-id",
						},
					},
				},
			},
			want: nil,
		},
		{
			name:  "no match",
			check: defaultCheck,
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:        "foo-pod",
					Annotations: map[string]string{"foo": "bar"},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr",
							ID:   "foo-ctr-id",
						},
					},
					AllContainers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr",
							ID:   "foo-ctr-id",
						},
					},
				},
			},
			want: nil,
		},
		{
			name:  "multi containers, match all",
			check: defaultCheck,
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:        "foo-pod",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr1",
							ID:   "foo-ctr1-id",
						},
						{
							Name: "foo-ctr2",
							ID:   "foo-ctr2-id",
						},
					},
					AllContainers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr1",
							ID:   "foo-ctr1-id",
						},
						{
							Name: "foo-ctr2",
							ID:   "foo-ctr2-id",
						},
					},
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"prometheus_url\":\"http://%%host%%:%%port%%/metrics\",\"namespace\":\"\",\"metrics\":[\"*\"]}")},
					Provider:      names.Prometheus,
					Source:        "kubelet:foo-ctr1-id",
					ADIdentifiers: []string{"foo-ctr1-id"},
				},
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"prometheus_url\":\"http://%%host%%:%%port%%/metrics\",\"namespace\":\"\",\"metrics\":[\"*\"]}")},
					Provider:      names.Prometheus,
					Source:        "kubelet:foo-ctr2-id",
					ADIdentifiers: []string{"foo-ctr2-id"},
				},
			},
		},
		{
			name: "multi containers, match one container",
			check: &PrometheusCheck{
				AD: &ADConfig{
					KubeContainerNames: []string{"foo-ctr1"},
				},
			},
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:        "foo-pod",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr1",
							ID:   "foo-ctr1-id",
						},
						{
							Name: "foo-ctr2",
							ID:   "foo-ctr2-id",
						},
					},
					AllContainers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr1",
							ID:   "foo-ctr1-id",
						},
						{
							Name: "foo-ctr2",
							ID:   "foo-ctr2-id",
						},
					},
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"prometheus_url\":\"http://%%host%%:%%port%%/metrics\",\"namespace\":\"\",\"metrics\":[\"*\"]}")},
					Provider:      names.Prometheus,
					Source:        "kubelet:foo-ctr1-id",
					ADIdentifiers: []string{"foo-ctr1-id"},
				},
			},
		},
		{
			name: "container name mismatch",
			check: &PrometheusCheck{
				AD: &ADConfig{
					KubeContainerNames: []string{"bar"},
				},
			},
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:        "foo-pod",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr",
							ID:   "foo-ctr-id",
						},
					},
					AllContainers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr",
							ID:   "foo-ctr-id",
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "container name regex",
			check: &PrometheusCheck{
				AD: &ADConfig{
					KubeContainerNames: []string{"bar", "*o-c*"},
				},
			},
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:        "foo-pod",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr",
							ID:   "foo-ctr-id",
						},
					},
					AllContainers: []kubelet.ContainerStatus{
						{
							Name: "foo-ctr",
							ID:   "foo-ctr-id",
						},
					},
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"prometheus_url\":\"http://%%host%%:%%port%%/metrics\",\"namespace\":\"\",\"metrics\":[\"*\"]}")},
					Provider:      names.Prometheus,
					Source:        "kubelet:foo-ctr-id",
					ADIdentifiers: []string{"foo-ctr-id"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check.init()
			configs := tt.check.configsForPod(tt.pod)
			assert.ElementsMatch(t, configs, tt.want)
		})
	}
}
