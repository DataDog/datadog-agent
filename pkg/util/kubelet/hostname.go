// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/ddot"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	k "github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/nodename"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type kubeUtilGetter func() (k.KubeUtilInterface, error)

var kubeUtilGet kubeUtilGetter = k.GetKubeUtil

// GetHostname builds a hostname from the kubernetes nodename and an optional cluster-name
func GetHostname(ctx context.Context) (string, error) {
	if !env.IsFeaturePresent(env.Kubernetes) {
		return "", nil
	}

	nodeName, err := getNodename(ctx)
	if err != nil {
		return "", err
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

// getNodename returns the name of the node the agent runs on.
//
// otel-agent in DDOT standalone mode first reads it from the environment
// variables listed in otelcollector.standalone.node_from_env_var, as its
// nodefilter workloadmeta collector does. That way, it works with what a
// deployment set up for the upstream OpenTelemetry Collector provides: no
// kubelet API access, which RBAC meant for its k8sattributesprocessor doesn't
// grant, and none of the Datadog-specific kubelet settings
// (kubernetes_kubelet_host, kubelet_tls_verify), which the OpenTelemetry Helm
// charts don't set. It falls back to the kubelet unless
// otelcollector.standalone.use_kubelet_collector is false: a deployment set up
// for the nodefilter collector lacks kubelet API access.
func getNodename(ctx context.Context) (string, error) {
	cfg := pkgconfigsetup.Datadog()
	if ddot.IsStandalone(cfg) {
		nodeName, envVar, err := nodename.FromEnv(cfg.GetString("otelcollector.standalone.node_from_env_var"))
		if err == nil {
			log.Debugf("Using kubernetes nodename %q from environment variable %q", nodeName, envVar)
			return nodeName, nil
		}
		if ddot.KubeletDisabled(cfg) {
			return "", fmt.Errorf("couldn't read the host nodename from the environment: %w (otelcollector.standalone.node_from_env_var)", err)
		}
		log.Debugf("Couldn't read the host nodename from the environment, falling back to the kubelet: %s", err)
	}

	ku, err := kubeUtilGet()
	if err != nil {
		return "", err
	}
	nodeName, err := ku.GetNodename(ctx)
	if err != nil {
		return "", fmt.Errorf("couldn't fetch the host nodename from the kubelet: %s", err)
	}
	return nodeName, nil
}

// getRFC1123CompliantClusterName returns a k8s cluster name if it exists, either directly specified or autodiscovered
// Some kubernetes cluster-names (EKS,AKS) are not RFC1123 compliant, mostly due to an `_`.
// This function replaces the invalid `_` with a valid `-`.
func getRFC1123CompliantClusterName(ctx context.Context, hostname string) (string, string) {
	if !env.IsFeaturePresent(env.Kubernetes) {
		return "", ""
	}
	clusterName := clustername.GetClusterName(ctx, hostname)
	return makeClusterNameRFC1123Compliant(clusterName)
}

// makeClusterNameRFC1123Compliant returns the compliant cluster name and as the second return value the initial clusterName
func makeClusterNameRFC1123Compliant(clusterName string) (string, string) {
	return clustername.MakeClusterNameRFC1123Compliant(clusterName), clusterName
}
