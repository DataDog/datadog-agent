// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && kubelet

package nvml

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
)

// fetchNodeLabels reads this node's labels from the cluster agent or the API
// server.
func fetchNodeLabels(ctx context.Context) (map[string]string, error) {
	ni, err := hostinfo.NewNodeInfo()
	if err != nil {
		return nil, err
	}
	return ni.GetNodeLabels(ctx)
}
