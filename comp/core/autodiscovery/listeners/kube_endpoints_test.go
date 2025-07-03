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

	filter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
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

	eps := processEndpoints(kep, []string{"foo:bar"})

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
		filterScope     filter.Scope
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
			filterScope:     filter.MetricsFilter,
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
			filterScope:     filter.MetricsFilter,
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
			filterScope:     filter.LogsFilter,
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
			filterScope:     filter.LogsFilter,
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
			filterScope:     filter.GlobalFilter,
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
			filterScope:     filter.GlobalFilter,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := processService(tt.ksvc)
			svc.metricsExcluded = tt.metricsExcluded
			svc.globalExcluded = tt.globalExcluded
			isFilter := svc.HasFilter(tt.filterScope)
			assert.Equal(t, isFilter, tt.want)
		})
	}
}
