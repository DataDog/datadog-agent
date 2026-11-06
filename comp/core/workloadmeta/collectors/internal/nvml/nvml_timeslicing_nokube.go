// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && !kubelet

package nvml

import "context"

// fetchNodeGPULabels reads no labels without kubelet access: the node's
// labels are read through the kubelet's view of the node name, the cluster
// agent or the API server.
func (c *collector) fetchNodeGPULabels(_ context.Context) (map[string]string, error) {
	return nil, nil
}

// getKubeletDeviceIDs returns no devices without kubelet access.
func (c *collector) getKubeletDeviceIDs(_ context.Context) ([]string, error) {
	return nil, nil
}
