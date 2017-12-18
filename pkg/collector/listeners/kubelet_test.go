// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubelet

package listeners

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/stretchr/testify/assert"
)

func getMockedPod() *kubelet.Pod {
	containerSpecs := []kubelet.ContainerSpec{
		{
			Name:  "foo",
			Image: "datadoghq.com/foo:latest",
			Ports: []kubelet.ContainerPortSpec{
				{
					ContainerPort: 1337,
					HostPort:      1338,
					Name:          "footcpport",
					Protocol:      "TCP",
				},
				{
					ContainerPort: 1339,
					HostPort:      1340,
					Name:          "fooudpport",
					Protocol:      "UDP",
				},
			},
		},
		{
			Name:  "bar",
			Image: "datadoghq.com/bar:latest",
			Ports: []kubelet.ContainerPortSpec{
				{
					ContainerPort: 1122,
					HostPort:      1133,
					Name:          "barport",
					Protocol:      "TCP",
				},
			},
		},
	}
	kubeletSpec := kubelet.Spec{
		HostNetwork: false,
		Hostname:    "mock",
		NodeName:    "mockn-node",
		Containers:  containerSpecs,
	}
	containerStatuses := []kubelet.ContainerStatus{
		{
			Name:  "foo",
			Image: "datadoghq.com/foo:latest",
			ID:    "docker://foorandomhash",
		},
		{
			Name:  "bar",
			Image: "datadoghq.com/bar:latest",
			ID:    "docker://barrandomhash",
		},
	}
	kubeletStatus := kubelet.Status{
		Phase:      "Running",
		PodIP:      "127.0.0.1",
		HostIP:     "127.0.0.2",
		Containers: containerStatuses,
	}
	return &kubelet.Pod{
		Spec:   kubeletSpec,
		Status: kubeletStatus,
		Metadata: kubelet.PodMetadata{
			Name: "mock-pod",
		},
	}
}

func TestProcessNewPod(t *testing.T) {
	services := make(chan Service, 2)
	listener := KubeletListener{
		newService: services,
		services:   make(map[ID]Service),
	}
	listener.processNewPod(getMockedPod())

	service := <-services
	assert.Equal(t, "docker://foorandomhash", string(service.GetID()))
	adIdentifiers, err := service.GetADIdentifiers()
	assert.Nil(t, err)
	assert.Equal(t, []string{"foo", "datadoghq.com/foo:latest"}, adIdentifiers)
	hosts, err := service.GetHosts()
	assert.Nil(t, err)
	assert.Equal(t, map[string]string{"pod": "127.0.0.1"}, hosts)
	ports, err := service.GetPorts()
	assert.Nil(t, err)
	assert.Equal(t, []int{1337, 1339}, ports)
	_, err = service.GetPid()
	assert.Equal(t, ErrNotSupported, err)

	service = <-services
	assert.Equal(t, "docker://barrandomhash", string(service.GetID()))
	adIdentifiers, err = service.GetADIdentifiers()
	assert.Nil(t, err)
	assert.Equal(t, []string{"bar", "datadoghq.com/bar:latest"}, adIdentifiers)
	hosts, err = service.GetHosts()
	assert.Nil(t, err)
	assert.Equal(t, map[string]string{"pod": "127.0.0.1"}, hosts)
	ports, err = service.GetPorts()
	assert.Nil(t, err)
	assert.Equal(t, []int{1122}, ports)
	_, err = service.GetPid()
	assert.Equal(t, ErrNotSupported, err)
}

func TestGetKubeletADIdentifiers(t *testing.T) {
	pod := PodContainerService{
		ID: "docker://foobarrandomhash",
		PodInfos: &kubelet.Pod{
			Status: kubelet.Status{
				Containers: []kubelet.ContainerStatus{
					{
						Name:  "foo",
						Image: "datadoghq.com/foo:latest",
						ID:    "docker://foorandomhash",
					},
					{
						Name:  "bar",
						Image: "datadoghq.com/bar:latest",
						ID:    "docker://barrandomhash",
					},
					{
						Name:  "foobar",
						Image: "datadoghq.com/foobar:latest",
						ID:    "docker://foobarrandomhash",
					},
				},
			},
		},
	}
	identifiers, err := pod.GetADIdentifiers()
	assert.Nil(t, err)
	assert.Equal(t, []string{"foobar", "datadoghq.com/foobar:latest"}, identifiers)
}

func TestGetKubeletHosts(t *testing.T) {
	pod := PodContainerService{
		PodInfos: &kubelet.Pod{
			Status: kubelet.Status{
				PodIP: "1.2.3.4",
			},
		},
	}
	hosts, err := pod.GetHosts()
	assert.Nil(t, err)
	assert.Equal(t, map[string]string{"pod": "1.2.3.4"}, hosts)
}

func TestGetKubeletPorts(t *testing.T) {
	pod := PodContainerService{
		ID: "docker://foobarrandomhash",
		PodInfos: &kubelet.Pod{
			Status: kubelet.Status{
				Containers: []kubelet.ContainerStatus{
					{
						Name:  "foo",
						Image: "datadoghq.com/foo:latest",
						ID:    "docker://foorandomhash",
					},
					{
						Name:  "bar",
						Image: "datadoghq.com/bar:latest",
						ID:    "docker://barrandomhash",
					},
					{
						Name:  "foobar",
						Image: "datadoghq.com/foobar:latest",
						ID:    "docker://foobarrandomhash",
					},
				},
			},
			Spec: kubelet.Spec{
				Containers: []kubelet.ContainerSpec{
					{
						Name:  "foo",
						Image: "datadoghq.com/foo:latest",
						Ports: []kubelet.ContainerPortSpec{
							{
								ContainerPort: 1111,
								HostPort:      1111,
								Name:          "fooport",
								Protocol:      "UDP",
							},
						},
					},
					{
						Name:  "bar",
						Image: "datadoghq.com/bar:latest",
						Ports: []kubelet.ContainerPortSpec{
							{
								ContainerPort: 2222,
								HostPort:      2222,
								Name:          "barport",
								Protocol:      "UDP",
							},
						},
					},
					{
						Name:  "foobar",
						Image: "datadoghq.com/foo:latest",
						Ports: []kubelet.ContainerPortSpec{
							{
								ContainerPort: 1337,
								HostPort:      1337,
								Name:          "foobarport",
								Protocol:      "TCP",
							},
						},
					},
				},
			},
		},
	}
	ports, err := pod.GetPorts()
	assert.Nil(t, err)
	assert.Equal(t, []int{1337}, ports)
}
