// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestWorkloadmetaResolvable_PodOnly(t *testing.T) {
	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "pod-uid-123",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		IP: "10.244.0.15",
	}

	adapter := newResolvableAdapter(pod, nil)

	t.Run("GetServiceID", func(t *testing.T) {
		serviceID := adapter.GetServiceID()
		assert.Contains(t, serviceID, "pod-uid-123")
	})

	t.Run("GetHosts", func(t *testing.T) {
		hosts, err := adapter.GetHosts()
		require.NoError(t, err)
		assert.Equal(t, "10.244.0.15", hosts["pod"])
	})

	t.Run("GetHostname", func(t *testing.T) {
		_, err := adapter.GetHostname()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no container available")
	})

	t.Run("GetExtraConfig", func(t *testing.T) {
		namespace, err := adapter.GetExtraConfig("namespace")
		require.NoError(t, err)
		assert.Equal(t, "default", namespace)

		podName, err := adapter.GetExtraConfig("pod_name")
		require.NoError(t, err)
		assert.Equal(t, "test-pod", podName)

		podUID, err := adapter.GetExtraConfig("pod_uid")
		require.NoError(t, err)
		assert.Equal(t, "pod-uid-123", podUID)

		// Unknown key
		_, err = adapter.GetExtraConfig("unknown")
		assert.Error(t, err)
	})

	t.Run("GetPorts", func(t *testing.T) {
		_, err := adapter.GetPorts()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no container available")
	})

	t.Run("GetPid", func(t *testing.T) {
		_, err := adapter.GetPid()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pid not available")
	})
}

func TestWorkloadmetaResolvable_ContainerOnly(t *testing.T) {
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "container-123",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "nginx",
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
		NetworkIPs: map[string]string{
			"bridge": "172.17.0.5",
			"custom": "192.168.1.10",
		},
		Ports: []workloadmeta.ContainerPort{
			{Port: 80, Name: "http"},
			{Port: 443, Name: "https"},
		},
		PID:      1234,
		Hostname: "nginx-container",
	}

	adapter := newResolvableAdapter(nil, container)

	t.Run("GetServiceID", func(t *testing.T) {
		serviceID := adapter.GetServiceID()
		assert.Contains(t, serviceID, "container-123")
	})

	t.Run("GetHosts", func(t *testing.T) {
		hosts, err := adapter.GetHosts()
		require.NoError(t, err)
		assert.Equal(t, "172.17.0.5", hosts["bridge"])
		assert.Equal(t, "192.168.1.10", hosts["custom"])
	})

	t.Run("GetPorts", func(t *testing.T) {
		ports, err := adapter.GetPorts()
		require.NoError(t, err)
		require.Len(t, ports, 2)
		assert.Equal(t, 80, ports[0].Port)
		assert.Equal(t, "http", ports[0].Name)
		assert.Equal(t, 443, ports[1].Port)
		assert.Equal(t, "https", ports[1].Name)
	})

	t.Run("GetPid", func(t *testing.T) {
		pid, err := adapter.GetPid()
		require.NoError(t, err)
		assert.Equal(t, 1234, pid)
	})

	t.Run("GetHostname", func(t *testing.T) {
		hostname, err := adapter.GetHostname()
		require.NoError(t, err)
		assert.Equal(t, "nginx-container", hostname)
	})

	t.Run("GetExtraConfig without pod", func(t *testing.T) {
		_, err := adapter.GetExtraConfig("namespace")
		assert.Error(t, err)
	})
}

func TestWorkloadmetaResolvable_ContainerWithPod(t *testing.T) {
	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "pod-uid-456",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "my-pod",
			Namespace: "production",
		},
		IP: "10.244.0.20",
	}

	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "container-456",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "app",
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
		NetworkIPs: map[string]string{
			"bridge": "172.17.0.10",
		},
		Ports: []workloadmeta.ContainerPort{
			{Port: 8080, Name: "http"},
		},
		PID:      5678,
		Hostname: "app-container",
	}

	adapter := newResolvableAdapter(pod, container)

	t.Run("GetHosts includes both container and pod IPs", func(t *testing.T) {
		hosts, err := adapter.GetHosts()
		require.NoError(t, err)
		// Should have container networks plus pod IP
		assert.Contains(t, hosts, "bridge")
		assert.Contains(t, hosts, "pod")
		assert.Equal(t, "10.244.0.20", hosts["pod"])
	})

	t.Run("GetExtraConfig with pod context", func(t *testing.T) {
		namespace, err := adapter.GetExtraConfig("namespace")
		require.NoError(t, err)
		assert.Equal(t, "production", namespace)

		podName, err := adapter.GetExtraConfig("pod_name")
		require.NoError(t, err)
		assert.Equal(t, "my-pod", podName)

		podUID, err := adapter.GetExtraConfig("pod_uid")
		require.NoError(t, err)
		assert.Equal(t, "pod-uid-456", podUID)
	})

	t.Run("Container methods still work with pod context", func(t *testing.T) {
		ports, err := adapter.GetPorts()
		require.NoError(t, err)
		assert.Len(t, ports, 1)
		assert.Equal(t, 8080, ports[0].Port)

		pid, err := adapter.GetPid()
		require.NoError(t, err)
		assert.Equal(t, 5678, pid)

		hostname, err := adapter.GetHostname()
		require.NoError(t, err)
		assert.Equal(t, "app-container", hostname)
	})
}
