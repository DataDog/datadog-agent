// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"errors"

	corev1 "k8s.io/api/core/v1"
)

const (
	// librarySubPath is the path where the library files are stored in the library image.
	// See library Dockerfile: https://github.com/DataDog/libdatadog-build/blob/f2325768e60d6bb02e8467f5321b6f9fa10ff850/scripts/lib-injection/Dockerfile.
	librarySubPath = "datadog-init/package"

	// InjectLDPreloadInitContainerName is the name of the "micro" init container used by image_volume injection
	// to copy the ld.so.preload file into the pod's /etc.
	InjectLDPreloadInitContainerName = "datadog-apm-inject-preload"
)

type ImageVolumeProvider struct {
	cfg LibraryInjectionConfig
}

func NewImageVolumeProvider(cfg LibraryInjectionConfig) *ImageVolumeProvider {
	return &ImageVolumeProvider{
		cfg: cfg,
	}
}

func (p *ImageVolumeProvider) InjectInjector(pod *corev1.Pod, cfg InjectorConfig) MutationResult {
	// Validate that the pod has sufficient resources for the micro init container.
	result, err := ComputeInitContainerResourceRequirementsForInitContainer(pod, p.cfg.DefaultResourceRequirements, InjectLDPreloadInitContainerName)
	if err != nil {
		return MutationResult{Status: MutationStatusSkipped, Err: err}
	}
	if result.ShouldSkip {
		return MutationResult{Status: MutationStatusSkipped, Err: errors.New(result.Message)}
	}
	requirements := result.Requirements
	patcher := NewPodPatcher(pod, p.cfg.ContainerFilter)

	// Image volume for the injector image contents
	patcher.AddVolume(corev1.Volume{
		Name: InstrumentationVolumeName,
		VolumeSource: corev1.VolumeSource{
			Image: &corev1.ImageVolumeSource{
				Reference:  cfg.Package.FullRef(),
				PullPolicy: corev1.PullIfNotPresent,
			},
		},
	})

	// Volume mount for the injector files
	injectorMount := corev1.VolumeMount{
		Name:      InstrumentationVolumeName,
		MountPath: asAbsPath(injectPackageDir),
		SubPath:   injectPackageDir,
		ReadOnly:  true,
	}
	patcher.AddVolumeMount(injectorMount)

	etcMountInitContainer := addEtcLdSoPreloadVolumeAndMounts(patcher)

	src := asAbsPath(injectorFilePath("ld.so.preload"))
	dst := etcMountPath + "/" + ldSoPreloadFileName

	// Init container to copy the ld.so.preload file into /etc/ld.so.preload.
	patcher.AddInitContainer(corev1.Container{
		Name:  InjectLDPreloadInitContainerName,
		Image: cfg.Package.FullRef(),
		VolumeMounts: []corev1.VolumeMount{
			injectorMount,
			etcMountInitContainer,
		},
		Command:         []string{"cp", src, dst},
		SecurityContext: resolveInitSecurityContext(p.cfg, pod.Namespace),
		Resources:       requirements,
	})

	return MutationResult{
		Status: MutationStatusInjected,
		Context: MutationContext{
			ResourceRequirements: requirements,
		},
	}
}

func (p *ImageVolumeProvider) InjectLibrary(pod *corev1.Pod, cfg LibraryConfig) MutationResult {
	patcher := NewPodPatcher(pod, p.cfg.ContainerFilter)

	volumeName := "dd-lib-" + cfg.Language
	patcher.AddVolume(corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			Image: &corev1.ImageVolumeSource{
				Reference:  cfg.Package.FullRef(),
				PullPolicy: corev1.PullIfNotPresent,
			},
		},
	})

	patcher.AddVolumeMount(corev1.VolumeMount{
		Name:      volumeName,
		MountPath: asAbsPath(libraryPackagesDir) + "/" + cfg.Language,
		SubPath:   librarySubPath,
		ReadOnly:  true,
	})

	return MutationResult{
		Status: MutationStatusInjected,
	}
}

// Verify that ImageVolumeProvider implements LibraryInjectionProvider.
var _ LibraryInjectionProvider = (*ImageVolumeProvider)(nil)
