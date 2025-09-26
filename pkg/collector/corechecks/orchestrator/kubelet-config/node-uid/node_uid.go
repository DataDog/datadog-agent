// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build kubelet

package kubelet_config

import (
	"context"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// GetNodeUID returns the UID of the node
func GetNodeUID(ctx context.Context, nodeName string) (string, error) {
	ku, err := kubelet.GetKubeUtil()
	if err != nil {
		return "", err
	}

	if pkgconfigsetup.Datadog().GetBool("cluster_agent.enabled") {
		cl, err := clusteragent.GetClusterAgentClient()
		if err != nil {
			return "", err
		}
		return cl.GetNodeUID(nodeName)
	}
	return apiserverNodeUID(ctx, nodeName)
}
