// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestConfigsForPod(t *testing.T) {
	tests := []struct {
		name        string
		check       *types.PrometheusCheck
		version     int
		pod         *workloadmeta.KubernetesPod
		containers  []*workloadmeta.Container
		want        []integration.Config
		expectError bool
	}{
		{
			name:    "nominal case v1",
			check:   types.DefaultPrometheusCheck,
			version: 1,
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "pod-id",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "pod-name",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "cont-id-1",
						Name: "cont-name-1",
					},
				},
			},
			containers: []*workloadmeta.Container{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-1",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-1",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"prometheus_url":"http://%%host%%:%%port%%/metrics","namespace":"","metrics":["*"]}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:containerd://cont-id-1",
					ADIdentifiers: []string{"containerd://cont-id-1"},
				},
			},
		},
		{
			name:    "nominal case v2",
			check:   types.DefaultPrometheusCheck,
			version: 2,
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "pod-id",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "pod-name",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "cont-id-1",
						Name: "cont-name-1",
					},
				},
			},
			containers: []*workloadmeta.Container{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-1",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-1",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"namespace":"","metrics":[".*"],"openmetrics_endpoint":"http://%%host%%:%%port%%/metrics"}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:containerd://cont-id-1",
					ADIdentifiers: []string{"containerd://cont-id-1"},
				},
			},
		},
		{
			name: "custom openmetrics_endpoint",
			check: &types.PrometheusCheck{
				Instances: []*types.OpenmetricsInstance{
					{
						OpenMetricsEndpoint: "foo/bar",
						Metrics:             []interface{}{".*"},
						Namespace:           "",
					},
				},
			},
			version: 1,
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "pod-id",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "pod-name",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "cont-id-1",
						Name: "cont-name-1",
					},
				},
			},
			containers: []*workloadmeta.Container{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-1",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-1",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"namespace":"","metrics":[".*"],"openmetrics_endpoint":"foo/bar"}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:containerd://cont-id-1",
					ADIdentifiers: []string{"containerd://cont-id-1"},
				},
			},
		},
		{
			name: "custom prometheus_url",
			check: &types.PrometheusCheck{
				Instances: []*types.OpenmetricsInstance{
					{
						PrometheusURL: "foo/bar",
						Metrics:       []interface{}{"*"},
						Namespace:     "",
					},
				},
			},
			version: 2,
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "pod-id",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "pod-name",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "cont-id-1",
						Name: "cont-name-1",
					},
				},
			},
			containers: []*workloadmeta.Container{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-1",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-1",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"prometheus_url":"foo/bar","namespace":"","metrics":["*"]}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:containerd://cont-id-1",
					ADIdentifiers: []string{"containerd://cont-id-1"},
				},
			},
		},
		{
			name:    "excluded",
			check:   types.DefaultPrometheusCheck,
			version: 2,
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "pod-id",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "pod-name",
					Annotations: map[string]string{"prometheus.io/scrape": "false"},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "cont-id-1",
						Name: "cont-name-1",
					},
				},
			},
			containers: []*workloadmeta.Container{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-1",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-1",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
				},
			},
			want: nil,
		},
		{
			name:    "multi containers, match all",
			check:   types.DefaultPrometheusCheck,
			version: 2,
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "pod-id",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "pod-name",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "cont-id-1",
						Name: "cont-name-1",
					},
					{
						ID:   "cont-id-2",
						Name: "cont-name-2",
					},
				},
			},
			containers: []*workloadmeta.Container{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-1",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-1",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-2",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-2",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"namespace":"","metrics":[".*"],"openmetrics_endpoint":"http://%%host%%:%%port%%/metrics"}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:containerd://cont-id-1",
					ADIdentifiers: []string{"containerd://cont-id-1"},
				},
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"namespace":"","metrics":[".*"],"openmetrics_endpoint":"http://%%host%%:%%port%%/metrics"}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:containerd://cont-id-2",
					ADIdentifiers: []string{"containerd://cont-id-2"},
				},
			},
		},
		{
			name: "multi containers, match one container",
			check: &types.PrometheusCheck{
				AD: &types.ADConfig{
					KubeContainerNames: []string{"cont-name-1"},
				},
			},
			version: 2,
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "pod-id",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "pod-name",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "cont-id-1",
						Name: "cont-name-1",
					},
					{
						ID:   "cont-id-2",
						Name: "cont-name-2",
					},
				},
			},
			containers: []*workloadmeta.Container{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-1",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-1",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-2",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-2",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"namespace":"","metrics":[".*"],"openmetrics_endpoint":"http://%%host%%:%%port%%/metrics"}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:containerd://cont-id-1",
					ADIdentifiers: []string{"containerd://cont-id-1"},
				},
			},
		},
		{
			name:    "multi containers, match based on port",
			check:   types.DefaultPrometheusCheck,
			version: 2,
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "pod-id",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: "pod-name",
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/port":   "8080",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "cont-id-1",
						Name: "cont-name-1",
					},
					{
						ID:   "cont-id-2",
						Name: "cont-name-2",
					},
				},
			},
			containers: []*workloadmeta.Container{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-1",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-1",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
					Ports: []workloadmeta.ContainerPort{
						{
							Port: 8080,
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-2",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-2",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
					Ports: []workloadmeta.ContainerPort{
						{
							Port: 8081,
						},
					},
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"namespace":"","metrics":[".*"],"openmetrics_endpoint":"http://%%host%%:8080/metrics"}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:containerd://cont-id-1",
					ADIdentifiers: []string{"containerd://cont-id-1"},
				},
			},
		},
		{
			name:    "multi containers, none match the port in the annotation",
			check:   types.DefaultPrometheusCheck,
			version: 2,
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "pod-id",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: "pod-name",
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/port":   "9999",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "cont-id-1",
						Name: "cont-name-1",
					},
					{
						ID:   "cont-id-2",
						Name: "cont-name-2",
					},
				},
			},
			containers: []*workloadmeta.Container{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-1",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-1",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
					Ports: []workloadmeta.ContainerPort{
						{
							Port: 8080, // Doesn't match port in annotation
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-2",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-2",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
					Ports: []workloadmeta.ContainerPort{
						{
							Port: 8081, // Doesn't match port in annotation
						},
					},
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"namespace":"","metrics":[".*"],"openmetrics_endpoint":"http://%%host%%:9999/metrics"}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:containerd://cont-id-1",
					ADIdentifiers: []string{"containerd://cont-id-1"},
				},
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"namespace":"","metrics":[".*"],"openmetrics_endpoint":"http://%%host%%:9999/metrics"}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:containerd://cont-id-2",
					ADIdentifiers: []string{"containerd://cont-id-2"},
				},
			},
		},
		{
			name:    "invalid port in annotation",
			check:   types.DefaultPrometheusCheck,
			version: 2,
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "pod-id",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: "pod-name",
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/port":   "invalid",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "cont-id-1",
						Name: "cont-name-1",
					},
				},
			},
			containers: []*workloadmeta.Container{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-1",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-1",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
					Ports: []workloadmeta.ContainerPort{
						{
							Port: 8080,
						},
					},
				},
			},
			expectError: true, // Invalid port in annotation
		},
		{
			name: "container name mismatch",
			check: &types.PrometheusCheck{
				AD: &types.ADConfig{
					KubeContainerNames: []string{"bar"},
				},
			},
			version: 2,
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "pod-id",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "pod-name",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "cont-id-1",
						Name: "cont-name-1",
					},
				},
			},
			containers: []*workloadmeta.Container{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-1",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-1",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
				},
			},
			want: nil,
		},
		{
			name: "container name regex",
			check: &types.PrometheusCheck{
				AD: &types.ADConfig{
					KubeContainerNames: []string{"some-other-name", "*cont-name-*"}, // should match cont-name-1
				},
			},
			version: 2,
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "pod-id",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "pod-name",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "cont-id-1",
						Name: "cont-name-1",
					},
				},
			},
			containers: []*workloadmeta.Container{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-1",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-1",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"namespace":"","metrics":[".*"],"openmetrics_endpoint":"http://%%host%%:%%port%%/metrics"}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:containerd://cont-id-1",
					ADIdentifiers: []string{"containerd://cont-id-1"},
				},
			},
		},
		{
			name: "metrics key value",
			check: &types.PrometheusCheck{
				Instances: []*types.OpenmetricsInstance{
					{
						PrometheusURL: "foo/bar",
						Metrics:       []interface{}{map[string]string{"foo": "bar"}},
						Namespace:     "",
					},
				},
			},
			version: 2,
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "pod-id",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "pod-name",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "cont-id-1",
						Name: "cont-name-1",
					},
				},
			},
			containers: []*workloadmeta.Container{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "cont-id-1",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "cont-name-1",
					},
					Runtime: workloadmeta.ContainerRuntimeContainerd,
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"prometheus_url":"foo/bar","namespace":"","metrics":[{"foo":"bar"}]}`)},
					Provider:      names.PrometheusPods,
					Source:        "prometheus_pods:containerd://cont-id-1",
					ADIdentifiers: []string{"containerd://cont-id-1"},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := mock.New(t)
			cfg.SetWithoutSource("prometheus_scrape.version", test.version)
			test.check.Init(test.version)

			wmeta := newMockWorkloadMeta(t)
			wmeta.Set(test.pod)
			for _, container := range test.containers {
				wmeta.Set(container)
			}

			configs, err := ConfigsForPod(test.check, test.pod, wmeta)
			if test.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.ElementsMatch(t, test.want, configs)
		})
	}
}

func newMockWorkloadMeta(t *testing.T) workloadmetamock.Mock {
	return fxutil.Test[workloadmetamock.Mock](
		t,
		fx.Options(
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		),
	)
}
