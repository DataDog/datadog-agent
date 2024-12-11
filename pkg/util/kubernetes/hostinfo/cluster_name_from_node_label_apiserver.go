// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet && kubeapiserver

package hostinfo

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// getANodeLabels returns node labels for a random node of this cluster
func (_ *NodeInfo) getANodeLabels(ctx context.Context) (map[string]string, error) {
	client, err := apiserver.WaitForAPIClient(ctx)
	if err != nil {
		return nil, err
	}

	aRandomNodeName, err := client.GetARandomNodeName(ctx)
	if err != nil {
		return nil, err
	}

	return client.NodeLabels(aRandomNodeName)
}
