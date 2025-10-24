// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml && kubelet

package nvml

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

func (c *collector) fillUnhealthyDevices(ctx context.Context, out map[string]struct{}) error {
	// for now support only kubernetes as a source of truth for devices health
	// todo(jasondellaluce): find some good heuristics for other kinds of deployment too
	if !env.IsFeaturePresent(env.KubernetesDevicePlugins) {
		return nil
	}

	kubeUtil, err := kubelet.GetKubeUtil()
	if err != nil {
		return fmt.Errorf("failed to get kubelet utils : %w", err)
	}

	kubeDevices, err := kubeUtil.GetDevicesList(ctx)
	if err != nil {
		return fmt.Errorf("failed getting kubelet devices list: %w", err)
	}

	for _, kDev := range kubeDevices {
		if !kDev.Healthy {
			out[kDev.ID] = struct{}{}
		}
	}

	return nil
}
