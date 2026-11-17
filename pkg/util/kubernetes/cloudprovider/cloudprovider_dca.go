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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DCAGetName returns the name of the kube distribution for the current node.
func DCAGetName(ctx context.Context) string {
	name, _ := DCAGetDistribution(ctx)
	return name
}

// DCAGetDistribution is DCAGetName, and also reports whether the answer is final. It is not when
// the API server could not be reached (for example before WaitForAPIClient returns) or ctx was
// done: nothing is cached then, and a later call can still find the distribution.
func DCAGetDistribution(ctx context.Context) (string, bool) {
	cacheKey := cache.BuildAgentKey(constants.NodeKubeDistributionKey)
	if cloudProvider, found := cache.Cache.Get(cacheKey); found {
		return cloudProvider.(string), true
	}

	nodeName, err := apiserver.HostNodeName(ctx)
	if err != nil {
		log.Warnf("Unable to get node name from apiserver: %v", err)
		return "", false
	}

	nl, sysInfo := getNodeMeta(ctx, nodeName)
	if nl == nil && sysInfo == nil {
		return "", false
	}
	kubeletVersion := ""
	kernelVersion := ""
	if sysInfo != nil {
		kubeletVersion = sysInfo.KubeletVersion
		kernelVersion = sysInfo.KernelVersion
	}

	providerName := getKubeDistributionName(nl, kubeletVersion, kernelVersion)
	// It is fine to save empty tag to avoid querying API server over and over again.
	// Empty tag are ignored.
	cache.Cache.Set(cacheKey, providerName, cache.NoExpiration)
	return providerName, true
}

// getNodeMeta retrieves node labels and system info for provided nodeName in cluster agent.
func getNodeMeta(ctx context.Context, nodeName string) (map[string]string, *corev1.NodeSystemInfo) {
	cl, err := apiserver.GetAPIClient()
	if err != nil {
		log.Warnf("Unable to get apiserver: %v", err)
		return nil, nil
	}
	nodeCl := cl.Cl.CoreV1().Nodes()

	node, err := nodeCl.Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		log.Warnf("Unable to get self node: %v", err)
		return nil, nil
	}
	if node == nil {
		return nil, nil
	}
	return node.Labels, &node.Status.NodeInfo
}
