// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows || darwin
// +build linux windows darwin

package hostname

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// callProvider returns the hostname for a specific Provider if it was register
func callProvider(ctx context.Context, provider func(context.Context) (string error), providerName string) (string, error) {
	log.Debugf("GetHostname trying provider '%s' ...", providerName)
	name, err := provider(ctx)
	if err != nil {
		return "", err
	}
	if validate.ValidHostname(name) != nil {
		return "", fmt.Errorf("Invalid hostname '%s' from %s provider", name, providerName)
	}
	return name, nil
}

func fromContainer(ctx context.Context, _ string) (string, error) {
	if !config.IsContainerized() {
		return "", fmt.Errorf("the agent is not containerized")
	}

	// Cluster-agent logic: Kube apiserver
	if config.IsFeaturePresent(config.Kubernetes) {
		return callProvider(ctx, kubeapiserverHostname, "kube_apiserver")
	}

	// Node-agent logic: docker or kubelet
	if config.IsFeaturePresent(config.Docker) {
		return callProvider(ctx, docker.GetHostname, "docker")
	}

	if config.IsFeaturePresent(config.Kubernetes) {
		return callProvider(ctx, kubelet.GetHostname, "kubelet")
	}

	return "", fmt.Errorf("no container environment detected")
}
