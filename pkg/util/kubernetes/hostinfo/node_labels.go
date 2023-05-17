// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package hostinfo

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// NodeInfo is use to get Kubernetes Node metadata information
type NodeInfo struct {
	// client use to get NodeName from the "/pods" kubelet api.
	client kubelet.KubeUtilInterface
	// getClusterAgentFunc get Cluster-Agent client to get Node Labels with the Cluster-Agent api.
	getClusterAgentFunc func() (clusteragent.DCAClientInterface, error)
	// apiserverNodeLabelsFunc get Node Labels from the API server directly
	apiserverNodeLabelsFunc func(ctx context.Context, nodeName string) (map[string]string, error)
}

// NewNodeInfo return a new NodeInfo instance
// return an error if it fails to access the kubelet client.
func NewNodeInfo() (*NodeInfo, error) {
	ku, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}

	nodeInfo := &NodeInfo{
		client:                  ku,
		getClusterAgentFunc:     clusteragent.GetClusterAgentClient,
		apiserverNodeLabelsFunc: apiserverNodeLabels,
	}

	return nodeInfo, nil
}

// GetNodeLabels returns node labels for this host
func (n *NodeInfo) GetNodeLabels(ctx context.Context) (map[string]string, error) {
	nodeName, err := n.client.GetNodename(ctx)
	if err != nil {
		return nil, err
	}

	if config.Datadog.GetBool("cluster_agent.enabled") {
		cl, err := n.getClusterAgentFunc()
		if err != nil {
			return nil, err
		}
		return cl.GetNodeLabels(nodeName)
	}
	return n.apiserverNodeLabelsFunc(ctx, nodeName)
}
