// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet

package hostinfo

import "context"

// NodeInfo is use to get Kubernetes Node metadata information
type NodeInfo struct {
}

// NewNodeInfo return a new NodeInfo instance
// return an error if it fails to access the kubelet client.
func NewNodeInfo() (*NodeInfo, error) {
	return &NodeInfo{}, nil
}

// GetNodeLabels returns node labels for this host
func (n *NodeInfo) GetNodeLabels(ctx context.Context) (map[string]string, error) {
	return nil, nil
}
