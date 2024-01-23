// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows || darwin

package hostname

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
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
	panic("not called")
}

// for testing purposes
func fromContainer(ctx context.Context, _ string) (string, error) {
	panic("not called")
}
