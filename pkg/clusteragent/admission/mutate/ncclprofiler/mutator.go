// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package ncclprofiler

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	// soVolumeName is the emptyDir volume name for the injected .so.
	soVolumeName = "datadog-nccl-profiler"

	// soMountPath is the directory where the .so volume is mounted.
	soMountPath = "/datadog-nccl"

	// soDestPath is the full in-container path to the .so after injection.
	soDestPath = "/datadog-nccl/libnccl-profiler-inspector.so"

	// soSourcePath is where the .so lives inside the injector image.
	soSourcePath = "/libnccl-profiler-inspector.so"

	// socketVolumeName is the hostPath volume name for the Datadog agent socket.
	socketVolumeName = "datadog-socket"

	// socketHostPath is the host directory that contains the agent's Unix socket.
	socketHostPath = "/var/run/datadog"

	// socketMountPath is where the agent socket directory is mounted in containers.
	socketMountPath = "/var/run/datadog"
)

// mutatePod injects the NCCL profiler plugin into pod by:
//  1. Adding an emptyDir volume for the .so file.
//  2. Prepending an init container that copies the .so from the injector image.
//  3. Mounting the .so volume and the agent socket into every app container.
//  4. Setting NCCL_PROFILER_PLUGIN and NCCL_DD_SOCKET_PATH env vars.
func mutatePod(pod *corev1.Pod, injectorImage string) (bool, error) {
	// 1. EmptyDir volume for the .so (written by init container, read by app containers).
	addVolume(pod, corev1.Volume{
		Name:         soVolumeName,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	})

	// 2. HostPath volume for the Datadog agent socket directory.
	addVolume(pod, corev1.Volume{
		Name: socketVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{Path: socketHostPath},
		},
	})

	// 3. Prepend init container: copies .so from injector image to emptyDir.
	initContainer := corev1.Container{
		Name:    "datadog-nccl-profiler-inject",
		Image:   injectorImage,
		Command: []string{"cp", soSourcePath, soDestPath},
		VolumeMounts: []corev1.VolumeMount{
			{Name: soVolumeName, MountPath: soMountPath}, // writable: init container writes here
		},
	}
	alreadyInjected := false
	for _, c := range pod.Spec.InitContainers {
		if c.Name == initContainer.Name {
			alreadyInjected = true
			break
		}
	}
	if !alreadyInjected {
		pod.Spec.InitContainers = append([]corev1.Container{initContainer}, pod.Spec.InitContainers...)
	}

	// 4. Inject mounts and NCCL env vars into every app container.
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		addVolumeMount(c, corev1.VolumeMount{Name: soVolumeName, MountPath: soMountPath, ReadOnly: true})
		addVolumeMount(c, corev1.VolumeMount{Name: socketVolumeName, MountPath: socketMountPath, ReadOnly: true})
		addEnv(c, corev1.EnvVar{Name: "NCCL_PROFILER_PLUGIN", Value: soDestPath})
		addEnv(c, corev1.EnvVar{Name: "NCCL_DD_SOCKET_PATH", Value: "/var/run/datadog/nccl.socket"})
	}

	return true, nil
}

// addVolume appends vol to pod.Spec.Volumes if a volume with the same name does not exist.
func addVolume(pod *corev1.Pod, vol corev1.Volume) {
	for _, v := range pod.Spec.Volumes {
		if v.Name == vol.Name {
			return
		}
	}
	pod.Spec.Volumes = append(pod.Spec.Volumes, vol)
}

// addVolumeMount appends mount to c.VolumeMounts if no existing mount has the same name or path.
func addVolumeMount(c *corev1.Container, mount corev1.VolumeMount) {
	for _, m := range c.VolumeMounts {
		if m.Name == mount.Name || m.MountPath == mount.MountPath {
			return
		}
	}
	c.VolumeMounts = append(c.VolumeMounts, mount)
}

// addEnv appends env to c.Env if no existing env var has the same name.
func addEnv(c *corev1.Container, env corev1.EnvVar) {
	for _, e := range c.Env {
		if e.Name == env.Name {
			return
		}
	}
	c.Env = append(c.Env, env)
}
