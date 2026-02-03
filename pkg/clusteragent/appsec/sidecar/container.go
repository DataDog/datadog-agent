// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package sidecar contains shared logic for building AppSec processor sidecar containers
package sidecar

import (
	"slices"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

const sidecarContainerName = "datadog-appsec-processor"

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
