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
)

const (
	// InjectorInitContainerName is the name of the APM injector init container.
	InjectorInitContainerName = "datadog-init-apm-inject"
	// LibraryInitContainerNameTemplate is the template for library init container names.
	LibraryInitContainerNameTemplate = "datadog-lib-%s-init"
)

// InitContainerProvider implements LibraryInjectionProvider using init containers
// and EmptyDir volumes. This is the traditional injection method where init containers
// copy library files from images into a shared EmptyDir volume.
type InitContainerProvider struct {
	cfg LibraryInjectionConfig
}

// NewInitContainerProvider creates a new InitContainerProvider.
func NewInitContainerProvider(cfg LibraryInjectionConfig) *InitContainerProvider {
	return &InitContainerProvider{
		cfg: cfg,
	}
}

// InjectInjector mutates the pod to add the APM injector using init containers.
func (p *InitContainerProvider) InjectInjector(pod *corev1.Pod, cfg InjectorConfig) MutationResult {
	// First, validate that the pod has sufficient resources
	result, err := ComputeInitContainerResourceRequirementsForInitContainer(pod, p.cfg.DefaultResourceRequirements, InjectorInitContainerName)
	if err != nil {
		return MutationResult{Status: MutationStatusSkipped, Err: err}
	}
	if result.ShouldSkip {
		return MutationResult{Status: MutationStatusSkipped, Err: errors.New(result.Message)}
	}
	requirements := result.Requirements

	patcher := NewPodPatcher(pod, p.cfg.ContainerFilter)

	// Main volume for library files (EmptyDir)
	sourceVolume := newEmptyDirVolume(InstrumentationVolumeName)
	patcher.AddVolume(sourceVolume)

	// Volume mount for the injector files
	injectorMount := corev1.VolumeMount{
		Name:      InstrumentationVolumeName,
		MountPath: "/datadog-inject",
		SubPath:   injectPackageDir,
	}

	// Add volume mounts to app containers
	etcMountInitContainer := addEtcLdSoPreloadVolumeAndMounts(patcher)
	patcher.AddVolumeMount(corev1.VolumeMount{
		Name:      InstrumentationVolumeName,
		MountPath: asAbsPath(injectPackageDir),
		SubPath:   injectPackageDir,
	})

	// Timestamp file path for tracking init container completion
	tsFilePath := injectorMount.MountPath + "/c-init-time." + InjectorInitContainerName

	// Init container that copies injector files
	initContainer := corev1.Container{
		Name:    InjectorInitContainerName,
		Image:   cfg.Package.FullRef(),
		Command: []string{"/bin/sh", "-c", "--"},
		Args: []string{
			fmt.Sprintf(
				`cp -r /%s/* %s && echo %s > %s/%s && echo $(date +%%s) >> %s`,
				injectorMount.SubPath,
				injectorMount.MountPath,
				asAbsPath(injectorFilePath("launcher.preload.so")),
				etcMountPath,
				ldSoPreloadFileName,
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
	resolvedSecurityContext := resolveInitSecurityContext(p.cfg, pod.Namespace)
	initContainer.SecurityContext = resolvedSecurityContext

	patcher.AddInitContainer(initContainer)

	return MutationResult{
		Status: MutationStatusInjected,
		Context: MutationContext{
			ResourceRequirements: requirements,
			InitSecurityContext:  resolvedSecurityContext,
		},
	}
}

// InjectLibrary mutates the pod to add a language-specific tracing library using init containers.
func (p *InitContainerProvider) InjectLibrary(pod *corev1.Pod, cfg LibraryConfig) MutationResult {
	patcher := NewPodPatcher(pod, p.cfg.ContainerFilter)

	// Main volume (should already exist from injector, but we add it for completeness)
	sourceVolume := newEmptyDirVolume(InstrumentationVolumeName)
	patcher.AddVolume(sourceVolume)

	// Volume mount for the library files in the init container
	initContainerMount := corev1.VolumeMount{
		Name:      InstrumentationVolumeName,
		MountPath: libraryMountPath,
		SubPath:   libraryPackagesDir + "/" + cfg.Language,
	}

	// Volume mount for injector (to write timestamp)
	injectorMount := corev1.VolumeMount{
		Name:      InstrumentationVolumeName,
		MountPath: asAbsPath(injectPackageDir),
		SubPath:   injectPackageDir,
	}

	// Init container name
	containerName := fmt.Sprintf(LibraryInitContainerNameTemplate, cfg.Language)

	// Timestamp file path
	tsFilePath := injectorMount.MountPath + "/c-init-time." + containerName

	// Init container that copies library files
	initContainer := corev1.Container{
		Name:    containerName,
		Image:   cfg.Package.FullRef(),
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
		Resources: cfg.Context.ResourceRequirements,
	}

	// Apply security context from the mutation context (resolved during InjectInjector)
	initContainer.SecurityContext = cfg.Context.InitSecurityContext

	patcher.AddInitContainer(initContainer)

	// Volume mount for application containers
	patcher.AddVolumeMount(corev1.VolumeMount{
		Name:      InstrumentationVolumeName,
		MountPath: asAbsPath(libraryPackagesDir),
		SubPath:   libraryPackagesDir,
	})

	return MutationResult{
		Status: MutationStatusInjected,
	}
}

// Verify that InitContainerProvider implements LibraryInjectionProvider.
var _ LibraryInjectionProvider = (*InitContainerProvider)(nil)
