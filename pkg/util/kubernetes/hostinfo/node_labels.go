// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubelet

package hostinfo

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// GetNodeLabels returns node labels for this host
func GetNodeLabels() (map[string]string, error) {
	ku, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}

	nodeName, err := ku.GetNodename()
	if err != nil {
		return nil, err
	}

	if config.Datadog.GetBool("cluster_agent.enabled") {
		cl, err := clusteragent.GetClusterAgentClient()
		if err != nil {
			return nil, err
		}
		return cl.GetNodeLabels(nodeName)
	}
	return apiserverNodeLabels(nodeName)
}

// GetNodeClusterNameLabel returns clustername by fetching a node label
func GetNodeClusterNameLabel() (string, error) {
	nodeLabels, err := GetNodeLabels()
	if err != nil {
		return "", err
	}

	clusterNameLabels := []string{
		"alpha.eksctl.io/cluster-name", // EKS cluster-name label
	}

	for _, l := range clusterNameLabels {
		if v, ok := nodeLabels[l]; ok {
			return v, nil
		}
	}
	return "", nil
}
