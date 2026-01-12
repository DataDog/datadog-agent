// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// injectorInitContainerName is the name of the APM injector init container.
	injectorInitContainerName = "datadog-init-apm-inject"
	// libraryInitContainerNameTemplate is the template for library init container names.
	libraryInitContainerNameTemplate = "datadog-lib-%s-init"
)

var (
	// minimumCPULimit is the minimum CPU limit required for init containers.
	// Below this, copying + library initialization would take too long.
	minimumCPULimit = resource.MustParse("0.05") // 0.05 core

	// minimumMemoryLimit is the minimum memory limit required for init containers.
	// This is the recommended minimum by Alpine.
	minimumMemoryLimit = resource.MustParse("100Mi") // 100 MB

	// defaultRestrictedSecurityContext is the security context used for init containers
	// in namespaces with the "restricted" Pod Security Standard.
	// https://datadoghq.atlassian.net/browse/INPLAT-492
	defaultRestrictedSecurityContext = &corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr.To(false),
		RunAsNonRoot:             ptr.To(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}
)

// initContainerProvider implements libraryInjectionProvider using init containers
// and EmptyDir volumes. This is the traditional injection method where init containers
// copy library files from images into a shared EmptyDir volume.
type initContainerProvider struct {
	cfg LibraryInjectionConfig
}

// newInitContainerProvider creates a new initContainerProvider.
func newInitContainerProvider(cfg LibraryInjectionConfig) *initContainerProvider {
	return &initContainerProvider{
		cfg: cfg,
	}
}

// injectInjector mutates the pod to add the APM injector using init containers.
func (p *initContainerProvider) injectInjector(pod *corev1.Pod, cfg InjectorConfig) mutationResult {
	// First, validate that the pod has sufficient resources
	requirements, shouldSkip, message := p.computeResourceRequirements(pod)
	if shouldSkip {
		return mutationResult{
			status: mutationStatusSkipped,
			err:    errors.New(message),
		}
	}
	patcher := newPodPatcher(pod, p.cfg.ContainerFilter)

	// Main volume for library files (EmptyDir)
	sourceVolume := newEmptyDirVolume(instrumentationVolumeName)
	patcher.AddVolume(sourceVolume)

	// Volume for /etc/ld.so.preload
	etcVolume := newEmptyDirVolume(etcVolumeName)
	patcher.AddVolume(etcVolume)

	// Volume mount for the injector files
	injectorMount := corev1.VolumeMount{
		Name:      instrumentationVolumeName,
		MountPath: "/datadog-inject",
		SubPath:   injectPackageDir,
	}

	// Volume mount for /etc directory in init container
	etcMountInitContainer := corev1.VolumeMount{
		Name:      etcVolumeName,
		MountPath: "/datadog-etc",
	}

	// Volume mount for /etc/ld.so.preload in app containers
	etcMountAppContainer := corev1.VolumeMount{
		Name:      etcVolumeName,
		MountPath: "/etc/ld.so.preload",
		SubPath:   "ld.so.preload",
		ReadOnly:  true,
	}

	// Add volume mounts to app containers
	patcher.AddVolumeMount(etcMountAppContainer)
	patcher.AddVolumeMount(corev1.VolumeMount{
		Name:      instrumentationVolumeName,
		MountPath: asAbsPath(injectPackageDir),
		SubPath:   injectPackageDir,
	})

	// Timestamp file path for tracking init container completion
	tsFilePath := injectorMount.MountPath + "/c-init-time." + injectorInitContainerName

	// Init container that copies injector files
	initContainer := corev1.Container{
		Name:    injectorInitContainerName,
		Image:   cfg.Image,
		Command: []string{"/bin/sh", "-c", "--"},
		Args: []string{
			fmt.Sprintf(
				`cp -r /%s/* %s && echo %s > /datadog-etc/ld.so.preload && echo $(date +%%s) >> %s`,
				injectorMount.SubPath,
				injectorMount.MountPath,
				asAbsPath(injectorFilePath("launcher.preload.so")),
				tsFilePath,
			),
		},
		VolumeMounts: []corev1.VolumeMount{
			injectorMount,
			etcMountInitContainer,
		},
		Resources: requirements,
	}

	// Resolve security context based on namespace labels and config
	resolvedSecurityContext := p.resolveInitSecurityContext(pod.Namespace)
	initContainer.SecurityContext = resolvedSecurityContext

	patcher.AddInitContainer(initContainer)

	return mutationResult{
		status: mutationStatusInjected,
		context: mutationContext{
			resourceRequirements: requirements,
			initSecurityContext:  resolvedSecurityContext,
		},
	}
}

// injectLibrary mutates the pod to add a language-specific tracing library using init containers.
func (p *initContainerProvider) injectLibrary(pod *corev1.Pod, cfg LibraryConfig) mutationResult {
	if !isLanguageSupported(cfg.Language) {
		return mutationResult{
			status: mutationStatusError,
			err:    fmt.Errorf("language %s is not supported", cfg.Language),
		}
	}

	patcher := newPodPatcher(pod, p.cfg.ContainerFilter)

	// Main volume (should already exist from injector, but we add it for completeness)
	sourceVolume := newEmptyDirVolume(instrumentationVolumeName)
	patcher.AddVolume(sourceVolume)

	// Volume mount for the library files in the init container
	initContainerMount := corev1.VolumeMount{
		Name:      instrumentationVolumeName,
		MountPath: libraryMountPath,
		SubPath:   libraryPackagesDir + "/" + cfg.Language,
	}

	// Volume mount for injector (to write timestamp)
	injectorMount := corev1.VolumeMount{
		Name:      instrumentationVolumeName,
		MountPath: asAbsPath(injectPackageDir),
		SubPath:   injectPackageDir,
	}

	// Init container name
	containerName := fmt.Sprintf(libraryInitContainerNameTemplate, cfg.Language)

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
		Resources: cfg.Context.resourceRequirements,
	}

	// Apply security context from the mutation context (resolved during injectInjector)
	initContainer.SecurityContext = cfg.Context.initSecurityContext

	patcher.AddInitContainer(initContainer)

	// Volume mount for application containers
	patcher.AddVolumeMount(corev1.VolumeMount{
		Name:      instrumentationVolumeName,
		MountPath: asAbsPath(libraryPackagesDir),
		SubPath:   libraryPackagesDir,
	})

	return mutationResult{
		status: mutationStatusInjected,
	}
}

// computeResourceRequirements computes the resource requirements for init containers.
// It returns the requirements, whether injection should be skipped, and an optional message.
func (p *initContainerProvider) computeResourceRequirements(pod *corev1.Pod) (corev1.ResourceRequirements, bool, string) {
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

// resolveInitSecurityContext determines the appropriate security context for init containers
// based on namespace labels and global configuration.
func (p *initContainerProvider) resolveInitSecurityContext(nsName string) *corev1.SecurityContext {
	// Use the configured security context if provided
	if p.cfg.InitSecurityContext != nil {
		return p.cfg.InitSecurityContext
	}

	// If wmeta is not available, we can't check namespace labels
	if p.cfg.Wmeta == nil {
		return nil
	}

	// Check namespace labels for Pod Security Standard
	id := util.GenerateKubeMetadataEntityID("", "namespaces", "", nsName)
	ns, err := p.cfg.Wmeta.GetKubernetesMetadata(id)
	if err != nil {
		log.Warnf("error getting labels for namespace=%s: %s", nsName, err)
		return nil
	}

	if val, ok := ns.EntityMeta.Labels["pod-security.kubernetes.io/enforce"]; ok && val == "restricted" {
		return defaultRestrictedSecurityContext
	}

	return nil
}

// initContainerIsSidecar returns true if the init container is a sidecar container.
func initContainerIsSidecar(container *corev1.Container) bool {
	return container.RestartPolicy != nil && *container.RestartPolicy == corev1.ContainerRestartPolicyAlways
}

// Verify that initContainerProvider implements libraryInjectionProvider.
var _ libraryInjectionProvider = (*initContainerProvider)(nil)
