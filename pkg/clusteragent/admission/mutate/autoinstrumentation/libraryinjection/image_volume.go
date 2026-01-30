// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	// librarySubPath is the path where the library files are stored in the library image.
	// See library Dockerfile: https://github.com/DataDog/libdatadog-build/blob/f2325768e60d6bb02e8467f5321b6f9fa10ff850/scripts/lib-injection/Dockerfile.
	librarySubPath = "datadog-init/package"
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
	patcher.AddVolumeMount(corev1.VolumeMount{
		Name:      InstrumentationVolumeName,
		MountPath: asAbsPath(injectPackageDir),
		SubPath:   injectPackageDir,
		ReadOnly:  true,
	})

	// Mount the injector-provided ld.so.preload file into /etc/ld.so.preload.
	// Note: this relies on the injector package layout inside the image.
	patcher.AddVolumeMount(corev1.VolumeMount{
		Name:      InstrumentationVolumeName,
		MountPath: "/etc/ld.so.preload",
		SubPath:   injectPackageDir + "/stable/inject/ld.so.preload",
		ReadOnly:  true,
	})

	// Is it possible for any of the patcher operations to fail? If so, how do we report it?
	return MutationResult{
		Status: MutationStatusInjected,
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

	// Is it possible for any of the patcher operations to fail? If so, how do we report it?
	return MutationResult{
		Status: MutationStatusInjected,
	}
}

// Verify that InitContainerProvider implements LibraryInjectionProvider.
var _ LibraryInjectionProvider = (*ImageVolumeProvider)(nil)
