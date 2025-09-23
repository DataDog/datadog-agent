// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudprovider

import (
	"context"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
)

// GetName returns the name of the cloud provider for the current node.
// GetName shouldn't be used on DCA. For DCA please refer to DCAGetName.
func GetName() (string, error) {
	cacheKey := cache.BuildAgentKey(constants.NodeCloudProviderKey)
	if cloudProvider, found := cache.Cache.Get(cacheKey); found {
		return cloudProvider.(string), nil
	}

	ni, err := hostinfo.NewNodeInfo()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithDeadline(context.TODO(), time.Now().Add(2*time.Second))
	defer cancel()

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

	cloudProvider := getProvideNameFromNodeLabels(nl)

	cache.Cache.Set(cacheKey, cloudProvider, cache.NoExpiration)
	return cloudProvider, nil
}

// getProvideNameFromNodeLabels checks certain node labels to determine the kube cloud provider.
// Returns an empty string if no provider is determined.
func getProvideNameFromNodeLabels(nl map[string]string) string {
	cloudProvider := ""
out:
	for labelName, labelValue := range nl {
		switch labelName {
		case "topology.k8s.aws/zone-id",
			"eks.amazonaws.com/nodegroup",
			"eks.amazonaws.com/compute-type",
			"alpha.eksctl.io/cluster-name":
			cloudProvider = "aws"
			break out
		case "topology.gke.io/zone",
			"cloud.google.com/gke-boot-disk":
			cloudProvider = "gcp"
			break out
		case "kubernetes.azure.com/nodepool-type",
			"kubernetes.azure.com/mode":
			cloudProvider = "azure"
			break out
		case "kubernetes.io/hostname":
			if strings.HasPrefix(labelValue, "aks") {
				cloudProvider = "azure"
				break out
			}
		}
	}
	return cloudProvider
}
