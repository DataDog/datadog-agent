// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet
// +build kubelet

package hostinfo

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

const (
	eksClusterNameLabelKey       = "alpha.eksctl.io/cluster-name"
	datadogADClusterNameLabelKey = "ad.datadoghq.com/cluster-name"
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

// clusterNameLabelType this struct is used to attach to the label key the information if this label should
// override cluster-name previously detected.
type clusterNameLabelType struct {
	key string
	// shouldOverride if set to true override previous cluster-name
	shouldOverride bool
}

var (
	// We use a slice to define the default Node label key to keep the ordering
	defaultClusterNameLabelKeyConfigs = []clusterNameLabelType{
		{key: eksClusterNameLabelKey, shouldOverride: false},
		{key: datadogADClusterNameLabelKey, shouldOverride: true},
	}
)

// GetNodeClusterNameLabel returns clustername by fetching a node label
func (n *NodeInfo) GetNodeClusterNameLabel(ctx context.Context, clusterName string) (string, error) {
	nodeLabels, err := n.GetNodeLabels(ctx)
	if err != nil {
		return "", err
	}

	var clusterNameLabelKeys []clusterNameLabelType
	// check if a node label has been added on the config
	if customLabels := config.Datadog.GetString("kubernetes_node_label_as_cluster_name"); customLabels != "" {
		clusterNameLabelKeys = append(clusterNameLabelKeys, clusterNameLabelType{key: customLabels, shouldOverride: true})
	} else {
		// Use default configuration
		clusterNameLabelKeys = defaultClusterNameLabelKeyConfigs
	}

	for _, labelConfig := range clusterNameLabelKeys {
		if v, ok := nodeLabels[labelConfig.key]; ok {
			if clusterName == "" {
				clusterName = v
				continue
			}

			if labelConfig.shouldOverride {
				clusterName = v
			}
		}
	}
	return clusterName, nil
}
