// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet,kubeapiserver

package hostinfo

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// getNodeLabels returns node labels for this host
func getNodeLabels() (map[string]string, error) {
	ku, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}

	nodeName, err := ku.GetNodename()
	if err != nil {
		return nil, err
	}

	var nodeLabels map[string]string
	if config.Datadog.GetBool("cluster_agent.enabled") {
		cl, err := clusteragent.GetClusterAgentClient()
		if err != nil {
			return nil, err
		}
		nodeLabels, err = cl.GetNodeLabels(nodeName)
		if err != nil {
			return nil, err
		}
	} else {
		client, err := apiserver.GetAPIClient()
		if err != nil {
			return nil, err
		}
		nodeLabels, err = client.NodeLabels(nodeName)
		if err != nil {
			return nil, err
		}
	}
	return nodeLabels, nil
}

// getNodeClusterNameLabel returns clustername by fetching a node label
func getNodeClusterNameLabel() (string, error) {
	nodeLabels, err := getNodeLabels()
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
