// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// withEnvOverrides applies the extraEnv overrides to the container. Returns a
// boolean that indicates if the container was mutated
func withEnvOverrides(container *corev1.Container, extraEnv ...corev1.EnvVar) (bool, error) {
	if container == nil {
		return false, errors.New("can't apply environment overrides to nil container")
	}

	mutated := false

	for _, envVarOverride := range extraEnv {
		// Check if the environment variable already exists in the container
		var found bool
		for i, envVar := range container.Env {
			if envVar.Name == envVarOverride.Name {
				// Override the existing environment variable value
				container.Env[i] = envVarOverride
				found = true
				if envVar.Value != envVarOverride.Value {
					mutated = true
				}
				break
			}
		}
		// If the environment variable doesn't exist, add it to the container
		if !found {
			container.Env = append(container.Env, envVarOverride)
			mutated = true
		}
	}

	return mutated, nil
}

// withResourceLimits applies the resource limits overrides to the container
func withResourceLimits(container *corev1.Container, resourceLimits corev1.ResourceRequirements) error {
	if container == nil {
		return errors.New("can't apply resource requirements overrides to nil container")
	}
	container.Resources = resourceLimits
	return nil
}

// parseAnnotationResourceOverrides parses per-pod resource override annotations
// and returns the resulting ResourceRequirements. It returns nil if no resource
// annotations are present. It returns an error if any annotation value is invalid
// or if request > limit for any resource type.
func parseAnnotationResourceOverrides(annotations map[string]string) (*corev1.ResourceRequirements, error) {
	if annotations == nil {
		return nil, nil
	}

	cpuReqStr, hasCPUReq := annotations[annotationSidecarCPURequest]
	cpuLimStr, hasCPULim := annotations[annotationSidecarCPULimit]
	memReqStr, hasMemReq := annotations[annotationSidecarMemoryRequest]
	memLimStr, hasMemLim := annotations[annotationSidecarMemoryLimit]

	// No resource annotations present
	if !hasCPUReq && !hasCPULim && !hasMemReq && !hasMemLim {
		return nil, nil
	}

	resources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	// Parse CPU annotations
	if hasCPUReq || hasCPULim {
		var cpuReq, cpuLim resource.Quantity
		var err error

		if hasCPUReq {
			cpuReq, err = resource.ParseQuantity(cpuReqStr)
			if err != nil {
				return nil, fmt.Errorf("invalid %s annotation value %q: %s", annotationSidecarCPURequest, cpuReqStr, err)
			}
			if cpuReq.Cmp(resource.MustParse("0")) < 0 {
				return nil, fmt.Errorf("negative %s annotation value %q", annotationSidecarCPURequest, cpuReqStr)
			}
		}
		if hasCPULim {
			cpuLim, err = resource.ParseQuantity(cpuLimStr)
			if err != nil {
				return nil, fmt.Errorf("invalid %s annotation value %q: %s", annotationSidecarCPULimit, cpuLimStr, err)
			}
			if cpuLim.Cmp(resource.MustParse("0")) < 0 {
				return nil, fmt.Errorf("negative %s annotation value %q", annotationSidecarCPULimit, cpuLimStr)
			}
		}

		// Fill missing side: limit-only → request=limit, request-only → limit=request
		if hasCPULim && !hasCPUReq {
			cpuReq = cpuLim
		} else if hasCPUReq && !hasCPULim {
			cpuLim = cpuReq
		}

		// Validate request <= limit
		if cpuReq.Cmp(cpuLim) > 0 {
			return nil, fmt.Errorf("CPU request (%s) exceeds limit (%s)", cpuReq.String(), cpuLim.String())
		}

		resources.Requests[corev1.ResourceCPU] = cpuReq
		resources.Limits[corev1.ResourceCPU] = cpuLim
	}

	// Parse memory annotations
	if hasMemReq || hasMemLim {
		var memReq, memLim resource.Quantity
		var err error

		if hasMemReq {
			memReq, err = resource.ParseQuantity(memReqStr)
			if err != nil {
				return nil, fmt.Errorf("invalid %s annotation value %q: %s", annotationSidecarMemoryRequest, memReqStr, err)
			}
			if memReq.Cmp(resource.MustParse("0")) < 0 {
				return nil, fmt.Errorf("negative %s annotation value %q", annotationSidecarMemoryRequest, memReqStr)
			}
		}
		if hasMemLim {
			memLim, err = resource.ParseQuantity(memLimStr)
			if err != nil {
				return nil, fmt.Errorf("invalid %s annotation value %q: %s", annotationSidecarMemoryLimit, memLimStr, err)
			}
			if memLim.Cmp(resource.MustParse("0")) < 0 {
				return nil, fmt.Errorf("negative %s annotation value %q", annotationSidecarMemoryLimit, memLimStr)
			}
		}

		// Fill missing side: limit-only → request=limit, request-only → limit=request
		if hasMemLim && !hasMemReq {
			memReq = memLim
		} else if hasMemReq && !hasMemLim {
			memLim = memReq
		}

		// Validate request <= limit
		if memReq.Cmp(memLim) > 0 {
			return nil, fmt.Errorf("memory request (%s) exceeds limit (%s)", memReq.String(), memLim.String())
		}

		resources.Requests[corev1.ResourceMemory] = memReq
		resources.Limits[corev1.ResourceMemory] = memLim
	}

	return resources, nil
}

// applyAnnotationResourceOverrides reads per-pod resource annotations and applies
// them to the sidecar container. Returns true if the container was mutated.
// If annotations are invalid, it logs a warning and returns false (no error),
// allowing the caller to fall back to profile/default resources.
func applyAnnotationResourceOverrides(pod *corev1.Pod, container *corev1.Container) (bool, error) {
	if pod == nil {
		return false, errors.New("can't apply annotation overrides to nil pod")
	}
	if container == nil {
		return false, errors.New("can't apply annotation overrides to nil container")
	}

	resources, err := parseAnnotationResourceOverrides(pod.Annotations)
	if err != nil {
		log.Errorf("Invalid sidecar resource annotations on pod %s, ignoring annotations: %v", pod.Name, err)
		return false, nil
	}

	if resources == nil {
		return false, nil
	}

	// Deep-copy the existing resource maps before writing to avoid mutating
	// shared profile state. withResourceLimits aliases container.Resources to
	// the webhook-global profileOverrides maps, so in-place writes here would
	// leak one pod's annotation overrides into subsequent pods and risk
	// concurrent map writes.
	newRequests := make(corev1.ResourceList)
	for k, v := range container.Resources.Requests {
		newRequests[k] = v
	}
	newLimits := make(corev1.ResourceList)
	for k, v := range container.Resources.Limits {
		newLimits[k] = v
	}

	// Merge annotation overrides into the copied maps.
	// This allows overriding only CPU or only memory while keeping the other from profile/defaults.
	if cpu, ok := resources.Requests[corev1.ResourceCPU]; ok {
		newRequests[corev1.ResourceCPU] = cpu
	}
	if cpu, ok := resources.Limits[corev1.ResourceCPU]; ok {
		newLimits[corev1.ResourceCPU] = cpu
	}
	if mem, ok := resources.Requests[corev1.ResourceMemory]; ok {
		newRequests[corev1.ResourceMemory] = mem
	}
	if mem, ok := resources.Limits[corev1.ResourceMemory]; ok {
		newLimits[corev1.ResourceMemory] = mem
	}

	container.Resources.Requests = newRequests
	container.Resources.Limits = newLimits

	return true, nil
}

// withSecurityContextOverrides applies the security context overrides to the container
func withSecurityContextOverrides(container *corev1.Container, securityContext *corev1.SecurityContext) (bool, error) {
	if container == nil {
		return false, errors.New("can't apply security context overrides to nil container")
	}

	mutated := false

	if securityContext != nil {
		container.SecurityContext = securityContext
		mutated = true
	}

	return mutated, nil
}
