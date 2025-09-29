// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cloudprovider

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DCAGetName returns the name of the cloud provider for the current node.
func DCAGetName(ctx context.Context) string {
	cacheKey := cache.BuildAgentKey(constants.NodeCloudProviderKey)
	if cloudProvider, found := cache.Cache.Get(cacheKey); found {
		return cloudProvider.(string)
	}

	nodeName, err := apiserver.HostNodeName(ctx)
	if err != nil {
		log.Warnf("Unable to get node name from apiserver: %v", err)
		return ""
	}

	nl := getNodeLabels(ctx, nodeName)
	providerName := getProvideNameFromNodeLabels(nl)

	// It is fine to save empty tag to avoid querying API server over and over again.
	// Empty tag are ignored.
	cache.Cache.Set(cacheKey, providerName, cache.NoExpiration)
	return providerName
}

// getNodeLabels retrieves node labels for provided nodeName in cluster agent.
func getNodeLabels(ctx context.Context, nodeName string) map[string]string {
	cl, err := apiserver.GetAPIClient()
	if err != nil {
		log.Warnf("Unable to get apiserver: %v", err)
		return nil
	}
	nodeCl := cl.Cl.CoreV1().Nodes()

	node, err := nodeCl.Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		log.Warnf("Unable to get self node: %v", err)
		return nil
	}
	if node == nil {
		return nil
	}
	return node.Labels
}
