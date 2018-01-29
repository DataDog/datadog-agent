// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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
			ID:    "rkt://bar-random-hash",
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

	select {
	case service := <-services:
		assert.Equal(t, "docker://foorandomhash", string(service.GetID()))
		adIdentifiers, err := service.GetADIdentifiers()
		assert.Nil(t, err)
		assert.Equal(t, []string{"docker://foorandomhash", "datadoghq.com/foo:latest", "foo"}, adIdentifiers)
		hosts, err := service.GetHosts()
		assert.Nil(t, err)
		assert.Equal(t, map[string]string{"pod": "127.0.0.1"}, hosts)
		ports, err := service.GetPorts()
		assert.Nil(t, err)
		assert.Equal(t, []int{1337, 1339}, ports)
		_, err = service.GetPid()
		assert.Equal(t, ErrNotSupported, err)
	default:
		t.FailNow()
	}

	select {
	case service := <-services:
		assert.Equal(t, "rkt://bar-random-hash", string(service.GetID()))
		adIdentifiers, err := service.GetADIdentifiers()
		assert.Nil(t, err)
		assert.Equal(t, []string{"rkt://bar-random-hash", "datadoghq.com/bar:latest", "bar"}, adIdentifiers)
		hosts, err := service.GetHosts()
		assert.Nil(t, err)
		assert.Equal(t, map[string]string{"pod": "127.0.0.1"}, hosts)
		ports, err := service.GetPorts()
		assert.Nil(t, err)
		assert.Equal(t, []int{1122}, ports)
		_, err = service.GetPid()
		assert.Equal(t, ErrNotSupported, err)
	default:
		t.FailNow()
	}
}
