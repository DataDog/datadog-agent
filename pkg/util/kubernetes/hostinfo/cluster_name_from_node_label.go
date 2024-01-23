// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostinfo

import (
	"context"
)

const (
	eksClusterNameLabelKey       = "alpha.eksctl.io/cluster-name"
	datadogADClusterNameLabelKey = "ad.datadoghq.com/cluster-name"
)

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
	panic("not called")
}
