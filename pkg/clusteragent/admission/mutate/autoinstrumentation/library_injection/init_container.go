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
	// minimumCPULimit is the minimum CPU limit required for init containers.
	// Below this, copying + library initialization would take too long.
	minimumCPULimit = resource.MustParse("0.05") // 0.05 core

	// minimumMemoryLimit is the minimum memory limit required for init containers.
	// This is the recommended minimum by Alpine.
	minimumMemoryLimit = resource.MustParse("100Mi") // 100 MB
)

// InitContainerProvider implements LibraryInjectionProvider using init containers
// and EmptyDir volumes. This is the traditional injection method where init containers
// copy library files from images into a shared EmptyDir volume.
type InitContainerProvider struct {
	cfg ProviderConfig
	// cachedRequirements stores the computed resource requirements for init containers.
	// This is computed once during InjectInjector and reused for InjectLibrary calls.
	cachedRequirements corev1.ResourceRequirements
}

// NewInitContainerProvider creates a new InitContainerProvider.
func NewInitContainerProvider(cfg ProviderConfig) *InitContainerProvider {
	return &InitContainerProvider{
		cfg: cfg,
	}
}

// InjectInjector mutates the pod to add the APM injector using init containers.
func (p *InitContainerProvider) InjectInjector(pod *corev1.Pod, cfg InjectorConfig) MutationResult {
	// First, validate that the pod has sufficient resources
	requirements, shouldSkip, message := p.computeResourceRequirements(pod)
	if shouldSkip {
		return MutationResult{
			Status:  MutationStatusSkipped,
			Message: message,
		}
	}
	p.cachedRequirements = requirements

	mutator := NewPodMutator(pod, p.cfg)

	// Main volume for library files (EmptyDir)
	sourceVolume := NewEmptyDirVolume(VolumeName)
	mutator.AddVolume(sourceVolume)

	// Volume for /etc/ld.so.preload
	etcVolume := NewEmptyDirVolume(EtcVolumeName)
	mutator.AddVolume(etcVolume)

	// Volume mount for the injector files
	injectorMount := corev1.VolumeMount{
		Name:      VolumeName,
		MountPath: "/datadog-inject",
		SubPath:   InjectPackageDir,
	}

	// Volume mount for /etc directory in init container
	etcMountInitContainer := corev1.VolumeMount{
		Name:      EtcVolumeName,
		MountPath: "/datadog-etc",
	}

	// Volume mount for /etc/ld.so.preload in app containers
	etcMountAppContainer := corev1.VolumeMount{
		Name:      EtcVolumeName,
		MountPath: "/etc/ld.so.preload",
		SubPath:   "ld.so.preload",
		ReadOnly:  true,
	}

	// Add volume mounts to app containers
	mutator.AddVolumeMount(etcMountAppContainer)
	mutator.AddVolumeMount(corev1.VolumeMount{
		Name:      VolumeName,
		MountPath: AsAbsPath(InjectPackageDir),
		SubPath:   InjectPackageDir,
	})

	// Timestamp file path for tracking init container completion
	tsFilePath := injectorMount.MountPath + "/c-init-time." + InjectorInitContainerName

	// Init container that copies injector files
	initContainer := corev1.Container{
		Name:    InjectorInitContainerName,
		Image:   cfg.Image,
		Command: []string{"/bin/sh", "-c", "--"},
		Args: []string{
			fmt.Sprintf(
				`cp -r /%s/* %s && echo %s > /datadog-etc/ld.so.preload && echo $(date +%%s) >> %s`,
				injectorMount.SubPath,
				injectorMount.MountPath,
				AsAbsPath(InjectorFilePath("launcher.preload.so")),
				tsFilePath,
			),
		},
		VolumeMounts: []corev1.VolumeMount{
			injectorMount,
			etcMountInitContainer,
		},
		Resources: requirements,
	}

	// Apply security context - prefer the one from InjectorConfig (namespace-specific),
	// fall back to the one from ProviderConfig (global default)
	if cfg.InitSecurityContext != nil {
		initContainer.SecurityContext = cfg.InitSecurityContext
	} else if p.cfg.InitSecurityContext != nil {
		initContainer.SecurityContext = p.cfg.InitSecurityContext
	}

	mutator.AddInitContainer(initContainer)

	return MutationResult{
		Status: MutationStatusInjected,
	}
}

// InjectLibrary mutates the pod to add a language-specific tracing library using init containers.
func (p *InitContainerProvider) InjectLibrary(pod *corev1.Pod, cfg LibraryConfig) MutationResult {
	if !IsLanguageSupported(cfg.Language) {
		return MutationResult{
			Status:  MutationStatusError,
			Message: fmt.Sprintf("language %s is not supported", cfg.Language),
		}
	}

	mutator := NewPodMutator(pod, p.cfg)

	// Main volume (should already exist from injector, but we add it for completeness)
	sourceVolume := NewEmptyDirVolume(VolumeName)
	mutator.AddVolume(sourceVolume)

	// Mount path for this language's library files
	libraryMountPath := MountPath
	librarySubPath := LibraryPackagesDir + "/" + cfg.Language

	// Volume mount for the library files in the init container
	initContainerMount := corev1.VolumeMount{
		Name:      VolumeName,
		MountPath: libraryMountPath,
		SubPath:   librarySubPath,
	}

	// Volume mount for injector (to write timestamp)
	injectorMount := corev1.VolumeMount{
		Name:      VolumeName,
		MountPath: AsAbsPath(InjectPackageDir),
		SubPath:   InjectPackageDir,
	}

	// Init container name
	containerName := LibraryInitContainerName(cfg.Language)

	// Timestamp file path
	tsFilePath := injectorMount.MountPath + "/c-init-time." + containerName

	// Init container that copies library files
	initContainer := corev1.Container{
		Name:    containerName,
		Image:   cfg.Image,
		Command: []string{"/bin/sh", "-c", "--"},
		Args: []string{
			fmt.Sprintf(
				`sh copy-lib.sh %s && echo $(date +%%s) >> %s`,
				initContainerMount.MountPath,
				tsFilePath,
			),
		},
		VolumeMounts: []corev1.VolumeMount{
			initContainerMount,
			injectorMount,
		},
		Resources: p.cachedRequirements,
	}

	// Apply security context - prefer the one from LibraryConfig (namespace-specific),
	// fall back to the one from ProviderConfig (global default)
	if cfg.InitSecurityContext != nil {
		initContainer.SecurityContext = cfg.InitSecurityContext
	} else if p.cfg.InitSecurityContext != nil {
		initContainer.SecurityContext = p.cfg.InitSecurityContext
	}

	mutator.AddInitContainer(initContainer)

	// Volume mount for application containers
	mutator.AddVolumeMount(corev1.VolumeMount{
		Name:      VolumeName,
		MountPath: AsAbsPath(LibraryPackagesDir),
		SubPath:   LibraryPackagesDir,
	})

	return MutationResult{
		Status: MutationStatusInjected,
	}
}

// computeResourceRequirements computes the resource requirements for init containers.
// It returns the requirements, whether injection should be skipped, and an optional message.
func (p *InitContainerProvider) computeResourceRequirements(pod *corev1.Pod) (corev1.ResourceRequirements, bool, string) {
	requirements := corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{},
		Requests: corev1.ResourceList{},
	}

	podRequirements := podSumResourceRequirements(pod)
	insufficientResourcesMessage := "The overall pod's containers limit is too low"
	shouldSkip := false

	for _, k := range [2]corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
		// If a resource quantity was set in config, use it
		if q, ok := p.cfg.DefaultResourceRequirements[k]; ok {
			requirements.Limits[k] = q
			requirements.Requests[k] = q
		} else {
			// Otherwise, try to use as much of the resource as we can without impacting pod scheduling
			if maxPodLim, ok := podRequirements.Limits[k]; ok {
				// Check if the pod has sufficient resources
				switch k {
				case corev1.ResourceMemory:
					if minimumMemoryLimit.Cmp(maxPodLim) == 1 {
						shouldSkip = true
						insufficientResourcesMessage += fmt.Sprintf(", %v pod_limit=%v needed=%v", k, maxPodLim.String(), minimumMemoryLimit.String())
					}
				case corev1.ResourceCPU:
					if minimumCPULimit.Cmp(maxPodLim) == 1 {
						shouldSkip = true
						insufficientResourcesMessage += fmt.Sprintf(", %v pod_limit=%v needed=%v", k, maxPodLim.String(), minimumCPULimit.String())
					}
				}
				requirements.Limits[k] = maxPodLim
			}
			if maxPodReq, ok := podRequirements.Requests[k]; ok {
				requirements.Requests[k] = maxPodReq
			}
		}
	}

	if shouldSkip {
		return requirements, true, insufficientResourcesMessage
	}
	return requirements, false, ""
}

// podSumResourceRequirements computes the sum of cpu/memory necessary for the whole pod.
// This is computed as max(max(initContainer resources), sum(container resources) + sum(sidecar containers))
// for both limit and request.
// See: https://kubernetes.io/docs/concepts/workloads/pods/sidecar-containers/#resource-sharing-within-containers
func podSumResourceRequirements(pod *corev1.Pod) corev1.ResourceRequirements {
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
			if initContainerIsSidecar(c) {
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
			if !initContainerIsSidecar(c) {
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

// initContainerIsSidecar returns true if the init container is a sidecar container.
func initContainerIsSidecar(container *corev1.Container) bool {
	return container.RestartPolicy != nil && *container.RestartPolicy == corev1.ContainerRestartPolicyAlways
}

// Verify that InitContainerProvider implements LibraryInjectionProvider.
var _ LibraryInjectionProvider = (*InitContainerProvider)(nil)
