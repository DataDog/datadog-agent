// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

// HostNodeName retrieves the hostname from the apiserver, pod name will be retrieved from DD_POD_NAME.
// It connects to the apiserver, and returns the node name where our pod is scheduled.
// Tested in the TestHostnameProvider integration test
func HostNodeName(ctx context.Context) (string, error) {
	c, err := GetAPIClient()
	if err != nil {
		return "", fmt.Errorf("could not connect to the apiserver: %s", err)
	}
	podName, err := common.GetSelfPodName()
	if err != nil {
		return "", fmt.Errorf("could not fetch our self pod name: %w", err)
	}

	nodeName, err := c.GetNodeForPod(ctx, common.GetMyNamespace(), podName)
	if err != nil {
		return "", fmt.Errorf("could not fetch the host nodename from the apiserver: %s", err)
	}
	return nodeName, nil
}
