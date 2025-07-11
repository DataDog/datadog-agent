// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package listeners

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestProcessEndpoints(t *testing.T) {
	kep := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "123",
			UID:             types.UID("endpoints-uid"),
			Name:            "myservice",
			Namespace:       "default",
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{IP: "10.0.0.1"},
					{IP: "10.0.0.2"},
				},
				Ports: []v1.EndpointPort{
					{Name: "port123", Port: 123},
					{Name: "port126", Port: 126},
				},
			},
		},
	}

	eps := processEndpoints(kep, []string{"foo:bar"}, workloadfilterfxmock.SetupMockFilter(t))

	// Sort eps to impose the order
	sort.Slice(eps, func(i, j int) bool {
		assert.Equal(t, 1, len(eps[i].hosts))
		assert.Equal(t, 1, len(eps[j].hosts))
		var keyi, keyj string
		for key := range eps[i].hosts {
			keyi = key
		}
		for key := range eps[j].hosts {
			keyj = key
		}
		return keyi < keyj
	})

	assert.Equal(t, "kube_endpoint_uid://default/myservice/10.0.0.1", eps[0].GetServiceID())

	adID := eps[0].GetADIdentifiers()
	assert.Equal(t, []string{"kube_endpoint_uid://default/myservice/10.0.0.1"}, adID)

	hosts, err := eps[0].GetHosts()
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"endpoint": "10.0.0.1"}, hosts)

	ports, err := eps[0].GetPorts()
	assert.NoError(t, err)
	assert.Equal(t, []ContainerPort{{123, "port123"}, {126, "port126"}}, ports)

	tags, err := eps[0].GetTags()
	assert.NoError(t, err)
	assert.Equal(t, []string{"kube_service:myservice", "kube_namespace:default", "kube_endpoint_ip:10.0.0.1", "foo:bar"}, tags)

	assert.Equal(t, "kube_endpoint_uid://default/myservice/10.0.0.2", eps[1].GetServiceID())

	adID = eps[1].GetADIdentifiers()
	assert.NoError(t, err)
	assert.Equal(t, []string{"kube_endpoint_uid://default/myservice/10.0.0.2"}, adID)

	hosts, err = eps[1].GetHosts()
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"endpoint": "10.0.0.2"}, hosts)

	ports, err = eps[1].GetPorts()
	assert.NoError(t, err)
	assert.Equal(t, []ContainerPort{{123, "port123"}, {126, "port126"}}, ports)

	tags, err = eps[1].GetTags()
	assert.NoError(t, err)
	assert.Equal(t, []string{"kube_service:myservice", "kube_namespace:default", "kube_endpoint_ip:10.0.0.2", "foo:bar"}, tags)
}

func TestSubsetsDiffer(t *testing.T) {
	for name, tc := range map[string]struct {
		first  *v1.Endpoints
		second *v1.Endpoints
		result bool
	}{
		"Same resversion": {
			first: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
			},
			second: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
			},
			result: false,
		},
		"Change resversion, same subsets": {
			first: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			second: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			result: false,
		},
		"Change IP": {
			first: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			second: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.3", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			result: true,
		},
		"Change Hostname": {
			first: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			second: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.3", Hostname: "host3"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			result: true,
		},
		"Change port number": {
			first: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			second: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 124},
						},
					},
				},
			},
			result: true,
		},
		"Remove IP": {
			first: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 124},
						},
					},
				},
			},
			second: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 124},
						},
					},
				},
			},
			result: true,
		},
		"Remove port": {
			first: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 124},
						},
					},
				},
			},
			second: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
						},
					},
				},
			},
			result: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.result, subsetsDiffer(tc.first, tc.second))
		})
	}
}

func Test_isLockForLE(t *testing.T) {
	tests := []struct {
		name string
		kep  *v1.Endpoints
		want bool
	}{
		{
			name: "nominal case",
			kep: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "lock",
			kep: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Annotations: map[string]string{
						"control-plane.alpha.kubernetes.io/leader": `{"holderIdentity":"gke-xx-vm","leaseDurationSeconds":15,"acquireTime":"2020-03-31T03:56:23Z","renewTime":"2020-04-30T21:27:47Z","leaderTransitions":10}`,
					},
				},
			},
			want: true,
		},
		{
			name: "nil",
			kep:  nil,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLockForLE(tt.kep); got != tt.want {
				t.Errorf("isLockForLE() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasFilterKubeEndpoints(t *testing.T) {
	tests := []struct {
		name            string
		ksvc            *v1.Service
		metricsExcluded bool
		globalExcluded  bool
		want            bool
		filterScope     workloadfilter.Scope
	}{
		{
			name: "metrics excluded is true",
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("test"),
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			metricsExcluded: true,
			want:            true,
			filterScope:     workloadfilter.MetricsFilter,
		},
		{
			name: "metrics excluded is false",
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("test"),
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			metricsExcluded: false,
			globalExcluded:  true,
			want:            false,
			filterScope:     workloadfilter.MetricsFilter,
		},
		{
			name: "metrics excluded is true with logs filter",
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("test"),
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			metricsExcluded: true,
			globalExcluded:  false,
			want:            false,
			filterScope:     workloadfilter.LogsFilter,
		},
		{
			name: "metrics excluded is false with logs filter",
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("test"),
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			metricsExcluded: false,
			globalExcluded:  true,
			want:            false,
			filterScope:     workloadfilter.LogsFilter,
		},
		{
			name: "global excluded is true",
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("test"),
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			metricsExcluded: true,
			globalExcluded:  true,
			want:            true,
			filterScope:     workloadfilter.GlobalFilter,
		},
		{
			name: "metrics excluded is false",
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("test"),
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			metricsExcluded: false,
			globalExcluded:  false,
			want:            false,
			filterScope:     workloadfilter.GlobalFilter,
		},
	}

	filterStore := workloadfilterfxmock.SetupMockFilter(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := processService(tt.ksvc, filterStore)
			svc.metricsExcluded = tt.metricsExcluded
			svc.globalExcluded = tt.globalExcluded
			isFilter := svc.HasFilter(tt.filterScope)
			assert.Equal(t, isFilter, tt.want)
		})
	}
}

func TestKubeEndpointsFiltering(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_exclude_metrics", []string{"kube_namespace:excluded-namespace"})
	mockConfig.SetWithoutSource("container_exclude", []string{"name:global-excluded"})
	mockFilterStore := workloadfilterfxmock.SetupMockFilter(t)

	// Create test endpoints with different scenarios
	testCases := []struct {
		name                string
		endpoint            *v1.Endpoints
		expectedMetricsExcl bool
		expectedGlobalExcl  bool
	}{
		{
			name: "normal endpoint: not excluded",
			endpoint: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "normal-service",
					Namespace: "default",
					UID:       types.UID("normal-uid"),
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1"},
						},
						Ports: []v1.EndpointPort{
							{Name: "http", Port: 80},
						},
					},
				},
			},
			expectedMetricsExcl: false,
			expectedGlobalExcl:  false,
		},
		{
			name: "endpoint in excluded namespace: metrics excluded",
			endpoint: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service-in-excluded-ns",
					Namespace: "excluded-namespace",
					UID:       types.UID("excluded-ns-uid"),
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "http", Port: 80},
						},
					},
				},
			},
			expectedMetricsExcl: true,
			expectedGlobalExcl:  false,
		},
		{
			name: "globally excluded endpoint",
			endpoint: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "global-excluded",
					Namespace: "default",
					UID:       types.UID("global-excluded-uid"),
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.3"},
						},
						Ports: []v1.EndpointPort{
							{Name: "http", Port: 80},
						},
					},
				},
			},
			expectedMetricsExcl: false,
			expectedGlobalExcl:  true,
		},
		{
			name: "endpoint with AD annotations: metrics excluded",
			endpoint: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ad-excluded",
					Namespace: "default",
					UID:       types.UID("ad-excluded-uid"),
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names": "[\"http_check\"]",
						"ad.datadoghq.com/metrics_exclude":     "true",
						"ad.datadoghq.com/exclude":             "false",
					},
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.4"},
						},
						Ports: []v1.EndpointPort{
							{Name: "http", Port: 80},
						},
					},
				},
			},
			expectedMetricsExcl: true,
			expectedGlobalExcl:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			eps := processEndpoints(tc.endpoint, []string{}, mockFilterStore)
			assert.NotEmpty(t, eps, "Should have at least one endpoint service")
			for _, ep := range eps {
				assert.Equal(t, tc.expectedMetricsExcl, ep.metricsExcluded,
					"Expected metricsExcluded to be %v for endpoint %s/%s",
					tc.expectedMetricsExcl, tc.endpoint.Namespace, tc.endpoint.Name)
				assert.Equal(t, tc.expectedGlobalExcl, ep.globalExcluded,
					"Expected globalExcluded to be %v for endpoint %s/%s",
					tc.expectedGlobalExcl, tc.endpoint.Namespace, tc.endpoint.Name)
			}
		})
	}
}
