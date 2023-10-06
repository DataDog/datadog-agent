// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package utils

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/stretchr/testify/assert"
)

func TestConfigsForService(t *testing.T) {
	tests := []struct {
		name    string
		check   *types.PrometheusCheck
		version int
		svc     *corev1.Service
		want    []integration.Config
	}{
		{
			name:    "nominal case v1",
			check:   types.DefaultPrometheusCheck,
			version: 1,
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:         k8stypes.UID("foo-uid"),
					Name:        "svc-foo",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
					Namespace:   "ns",
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"prometheus_url":"http://%%host%%:%%port%%/metrics","namespace":"","metrics":["*"]}`)},
					ClusterCheck:  true,
					Provider:      names.PrometheusServices,
					Source:        "prometheus_services:kube_service://ns/svc-foo",
					ADIdentifiers: []string{"kube_service://ns/svc-foo"},
				},
			},
		},
		{
			name:    "nominal case v2",
			check:   types.DefaultPrometheusCheck,
			version: 2,
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:         k8stypes.UID("foo-uid"),
					Name:        "svc-foo",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
					Namespace:   "ns",
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"namespace":"","metrics":[".*"],"openmetrics_endpoint":"http://%%host%%:%%port%%/metrics"}`)},
					ClusterCheck:  true,
					Provider:      names.PrometheusServices,
					Source:        "prometheus_services:kube_service://ns/svc-foo",
					ADIdentifiers: []string{"kube_service://ns/svc-foo"},
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
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:         k8stypes.UID("foo-uid"),
					Name:        "svc-foo",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
					Namespace:   "ns",
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"namespace":"","metrics":[".*"],"openmetrics_endpoint":"foo/bar"}`)},
					ClusterCheck:  true,
					Provider:      names.PrometheusServices,
					Source:        "prometheus_services:kube_service://ns/svc-foo",
					ADIdentifiers: []string{"kube_service://ns/svc-foo"},
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
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:         k8stypes.UID("foo-uid"),
					Name:        "svc-foo",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
					Namespace:   "ns",
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"prometheus_url":"foo/bar","namespace":"","metrics":["*"]}`)},
					ClusterCheck:  true,
					Provider:      names.PrometheusServices,
					Source:        "prometheus_services:kube_service://ns/svc-foo",
					ADIdentifiers: []string{"kube_service://ns/svc-foo"},
				},
			},
		},
		{
			name:    "excluded",
			check:   types.DefaultPrometheusCheck,
			version: 2,
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:         k8stypes.UID("foo-uid"),
					Name:        "svc-foo",
					Annotations: map[string]string{"prometheus.io/scrape": "false"},
				},
			},
			want: nil,
		},
		{
			name:    "no match",
			check:   types.DefaultPrometheusCheck,
			version: 2,
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:         k8stypes.UID("foo-uid"),
					Name:        "svc-foo",
					Annotations: map[string]string{"foo": "bar"},
				},
			},
			want: nil,
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
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:         k8stypes.UID("foo-uid"),
					Name:        "svc-foo",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
					Namespace:   "ns",
				},
			},
			want: []integration.Config{
				{
					Name:          "openmetrics",
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"prometheus_url":"foo/bar","namespace":"","metrics":[{"foo":"bar"}]}`)},
					ClusterCheck:  true,
					Provider:      names.PrometheusServices,
					Source:        "prometheus_services:kube_service://ns/svc-foo",
					ADIdentifiers: []string{"kube_service://ns/svc-foo"},
				},
			},
		},
		{
			name:    "headless service is ignored",
			check:   types.DefaultPrometheusCheck,
			version: 1,
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:         k8stypes.UID("foo-uid"),
					Name:        "svc-foo",
					Annotations: map[string]string{"prometheus.io/scrape": "true"},
					Namespace:   "ns",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "None",
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.Datadog.Set("prometheus_scrape.version", tt.version)
			assert.NoError(t, tt.check.Init(tt.version))
			assert.ElementsMatch(t, tt.want, ConfigsForService(tt.check, tt.svc))
		})
	}
}
