// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !kubelet

package hostinfo

// GetNodeLabels returns node labels for this host
func GetNodeLabels() (map[string]string, error) {
	return nil, nil
}

// GetNodeClusterNameLabel returns clustername by fetching a node label
func GetNodeClusterNameLabel() (string, error) {
	return "", nil
}
