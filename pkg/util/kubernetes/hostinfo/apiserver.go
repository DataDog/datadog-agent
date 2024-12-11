// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package hostinfo

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func apiserverNodeLabels(ctx context.Context, nodeName string) (map[string]string, error) {
	client, err := apiserver.WaitForAPIClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.NodeLabels(nodeName)
}

func apiserverNodeAnnotations(ctx context.Context, nodeName string) (map[string]string, error) {
	client, err := apiserver.WaitForAPIClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.NodeAnnotations(nodeName)
}
