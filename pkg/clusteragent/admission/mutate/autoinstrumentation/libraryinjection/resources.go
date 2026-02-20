// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var (
	// minimumCPULimit is the minimum CPU limit required for init containers.
	// Below this, copying + library initialization would take too long.
	minimumCPULimit = resource.MustParse("0.05") // 0.05 core

	// minimumMemoryLimit is the minimum memory limit required for init containers.
	// This is the recommended minimum by Alpine.
	minimumMemoryLimit = resource.MustParse("100Mi") // 100 MB

	// minimumMicroCPULimit is the minimum CPU limit required for the micro init container
	// used by image_volume injection (e.g., a simple `cp`).
	minimumMicroCPULimit = resource.MustParse("5m")

	// minimumMicroMemoryLimit is the minimum memory limit required for the micro init container
	// used by image_volume injection (e.g., a simple `cp`).
	minimumMicroMemoryLimit = resource.MustParse("16Mi")
)

// ResourceRequirementsResult holds the result of computing resource requirements.
type ResourceRequirementsResult struct {
	Requirements corev1.ResourceRequirements
	ShouldSkip   bool
	Message      string
}

// ComputeInitContainerResourceRequirementsForInitContainer computes init container resource requirements
// for a specific init-container type, identified by its name.
// Returns an error if initContainerName is unknown (programming error).
func ComputeInitContainerResourceRequirementsForInitContainer(
	pod *corev1.Pod,
	defaultRequirements map[corev1.ResourceName]resource.Quantity,
	initContainerName string,
) (ResourceRequirementsResult, error) {
	switch initContainerName {
	case InjectorInitContainerName:
		// NOTE: init_container injection mode historically treats configured init_resources as an explicit override,
		// so we do not enforce minimums on configured values (for now).
		return computeInitContainerResourceRequirements(pod, defaultRequirements, minimumCPULimit, minimumMemoryLimit, false), nil
	case InjectLDPreloadInitContainerName:
		// The image_volume micro init container is a critical prerequisite, so enforce minimums even when configured.
		return computeInitContainerResourceRequirements(pod, defaultRequirements, minimumMicroCPULimit, minimumMicroMemoryLimit, true), nil
	default:
		if isLibraryInitContainerName(initContainerName) {
			return computeInitContainerResourceRequirements(pod, defaultRequirements, minimumCPULimit, minimumMemoryLimit, false), nil
		}
		return ResourceRequirementsResult{}, fmt.Errorf("unknown init container name %q for resource requirement computation", initContainerName)
	}
}

// isLibraryInitContainerName returns true if name matches the naming convention used for language library init containers
// (LibraryInitContainerNameTemplate), e.g. "datadog-lib-java-init".
func isLibraryInitContainerName(name string) bool {
	return strings.HasPrefix(name, "datadog-lib-") && strings.HasSuffix(name, "-init")
}

func computeInitContainerResourceRequirements(pod *corev1.Pod, defaultRequirements map[corev1.ResourceName]resource.Quantity, minCPULimit resource.Quantity, minMemoryLimit resource.Quantity, enforceMinimumsOnConfigured bool) ResourceRequirementsResult {
	requirements := corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{},
		Requests: corev1.ResourceList{},
	}

	podRequirements := PodSumResourceRequirements(pod)
	baseMessage := "The overall pod's containers limit is too low for injection"
	var insufficientResourcesMessage strings.Builder
	insufficientResourcesMessage.WriteString(baseMessage)
	shouldSkip := false

	for _, k := range [2]corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
		min := resource.Quantity{}
		switch k {
		case corev1.ResourceCPU:
			min = minCPULimit
		case corev1.ResourceMemory:
			min = minMemoryLimit
		}

		// If a resource quantity was set in config, use it.
		if q, ok := defaultRequirements[k]; ok {
			requirements.Limits[k] = q
			requirements.Requests[k] = q
			if enforceMinimumsOnConfigured && min.Cmp(q) == 1 {
				shouldSkip = true
				_, _ = fmt.Fprintf(&insufficientResourcesMessage, ", %v configured=%v needed=%v", k, q.String(), min.String())
			}
			continue
		}

		// Otherwise, try to use as much of the resource as we can without impacting pod scheduling.
		if maxPodLim, ok := podRequirements.Limits[k]; ok {
			if min.Cmp(maxPodLim) == 1 {
				shouldSkip = true
				_, _ = fmt.Fprintf(&insufficientResourcesMessage, ", %v pod_limit=%v needed=%v", k, maxPodLim.String(), min.String())
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
			Message:      insufficientResourcesMessage.String(),
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
