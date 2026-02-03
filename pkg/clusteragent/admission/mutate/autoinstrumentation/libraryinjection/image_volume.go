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

	// // temporarily use InitContainerProvider to inject the injector
	// initContainerProvider := NewInitContainerProvider(p.cfg)
	// return initContainerProvider.InjectInjector(pod, cfg)

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

	// Shared volume for ld.so.preload
	etcVolume := newEmptyDirVolume(EtcVolumeName)
	patcher.AddVolume(etcVolume)

	// Volume mount for /etc directory in init container
	etcMountInitContainer := corev1.VolumeMount{
		Name:      EtcVolumeName,
		MountPath: "/datadog-etc",
	}

	// Copy the ld.so.preload file into /etc/ld.so.preload.
	patcher.AddInitContainer(corev1.Container{
		Name:    "copy-ld-so-preload",
		Image:   cfg.Package.FullRef(),
		Command: []string{"cp"},
		// TODO: Define constants for the source and dest paths.
		Args: []string{
			injectorMount.MountPath + "/stable/inject/ld.so.preload", etcMountInitContainer.MountPath + "/ld.so.preload",
		},
		VolumeMounts: []corev1.VolumeMount{
			etcMountInitContainer,
			injectorMount,
		},
	}) // TODO: Decide whether to include the ts file for tracking the completion of the init container.

	// Volume mount for /etc/ld.so.preload in app containers
	etcMountAppContainer := corev1.VolumeMount{
		Name:      EtcVolumeName,
		MountPath: "/etc/ld.so.preload",
		SubPath:   "ld.so.preload",
		ReadOnly:  true,
	}
	patcher.AddVolumeMount(etcMountAppContainer)

	// Mount the injector-provided ld.so.preload file into /etc/ld.so.preload.
	// Note: this relies on the injector package layout inside the image.
	// patcher.AddVolumeMount(corev1.VolumeMount{
	// 	Name:      InstrumentationVolumeName,
	// 	MountPath: "/etc/ld.so.preload",
	// 	SubPath:   injectPackageDir + "/stable/inject/ld.so.preload",
	// 	ReadOnly:  true,
	// })

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
