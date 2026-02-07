// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var (
	// MinimumCPULimit is the minimum CPU limit required for init containers.
	// Below this, copying + library initialization would take too long.
	MinimumCPULimit = resource.MustParse("0.05") // 0.05 core

	// MinimumMemoryLimit is the minimum memory limit required for init containers.
	// This is the recommended minimum by Alpine.
	MinimumMemoryLimit = resource.MustParse("100Mi") // 100 MB

	// MinimumMicroCPULimit is the minimum CPU limit required for the micro init container
	// used by image-volume injection (e.g., a simple `cp`).
	MinimumMicroCPULimit = resource.MustParse("5m")

	// MinimumMicroMemoryLimit is the minimum memory limit required for the micro init container
	// used by image-volume injection (e.g., a simple `cp`).
	MinimumMicroMemoryLimit = resource.MustParse("16Mi")
)

// ResourceRequirementsResult holds the result of computing resource requirements.
type ResourceRequirementsResult struct {
	Requirements corev1.ResourceRequirements
	ShouldSkip   bool
	Message      string
}

// ComputeResourceRequirements computes the resource requirements for init containers
// based on the pod's existing resources and optional default requirements.
// It returns whether injection should be skipped if the pod has insufficient resources.
func ComputeResourceRequirements(pod *corev1.Pod, defaultRequirements map[corev1.ResourceName]resource.Quantity) ResourceRequirementsResult {
	requirements := corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{},
		Requests: corev1.ResourceList{},
	}

	podRequirements := PodSumResourceRequirements(pod)
	insufficientResourcesMessage := "The overall pod's containers limit is too low"
	shouldSkip := false

	for _, k := range [2]corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
		// If a resource quantity was set in config, use it
		if q, ok := defaultRequirements[k]; ok {
			requirements.Limits[k] = q
			requirements.Requests[k] = q
		} else {
			// Otherwise, try to use as much of the resource as we can without impacting pod scheduling
			if maxPodLim, ok := podRequirements.Limits[k]; ok {
				// Check if the pod has sufficient resources
				switch k {
				case corev1.ResourceMemory:
					if MinimumMemoryLimit.Cmp(maxPodLim) == 1 {
						shouldSkip = true
						insufficientResourcesMessage += fmt.Sprintf(", %v pod_limit=%v needed=%v", k, maxPodLim.String(), MinimumMemoryLimit.String())
					}
				case corev1.ResourceCPU:
					if MinimumCPULimit.Cmp(maxPodLim) == 1 {
						shouldSkip = true
						insufficientResourcesMessage += fmt.Sprintf(", %v pod_limit=%v needed=%v", k, maxPodLim.String(), MinimumCPULimit.String())
					}
				default:
					// We don't support other resources
				}
				requirements.Limits[k] = maxPodLim
			}
			if maxPodReq, ok := podRequirements.Requests[k]; ok {
				requirements.Requests[k] = maxPodReq
			}
		}
	}

	if shouldSkip {
		return ResourceRequirementsResult{
			Requirements: requirements,
			ShouldSkip:   true,
			Message:      insufficientResourcesMessage,
		}
	}
	return ResourceRequirementsResult{
		Requirements: requirements,
		ShouldSkip:   false,
		Message:      "",
	}
}

// ComputeMicroInitResourceRequirements computes the resource requirements for the micro init container
// used by the image-volume injection mode.
//
// Unlike ComputeResourceRequirements, the minimums are tuned for a very lightweight operation (copying a file).
func ComputeMicroInitResourceRequirements(pod *corev1.Pod, defaultRequirements map[corev1.ResourceName]resource.Quantity) ResourceRequirementsResult {
	requirements := corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{},
		Requests: corev1.ResourceList{},
	}

	podRequirements := PodSumResourceRequirements(pod)
	insufficientResourcesMessage := "The overall pod's containers limit is too low for image-volume injection"
	shouldSkip := false

	for _, k := range [2]corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
		min := resource.Quantity{}
		switch k {
		case corev1.ResourceCPU:
			min = MinimumMicroCPULimit
		case corev1.ResourceMemory:
			min = MinimumMicroMemoryLimit
		}

		// If a resource quantity was set in config, use it.
		if q, ok := defaultRequirements[k]; ok {
			requirements.Limits[k] = q
			requirements.Requests[k] = q
			if min.Cmp(q) == 1 {
				shouldSkip = true
				insufficientResourcesMessage += fmt.Sprintf(", %v configured=%v needed=%v", k, q.String(), min.String())
			}
			continue
		}

		// Otherwise, try to use as much of the resource as we can without impacting pod scheduling.
		if maxPodLim, ok := podRequirements.Limits[k]; ok {
			if min.Cmp(maxPodLim) == 1 {
				shouldSkip = true
				insufficientResourcesMessage += fmt.Sprintf(", %v pod_limit=%v needed=%v", k, maxPodLim.String(), min.String())
			}
			requirements.Limits[k] = maxPodLim
		}
		if maxPodReq, ok := podRequirements.Requests[k]; ok {
			requirements.Requests[k] = maxPodReq
		}
	}

	if shouldSkip {
		return ResourceRequirementsResult{
			Requirements: requirements,
			ShouldSkip:   true,
			Message:      insufficientResourcesMessage,
		}
	}
	return ResourceRequirementsResult{
		Requirements: requirements,
		ShouldSkip:   false,
		Message:      "",
	}
}

// PodSumResourceRequirements computes the sum of cpu/memory necessary for the whole pod.
// This is computed as max(max(initContainer resources), sum(container resources) + sum(sidecar containers))
// for both limit and request.
// See: https://kubernetes.io/docs/concepts/workloads/pods/sidecar-containers/#resource-sharing-within-containers
func PodSumResourceRequirements(pod *corev1.Pod) corev1.ResourceRequirements {
	requirements := corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{},
		Requests: corev1.ResourceList{},
	}

	for _, k := range [2]corev1.ResourceName{corev1.ResourceMemory, corev1.ResourceCPU} {
		// Take max(initContainer resource)
		maxInitContainerLimit := resource.Quantity{}
		maxInitContainerRequest := resource.Quantity{}
		for i := range pod.Spec.InitContainers {
			c := &pod.Spec.InitContainers[i]
			if InitContainerIsSidecar(c) {
				// Sidecar containers run alongside main containers, so skip here
				continue
			}
			if limit, ok := c.Resources.Limits[k]; ok {
				if limit.Cmp(maxInitContainerLimit) == 1 {
					maxInitContainerLimit = limit
				}
			}
			if request, ok := c.Resources.Requests[k]; ok {
				if request.Cmp(maxInitContainerRequest) == 1 {
					maxInitContainerRequest = request
				}
			}
		}

		// Take sum(container resources) + sum(sidecar containers)
		limitSum := resource.Quantity{}
		reqSum := resource.Quantity{}
		for i := range pod.Spec.Containers {
			c := &pod.Spec.Containers[i]
			if l, ok := c.Resources.Limits[k]; ok {
				limitSum.Add(l)
			}
			if l, ok := c.Resources.Requests[k]; ok {
				reqSum.Add(l)
			}
		}
		for i := range pod.Spec.InitContainers {
			c := &pod.Spec.InitContainers[i]
			if !InitContainerIsSidecar(c) {
				continue
			}
			if l, ok := c.Resources.Limits[k]; ok {
				limitSum.Add(l)
			}
			if l, ok := c.Resources.Requests[k]; ok {
				reqSum.Add(l)
			}
		}

		// Take max(max(initContainer resources), sum(container resources) + sum(sidecar containers))
		if limitSum.Cmp(maxInitContainerLimit) == 1 {
			maxInitContainerLimit = limitSum
		}
		if reqSum.Cmp(maxInitContainerRequest) == 1 {
			maxInitContainerRequest = reqSum
		}

		// Ensure that the limit is greater or equal to the request
		if maxInitContainerRequest.Cmp(maxInitContainerLimit) == 1 {
			maxInitContainerLimit = maxInitContainerRequest
		}

		if maxInitContainerLimit.CmpInt64(0) == 1 {
			requirements.Limits[k] = maxInitContainerLimit
		}
		if maxInitContainerRequest.CmpInt64(0) == 1 {
			requirements.Requests[k] = maxInitContainerRequest
		}
	}

	return requirements
}

// InitContainerIsSidecar returns true if the init container is a sidecar
// (has RestartPolicy set to Always).
func InitContainerIsSidecar(container *corev1.Container) bool {
	return container.RestartPolicy != nil && *container.RestartPolicy == corev1.ContainerRestartPolicyAlways
}
