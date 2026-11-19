// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cloudprovider

import (
	"context"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DCAGetName returns the name of the kube distribution for the current node.
func DCAGetName(ctx context.Context) string {
	name, err := DCAGetDistribution(ctx)
	if err != nil {
		log.Warnf("Unable to get %v", err)
	}
	return name
}

// DCAGetDistribution is DCAGetName, and also returns why the distribution is not known: the API
// server could not be reached (for example before WaitForAPIClient returns), the self node
// could not be read (an RBAC Forbidden or a NotFound, which the error wraps), or ctx was done.
// Nothing is cached then, and a later call can still find the distribution. A nil error is a
// final answer. It does not log; DCAGetName does.
func DCAGetDistribution(ctx context.Context) (string, error) {
	cacheKey := cache.BuildAgentKey(constants.NodeKubeDistributionKey)
	if cloudProvider, found := cache.Cache.Get(cacheKey); found {
		return cloudProvider.(string), nil
	}

	nodeName, err := apiserver.HostNodeName(ctx)
	if err != nil {
		return "", fmt.Errorf("node name from apiserver: %w", err)
	}

	nl, sysInfo, err := getNodeMeta(ctx, nodeName)
	if err != nil {
		return "", err
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
	return providerName, nil
}

// getNodeMeta retrieves node labels and system info for provided nodeName in cluster agent.
func getNodeMeta(ctx context.Context, nodeName string) (map[string]string, *corev1.NodeSystemInfo, error) {
	cl, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, nil, fmt.Errorf("apiserver: %w", err)
	}
	nodeCl := cl.Cl.CoreV1().Nodes()

	node, err := nodeCl.Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("self node: %w", err)
	}
	if node == nil {
		return nil, nil, errors.New("self node: the API server returned no node")
	}
	return node.Labels, &node.Status.NodeInfo, nil
}
