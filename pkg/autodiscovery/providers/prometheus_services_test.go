// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks
// +build kubeapiserver

package providers

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestPromAnnotationsDiffer(t *testing.T) {
	tests := []struct {
		name   string
		checks []*common.PrometheusCheck
		first  map[string]string
		second map[string]string
		want   bool
	}{
		{
			name:   "scrape annotation changed",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/scrape": "true"},
			second: map[string]string{"prometheus.io/scrape": "false"},
			want:   true,
		},
		{
			name:   "scrape annotation unchanged",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/scrape": "true"},
			second: map[string]string{"prometheus.io/scrape": "true"},
			want:   false,
		},
		{
			name:   "scrape annotation removed",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/scrape": "true"},
			second: map[string]string{"foo": "bar"},
			want:   true,
		},
		{
			name:   "path annotation changed",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/path": "/metrics"},
			second: map[string]string{"prometheus.io/path": "/metrics_custom"},
			want:   true,
		},
		{
			name:   "path annotation unchanged",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/path": "/metrics"},
			second: map[string]string{"prometheus.io/path": "/metrics"},
			want:   false,
		},
		{
			name:   "port annotation changed",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/port": "1234"},
			second: map[string]string{"prometheus.io/port": "4321"},
			want:   true,
		},
		{
			name:   "port annotation unchanged",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/port": "1234"},
			second: map[string]string{"prometheus.io/port": "1234"},
			want:   false,
		},
		{
			name:   "include annotation changed",
			checks: []*common.PrometheusCheck{{AD: &common.ADConfig{KubeAnnotations: &common.InclExcl{Incl: map[string]string{"include": "true"}}}}},
			first:  map[string]string{"include": "true"},
			second: map[string]string{"include": "foo"},
			want:   true,
		},
		{
			name:   "include annotation unchanged",
			checks: []*common.PrometheusCheck{{AD: &common.ADConfig{KubeAnnotations: &common.InclExcl{Incl: map[string]string{"include": "true"}}}}},
			first:  map[string]string{"include": "true"},
			second: map[string]string{"include": "true"},
			want:   false,
		},
		{
			name:   "exclude annotation changed",
			checks: []*common.PrometheusCheck{{AD: &common.ADConfig{KubeAnnotations: &common.InclExcl{Excl: map[string]string{"exclude": "true"}}}}},
			first:  map[string]string{"exclude": "true"},
			second: map[string]string{"exclude": "foo"},
			want:   true,
		},
		{
			name:   "exclude annotation unchanged",
			checks: []*common.PrometheusCheck{{AD: &common.ADConfig{KubeAnnotations: &common.InclExcl{Excl: map[string]string{"exclude": "true"}}}}},
			first:  map[string]string{"exclude": "true"},
			second: map[string]string{"exclude": "true"},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PrometheusServicesConfigProvider{}
			p.checks = tt.checks
			if got := p.promAnnotationsDiffer(tt.first, tt.second); got != tt.want {
				t.Errorf("PrometheusServicesConfigProvider.promAnnotationsDiffer() = %v, want %v", got, tt.want)
			}
		})
	}
}

type MockServiceAPI struct {
	mock.Mock
}

func (api *MockServiceAPI) ListServices() ([]*v1.Service, error) {
	args := api.Called()
	return args.Get(0).([]*v1.Service), args.Error(1)
}

func (api *MockServiceAPI) GetEndpoints(namespace, name string) (*v1.Endpoints, error) {
	args := api.Called(namespace, name)
	return args.Get(0).(*v1.Endpoints), args.Error(1)
}

func TestPrometheusServicesCollect(t *testing.T) {
	nodeNames := []string{
		"node1",
		"node2",
	}

	tests := []struct {
		name             string
		checks           []*common.PrometheusCheck
		services         []*v1.Service
		endpoints        []*v1.Endpoints
		collectEndpoints bool
		expectConfigs    []integration.Config
		expectErr        error
	}{
		{
			name:   "collect services only",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			services: []*v1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID("test"),
						Name:      "svc",
						Namespace: "ns",
						Annotations: map[string]string{
							"prometheus.io/scrape": "true",
							"prometheus.io/path":   "/mewtrix",
							"prometheus.io/port":   "1234",
						},
					},
				},
			},
			expectConfigs: []integration.Config{
				{
					Name:       "openmetrics",
					InitConfig: integration.Data("{}"),
					Instances: []integration.Data{
						integration.Data(`{"prometheus_url":"http://%%host%%:1234/mewtrix","namespace":"","metrics":["*"]}`),
					},
					ADIdentifiers: []string{"kube_service_uid://test"},
					Provider:      "prometheus-services",
					ClusterCheck:  true,
					Source:        "prometheus_services:kube_service_uid://test",
				},
			},
		},
		{
			name:   "collect services and endpoints",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			services: []*v1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID("test"),
						Name:      "svc",
						Namespace: "ns",
						Annotations: map[string]string{
							"prometheus.io/scrape": "true",
							"prometheus.io/path":   "/mewtrix",
							"prometheus.io/port":   "1234",
						},
					},
				},
			},
			endpoints: []*v1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc",
						Namespace: "ns",
					},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								{
									IP: "10.0.0.1",
									TargetRef: &v1.ObjectReference{
										Kind: "Pod",
										UID:  "svc-pod-1",
									},
									NodeName: &nodeNames[0],
								},
								{
									IP: "10.0.0.2",
									TargetRef: &v1.ObjectReference{
										Kind: "Pod",
										UID:  "svc-pod-2",
									},
									NodeName: &nodeNames[1],
								},
							},
						},
					},
				},
			},
			collectEndpoints: true,
			expectConfigs: []integration.Config{
				{
					Name:       "openmetrics",
					InitConfig: integration.Data("{}"),
					Instances: []integration.Data{
						integration.Data(`{"prometheus_url":"http://%%host%%:1234/mewtrix","namespace":"","metrics":["*"]}`),
					},
					ADIdentifiers: []string{"kube_service_uid://test"},
					Provider:      "prometheus-services",
					ClusterCheck:  true,
					Source:        "prometheus_services:kube_service_uid://test",
				},
				{
					Name:       "openmetrics",
					Entity:     "kube_endpoint_uid://ns/svc/10.0.0.1",
					InitConfig: integration.Data("{}"),
					Instances: []integration.Data{
						integration.Data(`{"prometheus_url":"http://%%host%%:1234/mewtrix","namespace":"","metrics":["*"]}`),
					},
					ADIdentifiers: []string{"kube_endpoint_uid://ns/svc/10.0.0.1", "kubernetes_pod://svc-pod-1"},
					NodeName:      "node1",
					Provider:      "prometheus-services",
					ClusterCheck:  true,
					Source:        "prometheus_services:kube_endpoint_uid://ns/svc/10.0.0.1",
				},
				{
					Name:       "openmetrics",
					Entity:     "kube_endpoint_uid://ns/svc/10.0.0.2",
					InitConfig: integration.Data("{}"),
					Instances: []integration.Data{
						integration.Data(`{"prometheus_url":"http://%%host%%:1234/mewtrix","namespace":"","metrics":["*"]}`),
					},
					ADIdentifiers: []string{"kube_endpoint_uid://ns/svc/10.0.0.2", "kubernetes_pod://svc-pod-2"},
					NodeName:      "node2",
					Provider:      "prometheus-services",
					ClusterCheck:  true,
					Source:        "prometheus_services:kube_endpoint_uid://ns/svc/10.0.0.2",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := &MockServiceAPI{}
			defer api.AssertExpectations(t)

			api.On("ListServices").Return(test.services, nil)
			for _, endpoint := range test.endpoints {
				api.On("GetEndpoints", endpoint.GetNamespace(), endpoint.GetName()).Return(endpoint, nil)
			}

			for _, check := range test.checks {
				check.Init()
			}

			p := newPromServicesProvider(test.checks, api, test.collectEndpoints)
			configs, err := p.Collect()

			assert.Equal(t, test.expectConfigs, configs)
			assert.Equal(t, test.expectErr, err)
		})
	}
}

func TestPrometheusServicesInvalidate(t *testing.T) {
	api := &MockServiceAPI{}
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			UID:       types.UID("test"),
			Name:      "svc",
			Namespace: "ns",
			Annotations: map[string]string{
				"prometheus.io/scrape": "true",
				"prometheus.io/path":   "/mewtrix",
				"prometheus.io/port":   "1234",
			},
		},
	}
	p := newPromServicesProvider([]*common.PrometheusCheck{}, api, true)
	p.monitoredEndpoints["kube_endpoint_uid://ns/svc/"] = true
	p.setUpToDate(true)
	p.invalidate(svc)

	upToDate, err := p.IsUpToDate()
	assert.NoError(t, err)
	assert.False(t, upToDate)
	assert.Empty(t, p.monitoredEndpoints)
}

func TestPrometheusServicesInvalidateIfChanged(t *testing.T) {
	api := &MockServiceAPI{}
	defer api.AssertExpectations(t)

	checks := []*common.PrometheusCheck{common.DefaultPrometheusCheck}
	for _, check := range checks {
		check.Init()
	}

	tests := []struct {
		name           string
		old            *v1.Service
		new            *v1.Service
		expectUpToDate bool
	}{
		{
			name: "no version change",

			old: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:             types.UID("test"),
					ResourceVersion: "v1",
					Name:            "svc",
					Namespace:       "ns",
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/path":   "/mewtrix",
						"prometheus.io/port":   "1234",
					},
				},
			},
			new: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:             types.UID("test"),
					ResourceVersion: "v1",
					Name:            "svc",
					Namespace:       "ns",
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/path":   "/mewtrix",
						"prometheus.io/port":   "1234",
					},
				},
			},
			expectUpToDate: true,
		},
		{
			name: "no prometheus annotation change",
			old: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "v1",
					UID:             types.UID("test"),
					Name:            "svc",
					Namespace:       "ns",
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/path":   "/mewtrix",
						"prometheus.io/port":   "1234",
					},
				},
			},
			new: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:             types.UID("test"),
					ResourceVersion: "v2",
					Name:            "svc",
					Namespace:       "ns",
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/path":   "/mewtrix",
						"prometheus.io/port":   "1234",
						"something-else":       "yet",
					},
				},
			},
			expectUpToDate: true,
		},
		{
			name: "prometheus annotation change",
			old: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "v1",
					UID:             types.UID("test"),
					Name:            "svc",
					Namespace:       "ns",
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/path":   "/mewtrix",
						"prometheus.io/port":   "1234",
					},
				},
			},
			new: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:             types.UID("test"),
					ResourceVersion: "v2",
					Name:            "svc",
					Namespace:       "ns",
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/path":   "/mewtrix",
						"prometheus.io/port":   "1235",
					},
				},
			},
			expectUpToDate: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := newPromServicesProvider(checks, api, true)
			p.setUpToDate(true)
			p.invalidateIfChanged(test.old, test.new)

			upToDate, err := p.IsUpToDate()
			assert.NoError(t, err)
			assert.Equal(t, test.expectUpToDate, upToDate)
		})
	}

}

func TestPrometheusServicesInvalidateIfChangedEndpoints(t *testing.T) {
	api := &MockServiceAPI{}
	defer api.AssertExpectations(t)

	checks := []*common.PrometheusCheck{common.DefaultPrometheusCheck}
	for _, check := range checks {
		check.Init()
	}

	node := "node1"
	tests := []struct {
		name               string
		old                *v1.Endpoints
		new                *v1.Endpoints
		monitoredEndpoints []string
		expectUpToDate     bool
	}{
		{
			name: "no change",
			old: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "v1",
					Name:            "svc",
					Namespace:       "ns",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{
								IP: "10.0.0.1",
								TargetRef: &v1.ObjectReference{
									Kind: "Pod",
									UID:  "svc-pod-1",
								},
								NodeName: &node,
							},
						},
					},
				},
			},
			new: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "v1",
					Name:            "svc",
					Namespace:       "ns",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{
								IP: "10.0.0.1",
								TargetRef: &v1.ObjectReference{
									Kind: "Pod",
									UID:  "svc-pod-1",
								},
								NodeName: &node,
							},
						},
					},
				},
			},
			expectUpToDate: true,
		},
		{
			name: "no subsets change",
			old: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "v1",
					Name:            "svc",
					Namespace:       "ns",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{
								IP: "10.0.0.1",
								TargetRef: &v1.ObjectReference{
									Kind: "Pod",
									UID:  "svc-pod-1",
								},
								NodeName: &node,
							},
						},
					},
				},
			},
			new: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "v2",
					Name:            "svc",
					Namespace:       "ns",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{
								IP: "10.0.0.1",
								TargetRef: &v1.ObjectReference{
									Kind: "Pod",
									UID:  "svc-pod-1",
								},
								NodeName: &node,
							},
						},
					},
				},
			},
			monitoredEndpoints: []string{
				"kube_endpoint_uid://ns/svc/",
			},
			expectUpToDate: true,
		},
		{
			name: "subsets change",
			old: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "v1",
					Name:            "svc",
					Namespace:       "ns",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{
								IP: "10.0.0.1",
								TargetRef: &v1.ObjectReference{
									Kind: "Pod",
									UID:  "svc-pod-1",
								},
								NodeName: &node,
							},
						},
					},
				},
			},
			new: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "v2",
					Name:            "svc",
					Namespace:       "ns",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{
								IP: "10.0.0.1",
								TargetRef: &v1.ObjectReference{
									Kind: "Pod",
									UID:  "svc-pod-1",
								},
								NodeName: &node,
							},
							{
								IP: "10.0.0.2",
								TargetRef: &v1.ObjectReference{
									Kind: "Pod",
									UID:  "svc-pod-2",
								},
								NodeName: &node,
							},
						},
					},
				},
			},
			monitoredEndpoints: []string{
				"kube_endpoint_uid://ns/svc/",
			},
			expectUpToDate: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := newPromServicesProvider(checks, api, true)
			p.setUpToDate(true)
			for _, monitored := range test.monitoredEndpoints {
				p.monitoredEndpoints[monitored] = true
			}
			p.invalidateIfChangedEndpoints(test.old, test.new)

			upToDate, err := p.IsUpToDate()
			assert.NoError(t, err)
			assert.Equal(t, test.expectUpToDate, upToDate)
		})
	}
}
