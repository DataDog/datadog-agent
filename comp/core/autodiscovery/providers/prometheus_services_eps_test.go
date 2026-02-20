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
	discv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

type MockServiceEndpointSlicesAPI struct {
	mock.Mock
}

func (api *MockServiceEndpointSlicesAPI) ListServices() ([]*v1.Service, error) {
	args := api.Called()
	return args.Get(0).([]*v1.Service), args.Error(1)
}

func (api *MockServiceEndpointSlicesAPI) ListEndpointSlices(namespace, name string) ([]*discv1.EndpointSlice, error) {
	args := api.Called(namespace, name)
	return args.Get(0).([]*discv1.EndpointSlice), args.Error(1)
}

func TestPrometheusServicesEPS_Collect(t *testing.T) {
	nodeNames := []string{
		"node1",
		"node2",
	}

	tests := []struct {
		name             string
		checks           []*types.PrometheusCheck
		services         []*v1.Service
		endpointSlices   []*discv1.EndpointSlice
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
			name:   "collect only endpointslices",
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
			endpointSlices: []*discv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-abc123",
						Namespace: "ns",
						Labels: map[string]string{
							"kubernetes.io/service-name": "svc",
						},
					},
					Endpoints: []discv1.Endpoint{
						{
							Addresses: []string{"10.0.0.1"},
							TargetRef: &v1.ObjectReference{
								Kind: "Pod",
								UID:  "svc-pod-1",
							},
							NodeName: &nodeNames[0],
						},
						{
							Addresses: []string{"10.0.0.2"},
							TargetRef: &v1.ObjectReference{
								Kind: "Pod",
								UID:  "svc-pod-2",
							},
							NodeName: &nodeNames[1],
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
					Provider:      "prometheus-services-endpointslices",
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
					Provider:      "prometheus-services-endpointslices",
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
			api := &MockServiceEndpointSlicesAPI{}
			defer api.AssertExpectations(t)

			api.On("ListServices").Return(test.services, nil)
			for _, slice := range test.endpointSlices {
				serviceName := slice.Labels["kubernetes.io/service-name"]
				api.On("ListEndpointSlices", slice.GetNamespace(), serviceName).Return([]*discv1.EndpointSlice{slice}, nil)
			}

			for _, check := range test.checks {
				check.Init(2)
			}

			p := newPromServicesEndpointSlicesProvider(test.checks, api, test.collectEndpoints)
			configs, err := p.Collect(ctx)

			assert.Equal(t, test.expectConfigs, configs)
			assert.Equal(t, test.expectErr, err)
		})
	}
}

func TestPrometheusServicesEPS_Invalidate(t *testing.T) {
	ctx := context.Background()
	api := &MockServiceEndpointSlicesAPI{}
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
	p := newPromServicesEndpointSlicesProvider([]*types.PrometheusCheck{}, api, true)
	p.monitoredEndpoints["kube_endpoint_uid://ns/svc/"] = true
	p.setUpToDate(true)
	p.invalidate(svc)

	upToDate, err := p.IsUpToDate(ctx)
	assert.NoError(t, err)
	assert.False(t, upToDate)
	assert.Empty(t, p.monitoredEndpoints)
}

func TestPrometheusServicesEPS_InvalidateIfChanged(t *testing.T) {
	api := &MockServiceEndpointSlicesAPI{}
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
			p := newPromServicesEndpointSlicesProvider(checks, api, true)
			p.setUpToDate(true)
			p.invalidateIfChanged(test.old, test.new)

			upToDate, err := p.IsUpToDate(ctx)
			assert.NoError(t, err)
			assert.Equal(t, test.expectUpToDate, upToDate)
		})
	}

}

func TestPrometheusServicesEPS_InvalidateIfChangedEndpointSlices(t *testing.T) {
	api := &MockServiceEndpointSlicesAPI{}
	defer api.AssertExpectations(t)

	checks := []*types.PrometheusCheck{types.DefaultPrometheusCheck}
	for _, check := range checks {
		check.Init(0)
	}

	node := "node1"
	tests := []struct {
		name               string
		old                *discv1.EndpointSlice
		new                *discv1.EndpointSlice
		monitoredEndpoints []string
		expectUpToDate     bool
	}{
		{
			name: "no change",
			old: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "v1",
					Name:            "svc-abc123",
					Namespace:       "ns",
					Labels: map[string]string{
						"kubernetes.io/service-name": "svc",
					},
				},
				Endpoints: []discv1.Endpoint{
					{
						Addresses: []string{"10.0.0.1"},
						TargetRef: &v1.ObjectReference{
							Kind: "Pod",
							UID:  "svc-pod-1",
						},
						NodeName: &node,
					},
				},
			},
			new: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "v1",
					Name:            "svc-abc123",
					Namespace:       "ns",
					Labels: map[string]string{
						"kubernetes.io/service-name": "svc",
					},
				},
				Endpoints: []discv1.Endpoint{
					{
						Addresses: []string{"10.0.0.1"},
						TargetRef: &v1.ObjectReference{
							Kind: "Pod",
							UID:  "svc-pod-1",
						},
						NodeName: &node,
					},
				},
			},
			expectUpToDate: true,
		},
		{
			name: "no endpoints change",
			old: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "v1",
					Name:            "svc-abc123",
					Namespace:       "ns",
					Labels: map[string]string{
						"kubernetes.io/service-name": "svc",
					},
				},
				Endpoints: []discv1.Endpoint{
					{
						Addresses: []string{"10.0.0.1"},
						TargetRef: &v1.ObjectReference{
							Kind: "Pod",
							UID:  "svc-pod-1",
						},
						NodeName: &node,
					},
				},
			},
			new: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "v2",
					Name:            "svc-abc123",
					Namespace:       "ns",
					Labels: map[string]string{
						"kubernetes.io/service-name": "svc",
					},
				},
				Endpoints: []discv1.Endpoint{
					{
						Addresses: []string{"10.0.0.1"},
						TargetRef: &v1.ObjectReference{
							Kind: "Pod",
							UID:  "svc-pod-1",
						},
						NodeName: &node,
					},
				},
			},
			monitoredEndpoints: []string{
				"kube_endpoint_uid://ns/svc/",
			},
			expectUpToDate: true,
		},
		{
			name: "new address added to endpoint slice",
			old: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "v1",
					Name:            "svc-abc123",
					Namespace:       "ns",
					Labels: map[string]string{
						"kubernetes.io/service-name": "svc",
					},
				},
				Endpoints: []discv1.Endpoint{
					{
						Addresses: []string{"10.0.0.1"},
						TargetRef: &v1.ObjectReference{
							Kind: "Pod",
							UID:  "svc-pod-1",
						},
						NodeName: &node,
					},
				},
			},
			new: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "v2",
					Name:            "svc-abc123",
					Namespace:       "ns",
					Labels: map[string]string{
						"kubernetes.io/service-name": "svc",
					},
				},
				Endpoints: []discv1.Endpoint{
					{
						Addresses: []string{"10.0.0.1"},
						TargetRef: &v1.ObjectReference{
							Kind: "Pod",
							UID:  "svc-pod-1",
						},
						NodeName: &node,
					},
					{
						Addresses: []string{"10.0.0.2"},
						TargetRef: &v1.ObjectReference{
							Kind: "Pod",
							UID:  "svc-pod-2",
						},
						NodeName: &node,
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
			p := newPromServicesEndpointSlicesProvider(checks, api, true)
			p.setUpToDate(true)
			for _, monitored := range test.monitoredEndpoints {
				p.monitoredEndpoints[monitored] = true
			}
			p.invalidateIfChangedEndpointSlices(test.old, test.new)

			upToDate, err := p.IsUpToDate(ctx)
			assert.NoError(t, err)
			assert.Equal(t, test.expectUpToDate, upToDate)
		})
	}
}
