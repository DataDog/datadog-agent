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

// PodPatcher provides helper functions for mutating pods.
// It encapsulates common operations like adding volumes, volume mounts, and init containers.
type PodPatcher struct {
	pod    *corev1.Pod
	filter func(*corev1.Container) bool
}

// NewPodPatcher creates a new PodPatcher for the given pod.
// The optional filter function can be used to exclude certain containers from mutations.
func NewPodPatcher(pod *corev1.Pod, filter func(*corev1.Container) bool) *PodPatcher {
	return &PodPatcher{
		pod:    pod,
		filter: filter,
	}
}

// AddVolume adds a volume to the pod. If a volume with the same name exists, it replaces it.
// It also marks the volume as safe to evict for the cluster autoscaler.
func (p *PodPatcher) AddVolume(vol corev1.Volume) {
	for i, existing := range p.pod.Spec.Volumes {
		if existing.Name == vol.Name {
			p.pod.Spec.Volumes[i] = vol
			mutatecommon.MarkVolumeAsSafeToEvictForAutoscaler(p.pod, vol.Name)
			return
		}
	}
	p.pod.Spec.Volumes = append(p.pod.Spec.Volumes, vol)
	mutatecommon.MarkVolumeAsSafeToEvictForAutoscaler(p.pod, vol.Name)
}

// AddVolumeMount adds a volume mount to all application containers (filtered by ContainerFilter).
// If a mount with the same name and path exists, it replaces it.
func (p *PodPatcher) AddVolumeMount(mount corev1.VolumeMount) {
	for i := range p.pod.Spec.Containers {
		ctr := &p.pod.Spec.Containers[i]
		if p.filter != nil && !p.filter(ctr) {
			continue
		}
		p.addVolumeMountToContainer(ctr, mount)
	}
}

// addVolumeMountToContainer adds a volume mount to a specific container.
func (p *PodPatcher) addVolumeMountToContainer(ctr *corev1.Container, mount corev1.VolumeMount) {
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
func (p *PodPatcher) AddInitContainer(initCtr corev1.Container) {
	for i, existing := range p.pod.Spec.InitContainers {
		if existing.Name == initCtr.Name {
			p.pod.Spec.InitContainers[i] = initCtr
			return
		}
	}
	// Prepend init containers
	p.pod.Spec.InitContainers = append([]corev1.Container{initCtr}, p.pod.Spec.InitContainers...)
}

// AddEnvVar adds an environment variable to all application containers (filtered by ContainerFilter).
// If the env var already exists, it is not overwritten.
func (p *PodPatcher) AddEnvVar(env corev1.EnvVar) {
	for i := range p.pod.Spec.Containers {
		ctr := &p.pod.Spec.Containers[i]
		if p.filter != nil && !p.filter(ctr) {
			continue
		}
		p.addEnvVarToContainer(ctr, env)
	}
}

// AddEnvVarWithJoin adds an environment variable to all application containers (filtered by ContainerFilter).
// If the env var already exists, the new value is appended using the specified separator.
func (p *PodPatcher) AddEnvVarWithJoin(name, value, separator string) {
	for i := range p.pod.Spec.Containers {
		ctr := &p.pod.Spec.Containers[i]
		if p.filter != nil && !p.filter(ctr) {
			continue
		}
		p.addEnvVarWithJoinToContainer(ctr, name, value, separator)
	}
}

// addEnvVarToContainer adds an env var to a specific container if it doesn't already exist.
func (p *PodPatcher) addEnvVarToContainer(ctr *corev1.Container, env corev1.EnvVar) {
	for _, existing := range ctr.Env {
		if existing.Name == env.Name {
			return // Already exists, don't overwrite
		}
	}
	// Prepend env var
	ctr.Env = append([]corev1.EnvVar{env}, ctr.Env...)
}

// addEnvVarWithJoinToContainer adds an env var to a container, joining with existing value if present.
func (p *PodPatcher) addEnvVarWithJoinToContainer(ctr *corev1.Container, name, value, separator string) {
	for i, existing := range ctr.Env {
		if existing.Name == name {
			// Append to existing value
			ctr.Env[i].Value = existing.Value + separator + value
			return
		}
	}
	// Prepend new env var
	ctr.Env = append([]corev1.EnvVar{{Name: name, Value: value}}, ctr.Env...)
}
