// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package common provides functions used by several mutating webhooks
package common

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

// K8sAutoscalerSafeToEvictVolumesAnnotation is the annotation used by the
// Kubernetes cluster-autoscaler to mark a volume as safe to evict
const K8sAutoscalerSafeToEvictVolumesAnnotation = "cluster-autoscaler.kubernetes.io/safe-to-evict-local-volumes"

// contains returns whether EnvVar slice contains an env var with a given name
func contains(envs []corev1.EnvVar, name string) bool {
	for _, env := range envs {
		if env.Name == name {
			return true
		}
	}
	return false
}

// BuildEnvVarFunc builds a fresh environment value for the selected container.
type BuildEnvVarFunc func(container *corev1.Container, init bool) (corev1.EnvVar, error)

// PodString returns a string that helps identify the pod
func PodString(pod *corev1.Pod) string {
	if pod.GetNamespace() == "" || pod.GetName() == "" {
		return "with generate name " + pod.GetGenerateName()
	}
	return pod.GetNamespace() + "/" + pod.GetName()
}

// containsVolumeMount returns whether a list of volume mounts contains
// at least one volume mount with a given name or mount path
func containsVolumeMount(volumeMounts []corev1.VolumeMount, element corev1.VolumeMount) bool {
	for _, volumeMount := range volumeMounts {
		if volumeMount.Name == element.Name {
			return true
		}
		if volumeMount.MountPath == element.MountPath {
			return true
		}
	}
	return false
}

// ContainerRegistry gets the container registry config using the specified
// config option, and falls back to the default container registry if no
// webhook-specific container registry is set.
func ContainerRegistry(datadogConfig config.Component, specificConfigOpt string) string {
	if datadogConfig.IsConfigured(specificConfigOpt) {
		return datadogConfig.GetString(specificConfigOpt)
	}

	return datadogConfig.GetString("admission_controller.container_registry")
}
