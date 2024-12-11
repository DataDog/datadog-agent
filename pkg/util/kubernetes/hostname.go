// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && !kubelet

package kubernetes

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetKubeAPIServerHostname returns the hostname from kubeapiserver
func GetKubeAPIServerHostname(ctx context.Context) (string, error) {
	nodeName, err := apiserver.HostNodeName(ctx)
	if err != nil {
		return "", err
	}

	clusterName := clustername.GetClusterName(ctx, nodeName)
	if clusterName == "" {
		log.Debugf("Now using plain kubernetes nodename as an alias: no cluster name was set and none could be autodiscovered")
		return nodeName, nil
	} else {
		return (nodeName + "-" + clusterName), nil
	}
}
