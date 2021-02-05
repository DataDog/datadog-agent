// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

// HostNodeName retrieves the hostname from the apiserver, assuming our hostname
// is the pod name. It connects to the apiserver, and returns the node name where
// our pod is scheduled.
// Tested in the TestHostnameProvider integration test
func HostNodeName() (string, error) {
	c, err := GetAPIClient()
	if err != nil {
		return "", fmt.Errorf("could not connect to the apiserver: %s", err)
	}
	podName, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("could not fetch our hostname: %s", err)
	}

	nodeName, err := c.GetNodeForPod(common.GetMyNamespace(), podName)
	if err != nil {
		return "", fmt.Errorf("could not fetch the host nodename from the apiserver: %s", err)
	}
	return nodeName, nil
}
