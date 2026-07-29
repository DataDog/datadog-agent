// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package kubernetes

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateKindConfigExtraMounts(t *testing.T) {
	config, err := generateKindConfig(kindClusterConfig, KindConfigFlags{
		ExtraMounts: []KindExtraMount{
			{
				HostPath:      "/etc/machine-id",
				ContainerPath: "/host/etc/machine-id",
				ReadOnly:      true,
			},
			{
				HostPath:      "/run/dbus/system_bus_socket",
				ContainerPath: "/host/run/dbus/system_bus_socket",
				ReadOnly:      true,
			},
		},
		WorkerNodes: []KindWorkerNode{{}},
	})
	require.NoError(t, err)

	nodes := strings.Split(config, "- role:")
	require.Len(t, nodes, 3)

	expectedMounts := []string{
		"  - hostPath: \"/etc/machine-id\"\n    containerPath: \"/host/etc/machine-id\"\n    readOnly: true",
		"  - hostPath: \"/run/dbus/system_bus_socket\"\n    containerPath: \"/host/run/dbus/system_bus_socket\"\n    readOnly: true",
	}
	for nodeIndex, node := range nodes[1:] {
		for _, expectedMount := range expectedMounts {
			assert.Contains(t, node, expectedMount, "mount missing from node %d", nodeIndex)
		}
	}
}
