// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package sidecar contains shared logic for building AppSec processor sidecar containers
package sidecar

import (
	"fmt"
	"path"
	"slices"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

const sidecarContainerName = "datadog-appsec-processor"

const (
	// SidecarContainerName is the name of the UDS ext_proc sidecar container.
	SidecarContainerName = "datadog-appsec"
	// SharedSocketVolumeName is the name of the shared emptyDir volume for the UDS socket.
	SharedSocketVolumeName = "datadog-appsec-uds"
)

// BuildExtProcProcessorContainer creates the appsec processor sidecar container
func BuildExtProcProcessorContainer(config appsecconfig.Sidecar) corev1.Container {
	image := config.Image
	if config.ImageTag != "" {
		image = image + ":" + config.ImageTag
	}

	env := []corev1.EnvVar{
		{
			Name:  "DD_SERVICE_EXTENSION_TLS",
			Value: "false", // TLS disabled for localhost communication
		},
		{
			Name:  "DD_SERVICE_EXTENSION_PORT",
			Value: strconv.Itoa(config.Port),
		},
		{
			Name:  "DD_SERVICE_EXTENSION_HEALTHCHECK_PORT",
			Value: strconv.Itoa(config.HealthPort),
		},
	}

	if config.BodyParsingSizeLimit != "" {
		env = append(env, corev1.EnvVar{
			Name:  "DD_APPSEC_BODY_PARSING_SIZE_LIMIT",
			Value: config.BodyParsingSizeLimit,
		})
	}

	limits := corev1.ResourceList{}
	if config.CPULimit != "" {
		limits[corev1.ResourceCPU] = resource.MustParse(config.CPULimit)
	}

	if config.MemoryLimit != "" {
		limits[corev1.ResourceMemory] = resource.MustParse(config.MemoryLimit)
	}

	requests := corev1.ResourceList{}
	if config.CPURequest != "" {
		requests[corev1.ResourceCPU] = resource.MustParse(config.CPURequest)
	}

	if config.MemoryRequest != "" {
		requests[corev1.ResourceMemory] = resource.MustParse(config.MemoryRequest)
	}

	return corev1.Container{
		Name:  sidecarContainerName,
		Image: image,
		Ports: []corev1.ContainerPort{
			{
				Name:          "grpc",
				ContainerPort: int32(config.Port),
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "health",
				ContainerPort: int32(config.HealthPort),
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: env,
		Resources: corev1.ResourceRequirements{
			Requests: requests,
			Limits:   limits,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/",
					Port: intstr.FromInt32(int32(config.HealthPort)),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/",
					Port: intstr.FromInt32(int32(config.HealthPort)),
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       20,
		},
	}
}

// HasProcessorSidecar checks if a pod already has the processor sidecar
func HasProcessorSidecar(pod *corev1.Pod) bool {
	return slices.ContainsFunc(pod.Spec.Containers, func(container corev1.Container) bool {
		return container.Name == sidecarContainerName
	})
}

// BuildExtProcProcessorContainerUDS creates the AppSec processor sidecar container for UDS mode.
// Unlike the TCP variant, it binds the ext_proc gRPC listener to a Unix domain socket and does
// not expose a gRPC TCP port. The health port remains a regular TCP/HTTP endpoint.
func BuildExtProcProcessorContainerUDS(config appsecconfig.Sidecar) corev1.Container {
	image := config.Image
	if config.ImageTag != "" {
		image = image + ":" + config.ImageTag
	}

	env := []corev1.EnvVar{
		{
			Name:  "DD_SERVICE_EXTENSION_UDS_PATH",
			Value: config.UDSPath,
		},
		{
			Name:  "DD_SERVICE_EXTENSION_TLS",
			Value: "false",
		},
		{
			Name:  "DD_SERVICE_EXTENSION_HEALTHCHECK_PORT",
			Value: strconv.Itoa(config.HealthPort),
		},
		{
			Name:  "DD_SERVICE_EXTENSION_OBSERVABILITY_MODE",
			Value: "false",
		},
		{
			Name:  "DD_APM_TRACING_ENABLED",
			Value: "false",
		},
	}

	if config.BodyParsingSizeLimit != "" {
		env = append(env, corev1.EnvVar{
			Name:  "DD_APPSEC_BODY_PARSING_SIZE_LIMIT",
			Value: config.BodyParsingSizeLimit,
		})
	}

	limits := corev1.ResourceList{}
	if config.CPULimit != "" {
		limits[corev1.ResourceCPU] = resource.MustParse(config.CPULimit)
	}
	if config.MemoryLimit != "" {
		limits[corev1.ResourceMemory] = resource.MustParse(config.MemoryLimit)
	}

	requests := corev1.ResourceList{}
	if config.CPURequest != "" {
		requests[corev1.ResourceCPU] = resource.MustParse(config.CPURequest)
	}
	if config.MemoryRequest != "" {
		requests[corev1.ResourceMemory] = resource.MustParse(config.MemoryRequest)
	}

	return corev1.Container{
		Name:  SidecarContainerName,
		Image: image,
		Ports: []corev1.ContainerPort{
			{
				Name:          "health",
				ContainerPort: int32(config.HealthPort),
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: env,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      SharedSocketVolumeName,
				MountPath: path.Dir(config.UDSPath),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:                ptr.To(config.RunAsUser),
			RunAsGroup:               ptr.To(config.RunAsUser),
			RunAsNonRoot:             ptr.To(true),
			AllowPrivilegeEscalation: ptr.To(false),
		},
		Resources: corev1.ResourceRequirements{
			Requests: requests,
			Limits:   limits,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/",
					Port: intstr.FromInt32(int32(config.HealthPort)),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/",
					Port: intstr.FromInt32(int32(config.HealthPort)),
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       20,
		},
	}
}

// EnsureSharedSocketVolume appends the shared emptyDir UDS volume to the pod if absent.
// Idempotent: calling it twice does not duplicate the volume.
// Returns SharedSocketVolumeName.
func EnsureSharedSocketVolume(pod *corev1.Pod) string {
	for _, v := range pod.Spec.Volumes {
		if v.Name == SharedSocketVolumeName {
			return SharedSocketVolumeName
		}
	}
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: SharedSocketVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})
	return SharedSocketVolumeName
}

// MountSocketIntoContainer adds a VolumeMount for volumeName at mountDir to the named container.
// Returns an error if the container is not found. Idempotent: does not add a duplicate mount.
func MountSocketIntoContainer(pod *corev1.Pod, containerName, volumeName, mountDir string) error {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name != containerName {
			continue
		}
		for _, vm := range pod.Spec.Containers[i].VolumeMounts {
			if vm.MountPath == mountDir {
				if vm.Name == volumeName {
					return nil
				}
				return fmt.Errorf("mount path %q in container %q is already used by volume %q", mountDir, containerName, vm.Name)
			}
		}
		pod.Spec.Containers[i].VolumeMounts = append(pod.Spec.Containers[i].VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: mountDir,
		})
		return nil
	}
	return fmt.Errorf("container %q not found in pod", containerName)
}

// EnsureSocketFSGroup sets the pod security context FSGroup to gid if it is not already set.
// Also sets FSGroupChangePolicy to OnRootMismatch when the FSGroup is first applied.
// Does not clobber a pre-existing FSGroup value. Idempotent.
func EnsureSocketFSGroup(pod *corev1.Pod, gid int64) {
	if pod.Spec.SecurityContext == nil {
		pod.Spec.SecurityContext = &corev1.PodSecurityContext{}
	}
	if pod.Spec.SecurityContext.FSGroup != nil {
		return
	}
	policy := corev1.FSGroupChangeOnRootMismatch
	pod.Spec.SecurityContext.FSGroup = &gid
	pod.Spec.SecurityContext.FSGroupChangePolicy = &policy
}
