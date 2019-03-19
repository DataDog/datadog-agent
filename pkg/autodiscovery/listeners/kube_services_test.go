// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks
// +build kubeapiserver

package listeners

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

func TestProcessService(t *testing.T) {
	ksvc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "123",
			UID:             types.UID("test"),
			Annotations: map[string]string{
				"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
				"ad.datadoghq.com/service.init_configs": "[{}]",
				"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
			},
			Name:      "myservice",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			ClusterIP: "10.0.0.1",
			Ports: []v1.ServicePort{
				{Name: "test1", Port: 123},
				{Name: "test2", Port: 126},
			},
		},
	}

	svc := processService(ksvc, true)
	assert.Equal(t, "kube_service://test", svc.GetEntity())
	assert.Equal(t, integration.Before, svc.GetCreationTime())

	adID, err := svc.GetADIdentifiers()
	assert.NoError(t, err)
	assert.Equal(t, []string{"kube_service://test"}, adID)

	hosts, err := svc.GetHosts()
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"cluster": "10.0.0.1"}, hosts)

	ports, err := svc.GetPorts()
	assert.NoError(t, err)
	assert.Equal(t, []ContainerPort{{123, "test1"}, {126, "test2"}}, ports)

	tags, err := svc.GetTags()
	assert.NoError(t, err)
	assert.Equal(t, []string{"kube_service:myservice", "kube_namespace:default"}, tags)

	svc = processService(ksvc, false)
	assert.Equal(t, integration.After, svc.GetCreationTime())
}

func TestServicesDiffer(t *testing.T) {
	for name, tc := range map[string]struct {
		first  *v1.Service
		second *v1.Service
		result bool
	}{
		"Same resversion": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
			},
			result: false,
		},
		"Change resversion, same spec": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			result: false,
		},
		"Change IP": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.10",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			result: true,
		},
		"Change port number": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 124},
						{Name: "test2", Port: 126},
					},
				},
			},
			result: true,
		},
		"Remove port": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			result: true,
		},
		"Add annotation": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			result: true,
		},
		"Remove annotation": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			result: true,
		},
		"Add endpoints annotations": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":    "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs":   "[{}]",
						"ad.datadoghq.com/service.instances":      "[{\"name\" \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"ad.datadoghq.com/endpoints.check_names":  "[\"etcd\"]",
						"ad.datadoghq.com/endpoints.init_configs": "[{}]",
						"ad.datadoghq.com/endpoints.instances":    "[{\"use_preview\": \"true\", \"prometheus_url\": \"http://%%host%%:2379/metrics\"}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			result: true,
		},
		"Remove endpoints annotations": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":    "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs":   "[{}]",
						"ad.datadoghq.com/service.instances":      "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"ad.datadoghq.com/endpoints.check_names":  "[\"etcd\"]",
						"ad.datadoghq.com/endpoints.init_configs": "[{}]",
						"ad.datadoghq.com/endpoints.instances":    "[{\"use_preview\": \"true\", \"prometheus_url\": \"http://%%host%%:2379/metrics\"}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\" \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			result: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.result, servicesDiffer(tc.first, tc.second))
		})
	}
}

func TestProcessEndpoint(t *testing.T) {
	kendpt := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "123",
			UID:             types.UID("test"),
			Name:            "myendpoint",
			Namespace:       "default",
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{IP: "10.0.0.1", Hostname: "testhost1"},
					{IP: "10.0.0.2", Hostname: "testhost2"},
				},
				Ports: []v1.EndpointPort{
					{Name: "testport1", Port: 123},
					{Name: "testport2", Port: 126},
				},
			},
		},
	}

	endpts := processEndpoint(kendpt, true)
	// sort endpts to impose the order
	sort.Slice(endpts, func(i, j int) bool {
		assert.Equal(t, 1, len(endpts[i].hosts))
		assert.Equal(t, 1, len(endpts[j].hosts))
		var keyi, keyj string
		for key := range endpts[i].hosts {
			keyi = key
		}
		for key := range endpts[j].hosts {
			keyj = key
		}
		return keyi < keyj
	})
	assert.Equal(t, "kube_endpoint://default/myendpoint/10.0.0.1", endpts[0].GetEntity())
	assert.Equal(t, integration.Before, endpts[0].GetCreationTime())

	adID, err := endpts[0].GetADIdentifiers()
	assert.NoError(t, err)
	assert.Equal(t, []string{"kube_endpoint://default/myendpoint"}, adID)

	hosts, err := endpts[0].GetHosts()
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"service": "10.0.0.1"}, hosts)

	ports, err := endpts[0].GetPorts()
	assert.NoError(t, err)
	assert.Equal(t, []ContainerPort{{123, "testport1"}, {126, "testport2"}}, ports)

	tags, err := endpts[0].GetTags()
	assert.NoError(t, err)
	assert.Equal(t, []string{"kube_service:myendpoint", "kube_namespace:default", "kube_endpoint_ip:10.0.0.1"}, tags)

	assert.Equal(t, "kube_endpoint://default/myendpoint/10.0.0.2", endpts[1].GetEntity())
	assert.Equal(t, integration.Before, endpts[1].GetCreationTime())

	adID, err = endpts[1].GetADIdentifiers()
	assert.NoError(t, err)
	assert.Equal(t, []string{"kube_endpoint://default/myendpoint"}, adID)

	hosts, err = endpts[1].GetHosts()
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"service": "10.0.0.2"}, hosts)

	ports, err = endpts[1].GetPorts()
	assert.NoError(t, err)
	assert.Equal(t, []ContainerPort{{123, "testport1"}, {126, "testport2"}}, ports)

	tags, err = endpts[1].GetTags()
	assert.NoError(t, err)
	assert.Equal(t, []string{"kube_service:myendpoint", "kube_namespace:default", "kube_endpoint_ip:10.0.0.2"}, tags)

	endpts = processEndpoint(kendpt, false)
	assert.Equal(t, integration.After, endpts[0].GetCreationTime())
	assert.Equal(t, integration.After, endpts[1].GetCreationTime())
}

func TestEndpointsDiffer(t *testing.T) {
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
							{IP: "10.0.0.1", Hostname: "testhost1"},
							{IP: "10.0.0.2", Hostname: "testhost2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "testport1", Port: 123},
							{Name: "testport2", Port: 126},
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
							{IP: "10.0.0.1", Hostname: "testhost1"},
							{IP: "10.0.0.2", Hostname: "testhost2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "testport1", Port: 123},
							{Name: "testport2", Port: 126},
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
							{IP: "10.0.0.1", Hostname: "testhost1"},
							{IP: "10.0.0.2", Hostname: "testhost2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "testport1", Port: 123},
							{Name: "testport2", Port: 126},
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
							{IP: "10.0.0.1", Hostname: "testhost1"},
							{IP: "10.0.0.3", Hostname: "testhost2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "testport1", Port: 123},
							{Name: "testport2", Port: 126},
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
							{IP: "10.0.0.1", Hostname: "testhost1"},
							{IP: "10.0.0.2", Hostname: "testhost2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "testport1", Port: 123},
							{Name: "testport2", Port: 126},
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
							{IP: "10.0.0.1", Hostname: "testhost1"},
							{IP: "10.0.0.3", Hostname: "testhost3"},
						},
						Ports: []v1.EndpointPort{
							{Name: "testport1", Port: 123},
							{Name: "testport2", Port: 126},
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
							{IP: "10.0.0.1", Hostname: "testhost1"},
							{IP: "10.0.0.2", Hostname: "testhost2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "testport1", Port: 123},
							{Name: "testport2", Port: 126},
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
							{IP: "10.0.0.1", Hostname: "testhost1"},
							{IP: "10.0.0.2", Hostname: "testhost2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "testport1", Port: 123},
							{Name: "testport2", Port: 124},
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
							{IP: "10.0.0.1", Hostname: "testhost1"},
							{IP: "10.0.0.2", Hostname: "testhost2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "testport1", Port: 123},
							{Name: "testport2", Port: 124},
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
							{IP: "10.0.0.1", Hostname: "testhost1"},
						},
						Ports: []v1.EndpointPort{
							{Name: "testport1", Port: 123},
							{Name: "testport2", Port: 124},
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
							{IP: "10.0.0.1", Hostname: "testhost1"},
							{IP: "10.0.0.2", Hostname: "testhost2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "testport1", Port: 123},
							{Name: "testport2", Port: 124},
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
							{IP: "10.0.0.1", Hostname: "testhost1"},
							{IP: "10.0.0.2", Hostname: "testhost2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "testport1", Port: 123},
						},
					},
				},
			},
			result: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.result, endpointsDiffer(tc.first, tc.second))
		})
	}
}

func TestDiffEndpoints(t *testing.T) {
	for name, tc := range map[string]struct {
		current, old, new, removed []*KubeEndpointService
	}{
		"one added endpoint": {
			current: []*KubeEndpointService{
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.1",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.1"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.1",
					},
				},
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.2",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.2"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.2",
					},
				},
			},
			old: []*KubeEndpointService{
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.1",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.1"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.1",
					},
				},
			},
			new: []*KubeEndpointService{
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.2",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.2"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.2",
					},
				},
			},
			removed: nil,
		},
		"one removed endpoint": {
			current: []*KubeEndpointService{
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.1",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.1"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.1",
					},
				},
			},
			old: []*KubeEndpointService{
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.1",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.1"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.1",
					},
				},
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.2",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.2"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.2",
					},
				},
			},
			new: nil,
			removed: []*KubeEndpointService{
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.2",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.2"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.2",
					},
				},
			},
		},
		"one removed endpoint and one added endpoint": {
			current: []*KubeEndpointService{
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.1",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.1"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.1",
					},
				},
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.3",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.3"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.3",
					},
				},
			},
			old: []*KubeEndpointService{
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.1",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.1"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.1",
					},
				},
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.2",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.2"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.2",
					},
				},
			},
			new: []*KubeEndpointService{
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.3",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.3"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.3",
					},
				},
			},
			removed: []*KubeEndpointService{
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.2",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.2"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.2",
					},
				},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			new, removed := diffEndpoints(tc.current, tc.old)
			assert.Equal(t, tc.new, new)
			assert.Equal(t, tc.removed, removed)
		})
	}
}

func TestContainsEndpointService(t *testing.T) {
	for name, tc := range map[string]struct {
		endptsSvcs []*KubeEndpointService
		endptSvc   *KubeEndpointService
		result     bool
	}{
		"contained": {
			endptsSvcs: []*KubeEndpointService{
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.1",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.1"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.1",
					},
				},
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.2",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.2"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.2",
					},
				},
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.3",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.3"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.3",
					},
				},
			},
			endptSvc: &KubeEndpointService{
				entity:       "kube_endpoint://default/myendpoint/10.0.0.3",
				creationTime: integration.After,
				hosts:        map[string]string{"service": "10.0.0.3"},
				ports:        []ContainerPort{{123, "testport1"}},
				tags: []string{
					"kube_service:myendpoint",
					"kube_namespace:default",
					"kube_endpoint_ip:10.0.0.3",
				},
			},
			result: true,
		},
		"not contained: same hostname and different address": {
			endptsSvcs: []*KubeEndpointService{
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.1",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.1"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.1",
					},
				},
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.2",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.2"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.3",
					},
				},
				{
					entity:       "kube_endpoint://default/myendpoint/10.0.0.3",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.3"},
					ports:        []ContainerPort{{123, "testport1"}},
					tags: []string{
						"kube_service:myendpoint",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.3",
					},
				},
			},
			endptSvc: &KubeEndpointService{
				entity:       "kube_endpoint://default/myendpoint/10.0.0.4",
				creationTime: integration.After,
				hosts:        map[string]string{"service": "10.0.0.4"},
				ports:        []ContainerPort{{123, "testport1"}},
				tags: []string{
					"kube_service:myendpoint",
					"kube_namespace:default",
					"kube_endpoint_ip:10.0.0.4",
				},
			},
			result: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.result, containsEndpointService(tc.endptsSvcs, tc.endptSvc))
		})
	}
}

func TestCreateEndpoint(t *testing.T) {
	for name, tc := range map[string]struct {
		ksvc           *v1.Service
		kendpt         *v1.Endpoints
		expectedSvcs   []*KubeServiceService
		expectedEndpts []*KubeEndpointService
	}{
		"nominal case": {
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("test"),
					Namespace:       "default",
					Name:            "myservice",
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":    "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs":   "[{}]",
						"ad.datadoghq.com/service.instances":      "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"ad.datadoghq.com/endpoints.check_names":  "[\"etcd\"]",
						"ad.datadoghq.com/endpoints.init_configs": "[{}]",
						"ad.datadoghq.com/endpoints.instances":    "[{\"use_preview\": \"true\", \"prometheus_url\": \"http://%%host%%:2379/metrics\"}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 126},
					},
				},
			},
			kendpt: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Name:            "myservice",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "testhost1"},
							{IP: "10.0.0.2", Hostname: "testhost2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "testport1", Port: 123},
							{Name: "testport2", Port: 124},
						},
					},
				},
			},
			expectedSvcs: []*KubeServiceService{
				{
					entity:       "kube_service://test",
					creationTime: integration.Before,
					hosts:        map[string]string{"cluster": "10.0.0.1"},
					ports:        []ContainerPort{{126, "test1"}},
					tags: []string{
						"kube_service:myservice",
						"kube_namespace:default",
					},
				},
			},
			expectedEndpts: []*KubeEndpointService{
				{
					entity:       "kube_endpoint://default/myservice/10.0.0.1",
					creationTime: integration.Before,
					hosts:        map[string]string{"service": "10.0.0.1"},
					ports:        []ContainerPort{{123, "testport1"}, {124, "testport2"}},
					tags: []string{
						"kube_service:myservice",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.1",
					},
					adID: "kube_endpoint://default/myservice",
				},
				{
					entity:       "kube_endpoint://default/myservice/10.0.0.2",
					creationTime: integration.Before,
					hosts:        map[string]string{"service": "10.0.0.2"},
					ports:        []ContainerPort{{123, "testport1"}, {124, "testport2"}},
					tags: []string{
						"kube_service:myservice",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.2",
					},
					adID: "kube_endpoint://default/myservice",
				},
			},
		},
		"add endpoints annotations to an existing service": {
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("test"),
					Namespace:       "default",
					Name:            "myservice",
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 126},
					},
				},
			},
			kendpt: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Name:            "myservice",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "testhost1"},
							{IP: "10.0.0.2", Hostname: "testhost2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "testport1", Port: 123},
							{Name: "testport2", Port: 124},
						},
					},
				},
			},
			expectedSvcs: []*KubeServiceService{
				{
					entity:       "kube_service://test",
					creationTime: integration.After,
					hosts:        map[string]string{"cluster": "10.0.0.1"},
					ports:        []ContainerPort{{126, "test1"}},
					tags: []string{
						"kube_service:myservice",
						"kube_namespace:default",
					},
				},
			},
			expectedEndpts: []*KubeEndpointService{
				{
					entity:       "kube_endpoint://default/myservice/10.0.0.1",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.1"},
					ports:        []ContainerPort{{123, "testport1"}, {124, "testport2"}},
					tags: []string{
						"kube_service:myservice",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.1",
					},
					adID: "kube_endpoint://default/myservice",
				},
				{
					entity:       "kube_endpoint://default/myservice/10.0.0.2",
					creationTime: integration.After,
					hosts:        map[string]string{"service": "10.0.0.2"},
					ports:        []ContainerPort{{123, "testport1"}, {124, "testport2"}},
					tags: []string{
						"kube_service:myservice",
						"kube_namespace:default",
						"kube_endpoint_ip:10.0.0.2",
					},
					adID: "kube_endpoint://default/myservice",
				},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := make(chan Service, 10)
			cr := make(chan Service, 10)
			listener := &KubeServiceListener{
				services:           make(map[types.UID]Service),
				endpoints:          make(map[string][]*KubeEndpointService),
				endpointsAnnotated: make(map[string]bool),
				newService:         c,
				delService:         cr,
				servicesInformer:   nil,
				endpointsInformer:  nil,
			}
			switch name {
			case "nominal case":
				svc := processService(tc.ksvc, true)
				listener.m.Lock()
				listener.services[tc.ksvc.UID] = svc
				listener.endpointsAnnotated[fmt.Sprintf("%s/%s", tc.ksvc.Namespace, tc.ksvc.Name)] = isEndpointAnnotated(tc.ksvc)
				listener.m.Unlock()
				listener.newService <- svc
				listener.createEndpoint(tc.kendpt, true)
				for i := 0; i < 3; i++ {
					select {
					case svc := <-c:
						switch i {
						case 0:
							assert.Equal(t, tc.expectedSvcs[0], svc)
						case 1:
							// endpts could be received in different order because listener.endooints is a map
							if svc.GetEntity() == "kube_endpoint://default/myservice/10.0.0.1" {
								assert.Equal(t, tc.expectedEndpts[0], svc)
							} else {
								assert.Equal(t, tc.expectedEndpts[1], svc)
							}
						case 2:
							// endpts could be received in different order because listener.endooints is a map
							if svc.GetEntity() == "kube_endpoint://default/myservice/10.0.0.1" {
								assert.Equal(t, tc.expectedEndpts[0], svc)
							} else {
								assert.Equal(t, tc.expectedEndpts[1], svc)
							}
						}
					case <-time.After(300 * time.Millisecond):
						assert.FailNow(t, "Timeout on receive channel")
					}
				}
			case "add endpoints annotations to an existing service":
				svc := processService(tc.ksvc, true)
				listener.m.Lock()
				listener.services[tc.ksvc.UID] = svc
				listener.endpointsAnnotated[fmt.Sprintf("%s/%s", tc.ksvc.Namespace, tc.ksvc.Name)] = isEndpointAnnotated(tc.ksvc)
				listener.m.Unlock()
				listener.newService <- svc
				// update service
				listener.removeService(tc.ksvc)
				newSvc := &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						ResourceVersion: "124",
						UID:             types.UID("test"),
						Namespace:       "default",
						Name:            "myservice",
						Annotations: map[string]string{
							"ad.datadoghq.com/service.check_names":    "[\"http_check\"]",
							"ad.datadoghq.com/service.init_configs":   "[{}]",
							"ad.datadoghq.com/service.instances":      "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
							"ad.datadoghq.com/endpoints.check_names":  "[\"etcd\"]",
							"ad.datadoghq.com/endpoints.init_configs": "[{}]",
							"ad.datadoghq.com/endpoints.instances":    "[{\"use_preview\": \"true\", \"prometheus_url\": \"http://%%host%%:2379/metrics\"}]",
						},
					},
					Spec: v1.ServiceSpec{
						ClusterIP: "10.0.0.1",
						Ports: []v1.ServicePort{
							{Name: "test1", Port: 126},
						},
					},
				}
				svc = processService(newSvc, false)
				listener.m.Lock()
				listener.services[newSvc.UID] = svc
				listener.endpointsAnnotated[fmt.Sprintf("%s/%s", newSvc.Namespace, newSvc.Name)] = isEndpointAnnotated(newSvc)
				listener.m.Unlock()
				listener.newService <- svc
				listener.addedEndpt(tc.kendpt)
				// listener.createEndpoint(tc.kendpt, true)
				for i := 0; i < 4; i++ {
					select {
					case svc := <-c:
						switch i {
						case 0:
							// receive the non endpoints annotated service
							continue
						case 1:
							assert.Equal(t, tc.expectedSvcs[0], svc)
						case 2:
							// endpts could be received in different order because listener.endooints is a map
							if svc.GetEntity() == "kube_endpoint://default/myservice/10.0.0.1" {
								assert.Equal(t, tc.expectedEndpts[0], svc)
							} else {
								assert.Equal(t, tc.expectedEndpts[1], svc)
							}
						case 3:
							// endpts could be received in different order because listener.endooints is a map
							if svc.GetEntity() == "kube_endpoint://default/myservice/10.0.0.1" {
								assert.Equal(t, tc.expectedEndpts[0], svc)
							} else {
								assert.Equal(t, tc.expectedEndpts[1], svc)
							}
						}
					case <-time.After(300 * time.Millisecond):
						assert.FailNow(t, "Timeout on receive channel")
					}
				}
			}
		})
	}
}
