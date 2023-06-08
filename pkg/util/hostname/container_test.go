// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows || darwin

package hostname

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func TestFromContainerNotContainerized(t *testing.T) {
	defer func() {
		configIsContainerized = config.IsContainerized
	}()
	configIsContainerized = func() bool { return false }

	_, err := fromContainer(context.TODO(), "")
	assert.Error(t, err)
}

func TestFromContainer(t *testing.T) {
	defer func() {
		configIsContainerized = config.IsContainerized
		configIsFeaturePresent = config.IsFeaturePresent
		kubernetesGetKubeAPIServerHostname = kubernetes.GetKubeAPIServerHostname
		dockerGetHostname = docker.GetHostname
		kubeletGetHostname = kubelet.GetHostname
	}()

	configIsContainerized = func() bool { return true }
	enabledFeature := config.Kubernetes
	configIsFeaturePresent = func(f config.Feature) bool { return f == enabledFeature }
	kubernetesGetKubeAPIServerHostname = func(context.Context) (string, error) { return "kubernetes-hostname", nil }
	dockerGetHostname = func(context.Context) (string, error) { return "docker-hostname", nil }
	kubeletGetHostname = func(context.Context) (string, error) { return "kubelet-hostname", nil }

	ctx := context.TODO()

	// kubernetes

	hostname, err := fromContainer(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "kubernetes-hostname", hostname)

	// kubelet
	kubernetesGetKubeAPIServerHostname = func(context.Context) (string, error) { return "", fmt.Errorf("some error") }

	hostname, err = fromContainer(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "kubelet-hostname", hostname)

	kubeletGetHostname = func(context.Context) (string, error) { return "", fmt.Errorf("some error") }
	_, err = fromContainer(ctx, "")
	assert.Error(t, err)

	// Docker
	enabledFeature = config.Docker

	hostname, err = fromContainer(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "docker-hostname", hostname)

	dockerGetHostname = func(context.Context) (string, error) { return "", fmt.Errorf("some error") }
	_, err = fromContainer(ctx, "")
	require.Error(t, err)
}

func TestFromContainerInvalidHostname(t *testing.T) {
	defer func() {
		configIsContainerized = config.IsContainerized
		configIsFeaturePresent = config.IsFeaturePresent
		kubernetesGetKubeAPIServerHostname = kubernetes.GetKubeAPIServerHostname
		dockerGetHostname = docker.GetHostname
		kubeletGetHostname = kubelet.GetHostname
	}()

	configIsContainerized = func() bool { return true }
	configIsFeaturePresent = func(f config.Feature) bool { return true }
	kubernetesGetKubeAPIServerHostname = func(context.Context) (string, error) { return "hostname_with_underscore", nil }
	dockerGetHostname = func(context.Context) (string, error) { return "hostname_with_underscore", nil }
	kubeletGetHostname = func(context.Context) (string, error) { return "hostname_with_underscore", nil }

	_, err := fromContainer(context.TODO(), "")
	require.Error(t, err)
}
