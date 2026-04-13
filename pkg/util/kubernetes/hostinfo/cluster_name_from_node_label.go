// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostinfo

import (
	"context"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
)

const (
	eksClusterNameLabelKey       = "alpha.eksctl.io/cluster-name"
	aksClusterNameLabelKey       = "kubernetes.azure.com/cluster"
	datadogADClusterNameLabelKey = "ad.datadoghq.com/cluster-name"
)

// clusterNameLabelType this struct is used to attach to the label key the information if this label should
// override cluster-name previously detected.
type clusterNameLabelType struct {
	key string
	// shouldOverride if set to true override previous cluster-name
	shouldOverride bool
	// parser is used to parse label content to grab cluster name for AKS for example
	parser func(string) string
}

// We use a slice to define the default Node label key to keep the ordering
var defaultClusterNameLabelKeyConfigs = []clusterNameLabelType{
	{key: eksClusterNameLabelKey, shouldOverride: false},
	{key: aksClusterNameLabelKey, shouldOverride: false, parser: aksClusterNameLabelParser},
	{key: datadogADClusterNameLabelKey, shouldOverride: true},
}

// GetNodeClusterNameLabel returns clustername by fetching a node label
func (n *NodeInfo) GetNodeClusterNameLabel(ctx context.Context, clusterName string) (string, error) {
	nodeLabels, err := n.getANodeLabels(ctx)
	if err != nil {
		return "", err
	}

	var clusterNameLabelKeys []clusterNameLabelType
	// check if a node label has been added on the config
	if customLabels := pkgconfigsetup.Datadog().GetString("kubernetes_node_label_as_cluster_name"); customLabels != "" {
		clusterNameLabelKeys = append(clusterNameLabelKeys, clusterNameLabelType{key: customLabels, shouldOverride: true})
	} else {
		// Use default configuration
		clusterNameLabelKeys = defaultClusterNameLabelKeyConfigs
	}

	for _, labelConfig := range clusterNameLabelKeys {
		if v, ok := nodeLabels[labelConfig.key]; ok {
			if labelConfig.parser != nil {
				v = labelConfig.parser(v)
			}
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

// aksClusterNameLabelParser tries to parse cluster name from resource group.
// If parsed cluster name is empty label is used unparsed instead.
func aksClusterNameLabelParser(label string) string {
	n, _ := azure.ParseClusterNameFromResouceGroup(label)
	if n != "" {
		return n
	}
	return label
}
