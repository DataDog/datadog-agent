// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows || darwin

package hostname

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// for testing purposes
var (
	configIsContainerized  = config.IsContainerized
	configIsFeaturePresent = config.IsFeaturePresent

	kubernetesGetKubeAPIServerHostname = kubernetes.GetKubeAPIServerHostname
	dockerGetHostname                  = docker.GetHostname
	kubeletGetHostname                 = kubelet.GetHostname
)

// callContainerProvider returns the hostname for a specific Provider
func callContainerProvider(ctx context.Context, provider func(context.Context) (string, error), providerName string) string {
	log.Debugf("GetHostname trying provider '%s' ...", providerName)
	hostname, err := provider(ctx)
	if err != nil {
		log.Debugf("error calling provider '%s': %s", providerName, err)
		return ""
	}
	if validate.ValidHostname(hostname) != nil {
		log.Debugf("provider '%s' return invalid hostname '%s'", providerName, hostname)
		return ""
	}
	return hostname
}

// for testing purposes
func fromContainer(ctx context.Context, _ string) (string, error) {
	if !configIsContainerized() {
		return "", fmt.Errorf("the agent is not containerized")
	}

	// Cluster-agent logic: Kube apiserver
	if configIsFeaturePresent(config.Kubernetes) {
		if hostname := callContainerProvider(ctx, kubernetesGetKubeAPIServerHostname, "kube_apiserver"); hostname != "" {
			return hostname, nil
		}
	}

	// Node-agent logic: docker or kubelet
	if configIsFeaturePresent(config.Docker) {
		if hostname := callContainerProvider(ctx, dockerGetHostname, "docker"); hostname != "" {
			return hostname, nil
		}
	}

	if configIsFeaturePresent(config.Kubernetes) {
		if hostname := callContainerProvider(ctx, kubeletGetHostname, "kubelet"); hostname != "" {
			return hostname, nil
		}
	}

	return "", fmt.Errorf("no container environment detected or none of them detected a valid hostname")
}
