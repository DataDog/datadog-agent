// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml && kubelet

package nvml

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

const migConfigStateLabel = "nvidia.com/mig.config.state"

func (c *collector) configureMigConfigStateProvider(ctx context.Context) {
	ddnvml.SetMigConfigStateProvider(func(ctx context.Context) (string, bool, error) {
		if c.store == nil {
			return "", false, nil
		}

		kubeUtil, err := kubelet.GetKubeUtil()
		if err != nil {
			return "", false, fmt.Errorf("failed to get kubelet util: %w", err)
		}

		nodeName, err := kubeUtil.GetNodename(ctx)
		if err != nil {
			return "", false, fmt.Errorf("failed to get kubelet node name: %w", err)
		}

		entityID := util.GenerateKubeMetadataEntityID("", "nodes", "", nodeName)
		metadata, err := c.store.GetKubernetesMetadata(entityID)
		if err != nil {
			return "", false, fmt.Errorf("failed to get node metadata for %s: %w", nodeName, err)
		}

		value, ok := metadata.Labels[migConfigStateLabel]
		return value, ok, nil
	})
}
