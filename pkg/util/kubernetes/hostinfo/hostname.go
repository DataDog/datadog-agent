// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet

package hostinfo

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// KubeletHostnameProvider is the kubelet implementation for the hostname provider
func KubeletHostnameProvider() (string, error) {
	return kubeletHostname()
}

// kubeletHostname builds a hostname from the kubernetes nodename and an optional cluster-name
func kubeletHostname() (string, error) {
	ku, err := kubelet.GetKubeUtil()
	if err != nil {
		return "", err
	}
	nodeName, err := ku.GetNodename()
	if err != nil {
		return "", fmt.Errorf("couldn't fetch the host nodename from the kubelet: %s", err)
	}

	clusterName := GetClusterName()
	if clusterName == "" {
		log.Debugf("Now using plain kubernetes nodename as an alias: no cluster name was set and none could be autodiscovered")
		return nodeName, nil
	}
	return (nodeName + "-" + clusterName), nil
}
