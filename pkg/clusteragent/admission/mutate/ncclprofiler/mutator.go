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
)

// mutatePod injects the NCCL profiler plugin into pod by:
//  1. Adding an emptyDir volume for the .so files.
//  2. Prepending an init container that copies both .so files from the injector image.
//  3. Mounting the .so volume and the agent socket directory into every app container.
//  4. Setting NCCL env vars (incl. NCCL_DD_SOCKET_PATH from the agent config).
//
// hostSocketPath is the host directory containing the agent's Unix socket
// (mounted into pods at the same path). socketPath is the full in-container
// socket path the wrapper connects to. Both come from gpu.nccl.{host_socket_path,socket_path}.
// initResources is the optional resource requirements applied to the injected
// init container. nil means no Resources block is set (cluster default applies);
// operators with a LimitRange or strict QoS requirements override via
// admission_controller.nccl_profiler.init_resources.{cpu,memory}.
//
// Pod-level opt-in policy (label + mutate_unlabelled) is enforced by the
// webhook objectSelector at the K8s API server, not re-checked here.
func mutatePod(pod *corev1.Pod, injectorImage, hostSocketPath, socketPath string, initResources *corev1.ResourceRequirements) (bool, error) {
	soVolume := corev1.Volume{
		Name:         soVolumeName,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}
	soMount := corev1.VolumeMount{Name: soVolumeName, MountPath: soMountPath, ReadOnly: true}

	hostPathType := corev1.HostPathDirectoryOrCreate
	socketVolume := corev1.Volume{
		Name: socketVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{Path: hostSocketPath, Type: &hostPathType},
		},
	}
	socketMount := corev1.VolumeMount{Name: socketVolumeName, MountPath: hostSocketPath, ReadOnly: true}

	// Inject volumes + mounts into all app containers using shared helpers.
	soVolAdded, soMountAdded := mutatecommon.InjectVolume(pod, soVolume, soMount)
	sockVolAdded, sockMountAdded := mutatecommon.InjectVolume(pod, socketVolume, socketMount)

	// Inject NCCL env vars into all app containers.
	envAdded := mutatecommon.InjectEnv(pod, corev1.EnvVar{Name: "NCCL_PROFILER_PLUGIN", Value: soDestPath})
	envAdded = mutatecommon.InjectEnv(pod, corev1.EnvVar{Name: "NCCL_DD_SOCKET_PATH", Value: socketPath}) || envAdded
	envAdded = mutatecommon.InjectEnv(pod, corev1.EnvVar{Name: "NCCL_DD_INSPECTOR_PATH", Value: soMountPath + "/libnccl-profiler-inspector.so"}) || envAdded
	envAdded = mutatecommon.InjectEnv(pod, corev1.EnvVar{Name: "NCCL_INSPECTOR_ENABLE", Value: "1"}) || envAdded

	// Prepend init container that copies both .so files from injector image.
	// SecurityContext drops all capabilities + disallows privilege escalation so
	// the container passes the "restricted" PodSecurity standard. Resource
	// requirements are operator-supplied (nil = cluster default applies).
	allowPrivEsc := false
	initContainer := corev1.Container{
		Name:            "datadog-nccl-profiler-inject",
		Image:           injectorImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command: []string{"sh", "-c",
			"cp " + soSourcePathWrapper + " " + soMountPath + "/ && " +
				"cp " + soSourcePathInspector + " " + soMountPath + "/"},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &allowPrivEsc,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: soVolumeName, MountPath: soMountPath}, // writable for init container
		},
	}
	if initResources != nil {
		initContainer.Resources = *initResources
	}
	alreadyInjected := false
	for _, c := range pod.Spec.InitContainers {
		if c.Name == initContainer.Name {
			alreadyInjected = true
			break
		}
	}
	initAdded := false
	if !alreadyInjected {
		pod.Spec.InitContainers = append([]corev1.Container{initContainer}, pod.Spec.InitContainers...)
		initAdded = true
	}

	mutated := soVolAdded || soMountAdded || sockVolAdded || sockMountAdded || envAdded || initAdded
	return mutated, nil
}
