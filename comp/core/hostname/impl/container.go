// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows || darwin

package hostnameimpl

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// These variables are overridable for testing.
var (
	configIsContainerized  = env.IsContainerized
	configIsFeaturePresent = env.IsFeaturePresent

	kubernetesGetKubeAPIServerHostname = kubernetes.GetKubeAPIServerHostname
	dockerGetHostname                  = docker.GetHostname
	kubeletGetHostname                 = kubelet.GetHostname
)

func callContainerProvider(ctx context.Context, provider func(context.Context) (string, error), providerName string) string {
	log.Debugf("GetHostname trying container provider '%s' ...", providerName)
	hostname, err := provider(ctx)
	if err != nil {
		log.Debugf("error calling container provider '%s': %s", providerName, err)
		return ""
	}
	if validate.ValidHostname(hostname) != nil {
		log.Debugf("container provider '%s' returned invalid hostname '%s'", providerName, hostname)
		return ""
	}
	return hostname
}

func fromContainer(ctx context.Context, _ pkgconfigmodel.Reader, _ string) (string, error) {
	if !configIsContainerized() {
		return "", errors.New("the agent is not containerized")
	}

	// Cluster-agent: Kube API server
	if configIsFeaturePresent(env.Kubernetes) {
		if hostname := callContainerProvider(ctx, kubernetesGetKubeAPIServerHostname, "kube_apiserver"); hostname != "" {
			return hostname, nil
		}
	}

	// Node-agent: Docker or kubelet
	if configIsFeaturePresent(env.Docker) {
		if hostname := callContainerProvider(ctx, dockerGetHostname, "docker"); hostname != "" {
			return hostname, nil
		}
	}

	if configIsFeaturePresent(env.Kubernetes) {
		if hostname := callContainerProvider(ctx, kubeletGetHostname, "kubelet"); hostname != "" {
			return hostname, nil
		}
	}

	return "", errors.New("no container environment detected or none of them detected a valid hostname")
}
