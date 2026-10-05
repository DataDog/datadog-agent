// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
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

// GetName returns the injection mode for this provider.
func (p *CSIProvider) GetName() string {
	return string(InjectionModeCSI)
}

// PlanInjector mutates the pod to add the APM injector using CSI volumes.
func (p *CSIProvider) PlanInjector(session *patch.PodSession, cfg InjectorConfig) MutationResult {
	patcher := NewPodPatcher(session, p.cfg.ContainerFilter)

	// CSI volume for the injector image contents

	if err := patcher.AddVolume(corev1.Volume{
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
	}); err != nil {
		return MutationResult{Status: MutationStatusError, Err: err}
	}

	if err := patcher.AddVolumeMount(corev1.VolumeMount{
		Name:      InstrumentationVolumeName,
		MountPath: asAbsPath(injectPackageDir),
		ReadOnly:  true,
	}); err != nil {
		return MutationResult{Status: MutationStatusError, Err: err}
	}

	// CSI volume for /etc/ld.so.preload

	if err := patcher.AddVolume(corev1.Volume{
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
	}); err != nil {
		return MutationResult{Status: MutationStatusError, Err: err}
	}

	if err := patcher.AddVolumeMount(corev1.VolumeMount{
		Name:      EtcVolumeName,
		MountPath: "/etc/ld.so.preload",
		ReadOnly:  true,
	}); err != nil {
		return MutationResult{Status: MutationStatusError, Err: err}
	}

	return MutationResult{
		Status: MutationStatusInjected,
	}
}

// PlanLibrary mutates the pod to add a language-specific tracing library using CSI volumes.
func (p *CSIProvider) PlanLibrary(session *patch.PodSession, cfg LibraryConfig) MutationResult {
	patcher := NewPodPatcher(session, p.cfg.ContainerFilter)

	// CSI volume for the library (uses DatadogLibrary type to mount OCI image contents)
	volumeName := "dd-lib-" + cfg.Language

	if err := patcher.AddVolume(corev1.Volume{
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
	}); err != nil {
		return MutationResult{Status: MutationStatusError, Err: err}
	}

	if err := patcher.AddVolumeMountWithTarget(corev1.VolumeMount{
		Name:      volumeName,
		MountPath: asAbsPath(libraryPackagesDir) + "/" + cfg.Language,
		ReadOnly:  true,
	}, cfg.ContainerName); err != nil {
		return MutationResult{Status: MutationStatusError, Err: err}
	}

	return MutationResult{
		Status: MutationStatusInjected,
	}
}

// Verify that CSIProvider implements LibraryInjectionProvider.
var _ LibraryInjectionProvider = (*CSIProvider)(nil)
