// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	configWebhook "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/config"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

////////////////////////////////
//                            //
//     Provider Overrides     //
//                            //
////////////////////////////////

const socketDir = "/var/run/datadog"
const apmSocket = socketDir + "/apm.socket"
const dogstatsdSocket = socketDir + "/dsd.socket"

// ddSocketsVolumeName is the name of the volume used to mount the APM and
// DogStatsD sockets. It needs to be different from the name used by the config
// webhook to distinguish them easily.
const ddSocketsVolumeName = "ddsockets"

// providerIsSupported indicates whether the provider is supported by agent sidecar injection
func providerIsSupported(provider string) bool {
	switch provider {
	case providerFargate:
		return true
	case "":
		// case of empty provider
		return true
	default:
		return false
	}
}

// applyProviderOverrides applies the necessary overrides for the provider
// configured. It returns a boolean that indicates if the pod was mutated.
func applyProviderOverrides(pod *corev1.Pod) (bool, error) {
	provider := config.Datadog.GetString("admission_controller.agent_sidecar.provider")

	if !providerIsSupported(provider) {
		return false, fmt.Errorf("unsupported provider: %v", provider)
	}

	switch provider {
	case providerFargate:
		return applyFargateOverrides(pod)
	}

	return false, nil
}

// applyFargateOverrides applies the necessary overrides for EKS Fargate.
// For the agent sidecar container:
//   - Sets DD_EKS_FARGATE=true
//   - Deletes the volume and volumeMounts for the APM socket added by the
//     config webhook when the injection mode is set to "socket". The volume is
//     "HostPath" and those don't work on Fargate. Notice that this means that
//     the agent sidecar webhook needs to be run after the config one. This is
//     guaranteed by the mutatingWebhooks function in the webhook package.
//   - Creates an "emptyDir" volume instead.
//   - Configures the APM UDS path with DD_APM_RECEIVER_SOCKET and the DogStatsD
//     socket with DD_DOGSTATSD_SOCKET.
//
// For the application containers:
//   - Sets DD_TRACE_AGENT_URL to the APM UDS path configured for the agent.
//   - Sets DD_DOGSTATSD_URL to the DogStatsD UDS path configured for the agent.
//
// This function returns a boolean that indicates if the pod was mutated.
func applyFargateOverrides(pod *corev1.Pod) (bool, error) {
	if pod == nil {
		return false, fmt.Errorf("can't apply profile overrides to nil pod")
	}

	mutated := false

	deleted := deleteConfigWebhookVolumeAndMounts(pod)
	mutated = mutated || deleted

	volume, volumeMount := socketsVolume()
	injected := common.InjectVolume(pod, volume, volumeMount)
	mutated = mutated || injected

	// ShareProcessNamespace is required for the process collection feature
	if pod.Spec.ShareProcessNamespace == nil || !*pod.Spec.ShareProcessNamespace {
		pod.Spec.ShareProcessNamespace = pointer.Ptr(true)
		mutated = true
	}

	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == agentSidecarContainerName {
			overridden, err := applyOverridesAgentContainer(&pod.Spec.Containers[i])
			if err != nil {
				return mutated, err
			}
			mutated = mutated || overridden
		} else {
			overridden, err := applyOverridesAppContainer(&pod.Spec.Containers[i])
			if err != nil {
				return mutated, err
			}
			mutated = mutated || overridden
		}
	}

	return mutated, nil
}

func applyOverridesAgentContainer(container *corev1.Container) (bool, error) {
	return withEnvOverrides(
		container,
		corev1.EnvVar{
			Name:  "DD_EKS_FARGATE",
			Value: "true",
		},
		corev1.EnvVar{
			Name:  "DD_APM_RECEIVER_SOCKET",
			Value: apmSocket,
		},
		corev1.EnvVar{
			Name:  "DD_DOGSTATSD_SOCKET",
			Value: dogstatsdSocket,
		},
	)
}

func applyOverridesAppContainer(container *corev1.Container) (bool, error) {
	return withEnvOverrides(
		container,
		corev1.EnvVar{
			Name:  "DD_TRACE_AGENT_URL",
			Value: "unix://" + apmSocket,
		},
		corev1.EnvVar{
			Name:  "DD_DOGSTATSD_URL",
			Value: "unix://" + dogstatsdSocket,
		},
	)
}

// socketsVolume returns the volume and volume mounts used for the APM and the
// DogStatsD sockets.
func socketsVolume() (corev1.Volume, corev1.VolumeMount) {
	volume := corev1.Volume{
		Name: ddSocketsVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	volumeMount := corev1.VolumeMount{
		Name:      ddSocketsVolumeName,
		MountPath: socketDir,
		ReadOnly:  false, // Need RW for UDS APM socket
	}

	return volume, volumeMount
}

// deleteConfigWebhookVolumeAndMounts deletes the volume and volumeMounts added
// by the config webhook. Returns a boolean that indicates if the pod was
// mutated.
func deleteConfigWebhookVolumeAndMounts(pod *corev1.Pod) bool {
	mutated := false

	// Delete the volume added by the config webhook
	for i, vol := range pod.Spec.Volumes {
		if vol.Name == configWebhook.DatadogVolumeName {
			pod.Spec.Volumes = append(pod.Spec.Volumes[:i], pod.Spec.Volumes[i+1:]...)
			mutated = true
			break
		}
	}

	deleted := deleteConfigWebhookVolumeMounts(pod.Spec.Containers)
	mutated = mutated || deleted

	deleted = deleteConfigWebhookVolumeMounts(pod.Spec.InitContainers)
	mutated = mutated || deleted

	return mutated
}

// deleteConfigWebhookVolumeMounts deletes the volumeMounts added by the config
// webhook. Returns a boolean that indicates if the pod was mutated.
func deleteConfigWebhookVolumeMounts(containers []corev1.Container) bool {
	mutated := false

	for i, container := range containers {
		for j, volMount := range container.VolumeMounts {
			if volMount.Name == configWebhook.DatadogVolumeName {
				containers[i].VolumeMounts = append(
					containers[i].VolumeMounts[:j],
					containers[i].VolumeMounts[j+1:]...,
				)
				mutated = true
				break
			}
		}
	}

	return mutated
}
