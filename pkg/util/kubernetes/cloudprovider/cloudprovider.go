// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudprovider

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
)

// GetName returns the name of the kube distribution for the current node.
// GetName shouldn't be used on DCA. For DCA please refer to DCAGetName.
func GetName(ctx context.Context) (string, error) {
	cacheKey := cache.BuildAgentKey(constants.NodeKubeDistributionKey)
	if cloudProvider, found := cache.Cache.Get(cacheKey); found {
		return cloudProvider.(string), nil
	}

	ni, err := hostinfo.NewNodeInfo()
	if err != nil {
		return "", err
	}

	nodeName, err := ni.GetNodeName(ctx)
	if err != nil {
		return "", err
	}

	dcaClient, err := clusteragent.GetClusterAgentClient()
	if err != nil {
		return "", err
	}

	nl, err := dcaClient.GetNodeLabels(nodeName)
	if err != nil {
		return "", err
	}

	kubeDistro := getKubeDistributionNameFromNodeLabels(nl)

	cache.Cache.Set(cacheKey, kubeDistro, cache.NoExpiration)
	return kubeDistro, nil
}

// getKubeDistributionNameFromNodeLabels checks certain node labels to determine the kube cloud provider.
// Returns an empty string if no provider is determined.
func getKubeDistributionNameFromNodeLabels(nl map[string]string) string {
	for labelName := range nl {
		switch labelName {
		case "eks.amazonaws.com/nodegroup",
			"eks.amazonaws.com/compute-type",
			"alpha.eksctl.io/cluster-name":
			return "eks"
		case "cloud.google.com/gke-boot-disk":
			return "gke"
		case "kubernetes.azure.com/nodepool-type",
			"kubernetes.azure.com/mode",
			"kubernetes.azure.com/cluster":
			return "aks"
		}
	}
	return ""
}
