// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet,kubeapiserver

package hostinfo

import (
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

func GetClusterName() string {
	clusterName := clustername.GetClusterName()
	if clusterName != "" {
		return clusterName
	}
	clusterName, _ = getNodeClusterName()
	return clusterName
}

// getNodeClusterName returns clustername by fetching a node label
func getNodeClusterName() (string, error) {
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
