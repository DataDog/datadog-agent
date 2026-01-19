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
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// InjectorInitContainerName is the name of the APM injector init container.
	InjectorInitContainerName = "datadog-init-apm-inject"
	// LibraryInitContainerNameTemplate is the template for library init container names.
	LibraryInitContainerNameTemplate = "datadog-lib-%s-init"
)

var (
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
	result := ComputeResourceRequirements(pod, p.cfg.DefaultResourceRequirements)
	if result.ShouldSkip {
		return MutationResult{
			Status: MutationStatusSkipped,
			Err:    errors.New(result.Message),
		}
	}
	requirements := result.Requirements

	patcher := NewPodPatcher(pod, p.cfg.ContainerFilter)

	// Main volume for library files (EmptyDir)
	sourceVolume := newEmptyDirVolume(InstrumentationVolumeName)
	patcher.AddVolume(sourceVolume)

	// Volume for /etc/ld.so.preload
	etcVolume := newEmptyDirVolume(EtcVolumeName)
	patcher.AddVolume(etcVolume)

	// Volume mount for the injector files
	injectorMount := corev1.VolumeMount{
		Name:      InstrumentationVolumeName,
		MountPath: "/datadog-inject",
		SubPath:   injectPackageDir,
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
	patcher.AddVolumeMount(etcMountAppContainer)
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

// resolveInitSecurityContext determines the appropriate security context for init containers
// based on namespace labels and global configuration.
func (p *InitContainerProvider) resolveInitSecurityContext(nsName string) *corev1.SecurityContext {
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

// Verify that InitContainerProvider implements LibraryInjectionProvider.
var _ LibraryInjectionProvider = (*InitContainerProvider)(nil)
