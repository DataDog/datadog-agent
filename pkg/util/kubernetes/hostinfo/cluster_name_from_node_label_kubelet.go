// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package hostinfo

import "context"

// getANodeLabels returns node labels for this host
func (n *NodeInfo) getANodeLabels(ctx context.Context) (map[string]string, error) {
	return n.GetNodeLabels(ctx)
}
