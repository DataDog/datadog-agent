// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks
// +build kubeapiserver

package listeners

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
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

	eps := processEndpoints(kep, true)

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

	assert.Equal(t, "kube_endpoint_uid://default/myservice/10.0.0.1", eps[0].GetEntity())
	assert.Equal(t, integration.Before, eps[0].GetCreationTime())

	adID, err := eps[0].GetADIdentifiers()
	assert.NoError(t, err)
	assert.Equal(t, []string{"kube_endpoint_uid://default/myservice/10.0.0.1"}, adID)

	hosts, err := eps[0].GetHosts()
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"endpoint": "10.0.0.1"}, hosts)

	ports, err := eps[0].GetPorts()
	assert.NoError(t, err)
	assert.Equal(t, []ContainerPort{{123, "port123"}, {126, "port126"}}, ports)

	tags, err := eps[0].GetTags()
	assert.NoError(t, err)
	assert.Equal(t, []string{"kube_service:myservice", "kube_namespace:default", "kube_endpoint_ip:10.0.0.1"}, tags)

	assert.Equal(t, "kube_endpoint_uid://default/myservice/10.0.0.2", eps[1].GetEntity())
	assert.Equal(t, integration.Before, eps[1].GetCreationTime())

	adID, err = eps[1].GetADIdentifiers()
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
	assert.Equal(t, []string{"kube_service:myservice", "kube_namespace:default", "kube_endpoint_ip:10.0.0.2"}, tags)

	eps = processEndpoints(kep, false)
	assert.Equal(t, integration.After, eps[0].GetCreationTime())
	assert.Equal(t, integration.After, eps[1].GetCreationTime())
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
