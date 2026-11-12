// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && kubelet

package nvml

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// fetchNodeGPULabels reads this node's labels from the cluster agent or
// the API server.
func (c *collector) fetchNodeGPULabels(ctx context.Context) (map[string]string, error) {
	ni, err := hostinfo.NewNodeInfo()
	if err != nil {
		return nil, err
	}
	return ni.GetNodeLabels(ctx)
}

// getKubeletDeviceIDs returns the IDs of every device registered to the
// kubelet on this node, across plugins.
func (c *collector) getKubeletDeviceIDs(ctx context.Context) ([]string, error) {
	// for now support only kubernetes as a source of truth for devices
	if !env.IsFeaturePresent(env.KubernetesDevicePlugins) {
		return nil, nil
	}

	kubeUtil, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}

	kubeDevices, err := kubeUtil.GetDevicesList(ctx)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(kubeDevices))
	for _, dev := range kubeDevices {
		ids = append(ids, dev.ID)
	}
	return ids, nil
}
