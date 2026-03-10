// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package ncclprofiler

import (
	corev1 "k8s.io/api/core/v1"

	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

const (
	// soVolumeName is the emptyDir volume name for the injected .so files.
	soVolumeName = "datadog-nccl-profiler"

	// soMountPath is the directory where the .so volume is mounted.
	soMountPath = "/datadog-nccl"

	// soDestPath is the full in-container path to the wrapper .so after injection.
	soDestPath = "/datadog-nccl/libnccl-profiler-dd.so"

	// soSourcePathWrapper is where the wrapper .so lives inside the injector image.
	soSourcePathWrapper = "/libnccl-profiler-dd.so"

	// soSourcePathInspector is where the Inspector .so lives inside the injector image.
	soSourcePathInspector = "/libnccl-profiler-inspector.so"

	// socketVolumeName is the hostPath volume name for the Datadog agent socket.
	socketVolumeName = "datadog-socket"

	// socketHostPath is the host directory that contains the agent's Unix socket.
	socketHostPath = "/var/run/datadog"

	// socketMountPath is where the agent socket directory is mounted in containers.
	socketMountPath = "/var/run/datadog"
)

// mutatePod injects the NCCL profiler plugin into pod by:
//  1. Adding an emptyDir volume for the .so files.
//  2. Prepending an init container that copies both .so files from the injector image.
//  3. Mounting the .so volume and the agent socket into every app container.
//  4. Setting NCCL env vars.
func mutatePod(pod *corev1.Pod, injectorImage string) (bool, error) {
	soVolume := corev1.Volume{
		Name:         soVolumeName,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}
	soMount := corev1.VolumeMount{Name: soVolumeName, MountPath: soMountPath, ReadOnly: true}

	socketVolume := corev1.Volume{
		Name: socketVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{Path: socketHostPath},
		},
	}
	socketMount := corev1.VolumeMount{Name: socketVolumeName, MountPath: socketMountPath, ReadOnly: true}

	// Inject volumes + mounts into all app containers using shared helpers.
	mutatecommon.InjectVolume(pod, soVolume, soMount)
	mutatecommon.InjectVolume(pod, socketVolume, socketMount)

	// Inject NCCL env vars into all app containers.
	mutatecommon.InjectEnv(pod, corev1.EnvVar{Name: "NCCL_PROFILER_PLUGIN", Value: soDestPath})
	mutatecommon.InjectEnv(pod, corev1.EnvVar{Name: "NCCL_DD_SOCKET_PATH", Value: "/var/run/datadog/nccl.socket"})
	mutatecommon.InjectEnv(pod, corev1.EnvVar{Name: "NCCL_INSPECTOR_ENABLE", Value: "1"})

	// Prepend init container that copies both .so files from injector image.
	initContainer := corev1.Container{
		Name:  "datadog-nccl-profiler-inject",
		Image: injectorImage,
		Command: []string{"sh", "-c",
			"cp " + soSourcePathWrapper + " " + soMountPath + "/ && " +
				"cp " + soSourcePathInspector + " " + soMountPath + "/"},
		VolumeMounts: []corev1.VolumeMount{
			{Name: soVolumeName, MountPath: soMountPath}, // writable for init container
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

	return true, nil
}
