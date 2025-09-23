// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudprovider

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
)

// ErrCloudProviderIndetermined is returned when a function is unable to determine the cloud provider
var ErrCloudProviderIndetermined = errors.New("node cloud provider indetermined")

// GetName returns the name of the cloud provider for the current node
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

	nl, err := ni.GetNodeLabels(ctx)
	if err != nil {
		return "", err
	}
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

	// If somehow node is missing those labels, try to get cloud provider from nodeinfo
	if cloudProvider == "" {
		// todo(dp): if this error is hit implement nodeInfo getters
		return "", ErrCloudProviderIndetermined
	}

	cache.Cache.Set(cacheKey, cloudProvider, cache.NoExpiration)
	return cloudProvider, nil
}
