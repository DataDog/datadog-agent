// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	corev1 "k8s.io/api/core/v1"

	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

// PodMutator provides helper functions for mutating pods.
// It encapsulates common operations like adding volumes, volume mounts, and init containers.
type PodMutator struct {
	pod *corev1.Pod
	cfg ProviderConfig
}

// NewPodMutator creates a new PodMutator for the given pod.
func NewPodMutator(pod *corev1.Pod, cfg ProviderConfig) *PodMutator {
	return &PodMutator{
		pod: pod,
		cfg: cfg,
	}
}

// AddVolume adds a volume to the pod. If a volume with the same name exists, it replaces it.
// It also marks the volume as safe to evict for the cluster autoscaler.
func (m *PodMutator) AddVolume(vol corev1.Volume) {
	for i, existing := range m.pod.Spec.Volumes {
		if existing.Name == vol.Name {
			m.pod.Spec.Volumes[i] = vol
			mutatecommon.MarkVolumeAsSafeToEvictForAutoscaler(m.pod, vol.Name)
			return
		}
	}
	m.pod.Spec.Volumes = append(m.pod.Spec.Volumes, vol)
	mutatecommon.MarkVolumeAsSafeToEvictForAutoscaler(m.pod, vol.Name)
}

// AddVolumeMount adds a volume mount to all application containers (filtered by ContainerFilter).
// If a mount with the same name and path exists, it replaces it.
func (m *PodMutator) AddVolumeMount(mount corev1.VolumeMount) {
	for i := range m.pod.Spec.Containers {
		ctr := &m.pod.Spec.Containers[i]
		if m.cfg.ContainerFilter != nil && !m.cfg.ContainerFilter(ctr) {
			continue
		}
		m.addVolumeMountToContainer(ctr, mount)
	}
}

// AddVolumeMountToContainer adds a volume mount to a specific container.
func (m *PodMutator) addVolumeMountToContainer(ctr *corev1.Container, mount corev1.VolumeMount) {
	for j, existing := range ctr.VolumeMounts {
		if existing.Name == mount.Name && existing.MountPath == mount.MountPath {
			ctr.VolumeMounts[j] = mount
			return
		}
	}
	// Prepend volume mounts
	ctr.VolumeMounts = append([]corev1.VolumeMount{mount}, ctr.VolumeMounts...)
}

// AddInitContainer adds an init container to the pod.
// If an init container with the same name exists, it replaces it.
// The init container is prepended to run before other init containers.
func (m *PodMutator) AddInitContainer(initCtr corev1.Container) {
	for i, existing := range m.pod.Spec.InitContainers {
		if existing.Name == initCtr.Name {
			m.pod.Spec.InitContainers[i] = initCtr
			return
		}
	}
	// Prepend init containers
	m.pod.Spec.InitContainers = append([]corev1.Container{initCtr}, m.pod.Spec.InitContainers...)
}

// AddEnvVar adds an environment variable to all application containers (filtered by ContainerFilter).
// If the env var already exists, it is not overwritten.
func (m *PodMutator) AddEnvVar(env corev1.EnvVar) {
	for i := range m.pod.Spec.Containers {
		ctr := &m.pod.Spec.Containers[i]
		if m.cfg.ContainerFilter != nil && !m.cfg.ContainerFilter(ctr) {
			continue
		}
		m.addEnvVarToContainer(ctr, env)
	}
}

// addEnvVarToContainer adds an env var to a specific container if it doesn't already exist.
func (m *PodMutator) addEnvVarToContainer(ctr *corev1.Container, env corev1.EnvVar) {
	for _, existing := range ctr.Env {
		if existing.Name == env.Name {
			return // Already exists, don't overwrite
		}
	}
	// Prepend env var
	ctr.Env = append([]corev1.EnvVar{env}, ctr.Env...)
}

// Pod returns the underlying pod being mutated.
func (m *PodMutator) Pod() *corev1.Pod {
	return m.pod
}
