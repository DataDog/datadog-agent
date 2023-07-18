// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build kubelet

package hostinfo

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// GetNodeAnnotations returns node labels for this host
func GetNodeAnnotations(ctx context.Context) (map[string]string, error) {
	ku, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}

	nodeName, err := ku.GetNodename(ctx)
	if err != nil {
		return nil, err
	}

	if config.Datadog.GetBool("cluster_agent.enabled") {
		cl, err := clusteragent.GetClusterAgentClient()
		if err != nil {
			return nil, err
		}
		return cl.GetNodeAnnotations(nodeName)
	}
	return apiserverNodeAnnotations(ctx, nodeName)
}
