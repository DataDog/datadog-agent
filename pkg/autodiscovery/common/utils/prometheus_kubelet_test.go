// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubelet

package utils

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"

	"github.com/stretchr/testify/assert"
)

func TestConfigsForPod(t *testing.T) {
	tests := []struct {
		name    string
		check   *types.PrometheusCheck
		pod     *kubelet.Pod
		want    []integration.Config
		matched bool
	}{
		{
			name:  "nominal case",
			check: types.DefaultPrometheusCheck,
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
					Instances:     []integration.Data{integration.Data(`{"prometheus_url":"http://%%host%%:%%port%%/metrics","namespace":"","metrics":["*"]}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:foo-ctr-id",
					ADIdentifiers: []string{"foo-ctr-id"},
				},
			},
		},
		{
			name: "custom prometheus_url",
			check: &types.PrometheusCheck{
				Instances: []*types.OpenmetricsInstance{
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
					Instances:     []integration.Data{integration.Data(`{"prometheus_url":"foo/bar","namespace":"","metrics":["*"]}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:foo-ctr-id",
					ADIdentifiers: []string{"foo-ctr-id"},
				},
			},
		},
		{
			name:  "excluded",
			check: types.DefaultPrometheusCheck,
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
			check: types.DefaultPrometheusCheck,
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
			check: types.DefaultPrometheusCheck,
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
					Instances:     []integration.Data{integration.Data(`{"prometheus_url":"http://%%host%%:%%port%%/metrics","namespace":"","metrics":["*"]}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:foo-ctr1-id",
					ADIdentifiers: []string{"foo-ctr1-id"},
				},
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"prometheus_url":"http://%%host%%:%%port%%/metrics","namespace":"","metrics":["*"]}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:foo-ctr2-id",
					ADIdentifiers: []string{"foo-ctr2-id"},
				},
			},
		},
		{
			name: "multi containers, match one container",
			check: &types.PrometheusCheck{
				AD: &types.ADConfig{
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
					Instances:     []integration.Data{integration.Data(`{"prometheus_url":"http://%%host%%:%%port%%/metrics","namespace":"","metrics":["*"]}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:foo-ctr1-id",
					ADIdentifiers: []string{"foo-ctr1-id"},
				},
			},
		},
		{
			name: "container name mismatch",
			check: &types.PrometheusCheck{
				AD: &types.ADConfig{
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
			check: &types.PrometheusCheck{
				AD: &types.ADConfig{
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
					Instances:     []integration.Data{integration.Data(`{"prometheus_url":"http://%%host%%:%%port%%/metrics","namespace":"","metrics":["*"]}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:foo-ctr-id",
					ADIdentifiers: []string{"foo-ctr-id"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check.Init()
			assert.ElementsMatch(t, tt.want, ConfigsForPod(tt.check, tt.pod))
		})
	}
}
