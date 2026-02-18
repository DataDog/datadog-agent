// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// CSI driver constants.
const (
	// csiDriverName is the name of the Datadog CSI driver.
	csiDriverName = "k8s.csi.datadoghq.com"

	// csiVolumeAttributeType is the key for the volume type attribute.
	csiVolumeAttributeType = "type"

	// csiVolumeTypeLibrary is the volume type for mounting OCI image contents.
	// This is used for both the APM injector and language-specific libraries.
	csiVolumeTypeLibrary = "DatadogLibrary"

	// csiVolumeTypeInjectorPreload is the volume type for mounting the injector preload file.
	csiVolumeTypeInjectorPreload = "DatadogInjectorPreload"

	// csiVolumeAttributePackage is the key for the package name to mount.
	csiVolumeAttributePackage = "dd.csi.datadog.com/library.package"

	// csiVolumeAttributeRegistry is the key for the OCI registry.
	csiVolumeAttributeRegistry = "dd.csi.datadog.com/library.registry"

	// csiVolumeAttributeVersion is the key for the package version.
	csiVolumeAttributeVersion = "dd.csi.datadog.com/library.version"
)

// CSIProvider implements LibraryInjectionProvider using a CSI driver.
// This provider mounts library files directly via CSI volumes without using init containers.
type CSIProvider struct {
	cfg LibraryInjectionConfig
}

// NewCSIProvider creates a new CSIProvider.
func NewCSIProvider(cfg LibraryInjectionConfig) *CSIProvider {
	return &CSIProvider{
		cfg: cfg,
	}
}

// InjectInjector mutates the pod to add the APM injector using CSI volumes.
func (p *CSIProvider) InjectInjector(pod *corev1.Pod, cfg InjectorConfig) MutationResult {
	patcher := NewPodPatcher(pod, p.cfg.ContainerFilter)

	// CSI volume for the injector image contents
	patcher.AddVolume(corev1.Volume{
		Name: InstrumentationVolumeName,
		VolumeSource: corev1.VolumeSource{
			CSI: &corev1.CSIVolumeSource{
				Driver:   csiDriverName,
				ReadOnly: ptr.To(true),
				VolumeAttributes: map[string]string{
					csiVolumeAttributeType:     csiVolumeTypeLibrary,
					csiVolumeAttributePackage:  cfg.Package.Name,
					csiVolumeAttributeRegistry: cfg.Package.Registry,
					csiVolumeAttributeVersion:  cfg.Package.Version,
				},
			},
		},
	})
	patcher.AddVolumeMount(corev1.VolumeMount{
		Name:      InstrumentationVolumeName,
		MountPath: asAbsPath(injectPackageDir),
		ReadOnly:  true,
	})

	// CSI volume for /etc/ld.so.preload
	patcher.AddVolume(corev1.Volume{
		Name: EtcVolumeName,
		VolumeSource: corev1.VolumeSource{
			CSI: &corev1.CSIVolumeSource{
				Driver:   csiDriverName,
				ReadOnly: ptr.To(true),
				VolumeAttributes: map[string]string{
					csiVolumeAttributeType: csiVolumeTypeInjectorPreload,
				},
			},
		},
	})
	patcher.AddVolumeMount(corev1.VolumeMount{
		Name:      EtcVolumeName,
		MountPath: "/etc/ld.so.preload",
		ReadOnly:  true,
	})

	return MutationResult{
		Status: MutationStatusInjected,
	}
}

// InjectLibrary mutates the pod to add a language-specific tracing library using CSI volumes.
func (p *CSIProvider) InjectLibrary(pod *corev1.Pod, cfg LibraryConfig) MutationResult {
	patcher := NewPodPatcher(pod, p.cfg.ContainerFilter)

	// CSI volume for the library (uses DatadogLibrary type to mount OCI image contents)
	volumeName := "dd-lib-" + cfg.Language
	patcher.AddVolume(corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			CSI: &corev1.CSIVolumeSource{
				Driver:   csiDriverName,
				ReadOnly: ptr.To(true),
				VolumeAttributes: map[string]string{
					csiVolumeAttributeType:     csiVolumeTypeLibrary,
					csiVolumeAttributePackage:  cfg.Package.Name,
					csiVolumeAttributeRegistry: cfg.Package.Registry,
					csiVolumeAttributeVersion:  cfg.Package.Version,
				},
			},
		},
	})
	patcher.AddVolumeMount(corev1.VolumeMount{
		Name:      volumeName,
		MountPath: asAbsPath(libraryPackagesDir) + "/" + cfg.Language,
		ReadOnly:  true,
	})

	return MutationResult{
		Status: MutationStatusInjected,
	}
}

// Verify that CSIProvider implements LibraryInjectionProvider.
var _ LibraryInjectionProvider = (*CSIProvider)(nil)
