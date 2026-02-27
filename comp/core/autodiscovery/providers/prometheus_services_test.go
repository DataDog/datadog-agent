// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	pkgconfigmock "github.com/DataDog/datadog-agent/pkg/config/mock"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

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
		checks           []*types.PrometheusCheck
		services         []*v1.Service
		endpoints        []*v1.Endpoints
		collectEndpoints bool
		expectConfigs    []integration.Config
		expectErr        error
	}{
		{
			name:   "collect services only",
			checks: []*types.PrometheusCheck{types.DefaultPrometheusCheck},
			services: []*v1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						UID:       k8stypes.UID("test"),
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
						integration.Data(`{"namespace":"","metrics":[".*"],"openmetrics_endpoint":"http://%%host%%:1234/mewtrix"}`),
					},
					ADIdentifiers: []string{"kube_service://ns/svc"},
					Provider:      "prometheus-services",
					ClusterCheck:  true,
					Source:        "prometheus_services:kube_service://ns/svc",
				},
			},
		},
		{
			name:   "collect only endpoints",
			checks: []*types.PrometheusCheck{types.DefaultPrometheusCheck},
			services: []*v1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						UID:       k8stypes.UID("test"),
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
					ServiceID:  "kube_endpoint_uid://ns/svc/10.0.0.1",
					InitConfig: integration.Data("{}"),
					Instances: []integration.Data{
						integration.Data(`{"namespace":"","metrics":[".*"],"openmetrics_endpoint":"http://%%host%%:1234/mewtrix"}`),
					},
					ADIdentifiers: []string{"kube_endpoint_uid://ns/svc/10.0.0.1", "kubernetes_pod://svc-pod-1"},
					NodeName:      "node1",
					Provider:      "prometheus-services",
					ClusterCheck:  true,
					Source:        "prometheus_services:kube_endpoint_uid://ns/svc/10.0.0.1",
				},
				{
					Name:       "openmetrics",
					ServiceID:  "kube_endpoint_uid://ns/svc/10.0.0.2",
					InitConfig: integration.Data("{}"),
					Instances: []integration.Data{
						integration.Data(`{"namespace":"","metrics":[".*"],"openmetrics_endpoint":"http://%%host%%:1234/mewtrix"}`),
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

	cfg := pkgconfigmock.New(t)
	cfg.SetWithoutSource("prometheus_scrape.version", 2)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			api := &MockServiceAPI{}
			defer api.AssertExpectations(t)

			api.On("ListServices").Return(test.services, nil)
			for _, endpoint := range test.endpoints {
				api.On("GetEndpoints", endpoint.GetNamespace(), endpoint.GetName()).Return(endpoint, nil)
			}

			for _, check := range test.checks {
				check.Init(2)
			}

			p := newPromServicesProvider(test.checks, api, test.collectEndpoints)
			configs, err := p.Collect(ctx)

			assert.Equal(t, test.expectConfigs, configs)
			assert.Equal(t, test.expectErr, err)
		})
	}
}

func TestPrometheusServicesInvalidate(t *testing.T) {
	ctx := context.Background()
	api := &MockServiceAPI{}
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			UID:       k8stypes.UID("test"),
			Name:      "svc",
			Namespace: "ns",
			Annotations: map[string]string{
				"prometheus.io/scrape": "true",
				"prometheus.io/path":   "/mewtrix",
				"prometheus.io/port":   "1234",
			},
		},
	}
	p := newPromServicesProvider([]*types.PrometheusCheck{}, api, true)
	p.monitoredEndpoints["kube_endpoint_uid://ns/svc/"] = true
	p.setUpToDate(true)
	p.invalidate(svc)

	upToDate, err := p.IsUpToDate(ctx)
	assert.NoError(t, err)
	assert.False(t, upToDate)
	assert.Empty(t, p.monitoredEndpoints)
}

func TestPrometheusServicesInvalidateIfChanged(t *testing.T) {
	api := &MockServiceAPI{}
	defer api.AssertExpectations(t)

	checks := []*types.PrometheusCheck{types.DefaultPrometheusCheck}
	for _, check := range checks {
		check.Init(0)
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
					UID:             k8stypes.UID("test"),
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
					UID:             k8stypes.UID("test"),
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
					UID:             k8stypes.UID("test"),
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
					UID:             k8stypes.UID("test"),
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
					UID:             k8stypes.UID("test"),
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
					UID:             k8stypes.UID("test"),
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
			ctx := context.Background()
			p := newPromServicesProvider(checks, api, true)
			p.setUpToDate(true)
			p.invalidateIfChanged(test.old, test.new)

			upToDate, err := p.IsUpToDate(ctx)
			assert.NoError(t, err)
			assert.Equal(t, test.expectUpToDate, upToDate)
		})
	}

}

func TestPrometheusServicesInvalidateIfChangedEndpoints(t *testing.T) {
	api := &MockServiceAPI{}
	defer api.AssertExpectations(t)

	checks := []*types.PrometheusCheck{types.DefaultPrometheusCheck}
	for _, check := range checks {
		check.Init(0)
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
			ctx := context.Background()
			p := newPromServicesProvider(checks, api, true)
			p.setUpToDate(true)
			for _, monitored := range test.monitoredEndpoints {
				p.monitoredEndpoints[monitored] = true
			}
			p.invalidateIfChangedEndpoints(test.old, test.new)

			upToDate, err := p.IsUpToDate(ctx)
			assert.NoError(t, err)
			assert.Equal(t, test.expectUpToDate, upToDate)
		})
	}
}
