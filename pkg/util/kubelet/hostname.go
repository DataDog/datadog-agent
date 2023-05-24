// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	k "github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type kubeUtilGetter func() (k.KubeUtilInterface, error)

var kubeUtilGet kubeUtilGetter = k.GetKubeUtil

// GetHostname builds a hostname from the kubernetes nodename and an optional cluster-name
func GetHostname(ctx context.Context) (string, error) {
	if !config.IsFeaturePresent(config.Kubernetes) {
		return "", nil
	}

	ku, err := kubeUtilGet()
	if err != nil {
		return "", err
	}
	nodeName, err := ku.GetNodename(ctx)
	if err != nil {
		return "", fmt.Errorf("couldn't fetch the host nodename from the kubelet: %s", err)
	}

	clusterName, initialClusterName := getRFC1123CompliantClusterName(ctx, nodeName)
	if clusterName == "" {
		log.Debugf("Now using plain kubernetes nodename as an alias: no cluster name was set and none could be autodiscovered")
		return nodeName, nil
	}
	if clusterName != initialClusterName {
		log.Debugf("hostAlias: cluster name: '%s' contains `_`, replacing it with `-` to be RFC1123 compliant", clusterName)
	}
	return nodeName + "-" + clusterName, nil
}

// getRFC1123CompliantClusterName returns a k8s cluster name if it exists, either directly specified or autodiscovered
// Some kubernetes cluster-names (EKS,AKS) are not RFC1123 compliant, mostly due to an `_`.
// This function replaces the invalid `_` with a valid `-`.
func getRFC1123CompliantClusterName(ctx context.Context, hostname string) (string, string) {
	if !config.IsFeaturePresent(config.Kubernetes) {
		return "", ""
	}
	clusterName := clustername.GetClusterName(ctx, hostname)
	return makeClusterNameRFC1123Compliant(clusterName)
}

// makeClusterNameRFC1123Compliant returns the compliant cluster name and as the second return value the initial clusterName
func makeClusterNameRFC1123Compliant(clusterName string) (string, string) {
	return clustername.MakeClusterNameRFC1123Compliant(clusterName), clusterName
}
