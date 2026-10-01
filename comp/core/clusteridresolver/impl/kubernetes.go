// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package clusteridresolverimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

// resolveFromKubernetes reads the cluster ID from Kubernetes, and persists it
// when it is missing.
var resolveFromKubernetes lookupFunc = func(ctx context.Context) (string, error) {
	client, err := apiserver.WaitForAPIClient(ctx)
	if err != nil {
		return "", err
	}
	return common.ResolveClusterID(ctx, client.Cl.CoreV1())
}
