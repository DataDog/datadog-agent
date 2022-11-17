// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containersorpods

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	dockerReadyBit     = 1 << 0
	kubernetesReadyBit = 1 << 1
)

func TestChoose(t *testing.T) {
	test := func(
		features config.FeatureMap,
		k8sContainerUseFile bool,
		ready int,
		expected LogWhat,
	) func(*testing.T) {
		return func(t *testing.T) {
			mockConfig := config.Mock(t)
			mockConfig.Set("logs_config.k8s_container_use_file", k8sContainerUseFile)
			config.SetDetectedFeatures(features)
			defer config.SetDetectedFeatures(config.FeatureMap{})

			chsr := chooser{
				choice:       make(chan LogWhat, 1),
				kubeletReady: func() (bool, time.Duration) { return 0 != ready&kubernetesReadyBit, 0 },
				dockerReady:  func() (bool, time.Duration) { return 0 != ready&dockerReadyBit, 0 },
			}
			chsr.choose(false)

			select {
			case c := <-chsr.choice:
				require.Equal(t, expected, c)
			default:
				require.Equal(t, expected, LogUnknown, "did not get a choice at all")
			}
		}
	}

	features := func(features ...config.Feature) config.FeatureMap {
		fm := config.FeatureMap{}
		for _, f := range features {
			fm[f] = struct{}{}
		}
		return fm
	}

	// - if any of the container features (docker, containerd, cri, podman) are
	//   present and kubernetes is not, wait for the docker service to start and
	//   return LogContainers

	t.Run("docker ready, only docker enabled",
		test(features(config.Docker), false, dockerReadyBit, LogContainers))

	t.Run("docker not ready, only docker enabled",
		test(features(config.Docker), false, 0, LogUnknown))

	t.Run("docker ready, only containerd enabled",
		test(features(config.Containerd), false, dockerReadyBit, LogContainers))

	t.Run("docker not ready, only containerd enabled",
		test(features(config.Containerd), false, 0, LogUnknown))

	t.Run("docker ready, only CRI enabled",
		test(features(config.Cri), false, dockerReadyBit, LogContainers))

	t.Run("docker not ready, only CRI enabled",
		test(features(config.Cri), false, 0, LogUnknown))

	t.Run("docker ready, only Podman enabled",
		test(features(config.Podman), false, dockerReadyBit, LogContainers))

	t.Run("docker not ready, only Podman enabled",
		test(features(config.Podman), false, 0, LogUnknown))

	// - if the kubernetes feature is available and no container features are
	//   available, wait for the kubelet service to start, and return LogPods

	t.Run("k8s ready, only k8s enabled",
		test(features(config.Kubernetes), false, kubernetesReadyBit, LogPods))

	t.Run("k8s not ready, only k8s enabled",
		test(features(config.Kubernetes), false, 0, LogUnknown))

	// - if none of the features are available, LogNothing

	t.Run("nothing ready, nothing enabled",
		test(features(), false, 0, LogNothing))

	// - if at least one container feature _and_ the kubernetes feature are available,
	//   then wait for either of the docker service or the kubelet service to start.
	//   This always tries both at the same time, and if both are available will
	//   return LogPods if `logs_config.k8s_container_use_file` is true or
	//   LogContainers if the configuration setting is false.

	t.Run("nothing ready, docker and kubernetes enabled",
		test(features(config.Docker, config.Kubernetes), false, 0, LogUnknown))

	t.Run("k8s ready, docker and kubernetes enabled",
		test(features(config.Docker, config.Kubernetes), false, kubernetesReadyBit, LogPods))

	t.Run("docker ready, docker and kubernetes enabled",
		test(features(config.Docker, config.Kubernetes), false, dockerReadyBit, LogContainers))

	t.Run("docker ready, Containerd and kubernetes enabled",
		test(features(config.Containerd, config.Kubernetes), false, dockerReadyBit, LogContainers))

	t.Run("both ready, docker and kubernetes enabled, k8s_container_use_file=true",
		test(features(config.Docker, config.Kubernetes), true, dockerReadyBit|kubernetesReadyBit, LogPods))

	t.Run("both ready, docker and kubernetes enabled, k8s_container_use_file=false",
		test(features(config.Docker, config.Kubernetes), false, dockerReadyBit|kubernetesReadyBit, LogContainers))

	t.Run("both ready, Containerds and kubernetes enabled, k8s_container_use_file=false",
		test(features(config.Containerd, config.Kubernetes), false, dockerReadyBit|kubernetesReadyBit, LogContainers))
}
