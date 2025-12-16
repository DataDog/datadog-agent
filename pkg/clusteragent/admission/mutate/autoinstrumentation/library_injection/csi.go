// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// CSI driver constants.
const (
	// CSIDriverName is the name of the Datadog CSI driver.
	CSIDriverName = "k8s.csi.datadoghq.com"

	// CSIVolumeAttributeType is the key for the volume type attribute.
	CSIVolumeAttributeType = "type"

	// CSIVolumeTypeLibrary is the volume type for mounting OCI image contents.
	// This is used for both the APM injector and language-specific libraries.
	CSIVolumeTypeLibrary = "DatadogLibrary"

	// CSIVolumeTypeInjectorPreload is the volume type for mounting the injector preload file.
	CSIVolumeTypeInjectorPreload = "DatadogInjectorPreload"

	// CSIVolumeAttributeImage is the key for the OCI image to mount.
	CSIVolumeAttributeImage = "dd.csi.datadog.com/library.image"

	// CSIVolumeAttributeSource is the key for the source path within the OCI image.
	CSIVolumeAttributeSource = "dd.csi.datadog.com/library.source"

	// CSIInjectorSourcePath is the source path for the APM injector within its OCI image.
	CSIInjectorSourcePath = "/opt/datadog-packages/datadog-apm-inject"

	// CSILibrarySourcePath is the source path for language libraries within their OCI images.
	CSILibrarySourcePath = "/datadog-init/package"
)

// CSIProvider implements LibraryInjectionProvider using a CSI driver.
// This provider mounts library files directly via CSI volumes without using init containers.
type CSIProvider struct {
	cfg ProviderConfig
}

// NewCSIProvider creates a new CSIProvider.
func NewCSIProvider(cfg ProviderConfig) *CSIProvider {
	return &CSIProvider{
		cfg: cfg,
	}
}

// InjectInjector mutates the pod to add the APM injector using CSI volumes.
func (p *CSIProvider) InjectInjector(pod *corev1.Pod, cfg InjectorConfig) MutationResult {
	mutator := NewPodMutator(pod, p.cfg)

	// CSI volume for the injector image contents
	injectorVolume := corev1.Volume{
		Name: VolumeName,
		VolumeSource: corev1.VolumeSource{
			CSI: &corev1.CSIVolumeSource{
				Driver:   CSIDriverName,
				ReadOnly: pointer.Ptr(true),
				VolumeAttributes: map[string]string{
					CSIVolumeAttributeType:   CSIVolumeTypeLibrary,
					CSIVolumeAttributeImage:  cfg.Image,
					CSIVolumeAttributeSource: CSIInjectorSourcePath,
				},
			},
		},
	}
	mutator.AddVolume(injectorVolume)
	mutator.AddVolumeMount(corev1.VolumeMount{
		Name:      VolumeName,
		MountPath: AsAbsPath(InjectPackageDir),
		ReadOnly:  true,
	})

	// CSI volume for /etc/ld.so.preload
	ldPreloadVolume := corev1.Volume{
		Name: EtcVolumeName,
		VolumeSource: corev1.VolumeSource{
			CSI: &corev1.CSIVolumeSource{
				Driver:   CSIDriverName,
				ReadOnly: pointer.Ptr(true),
				VolumeAttributes: map[string]string{
					CSIVolumeAttributeType: CSIVolumeTypeInjectorPreload,
				},
			},
		},
	}
	mutator.AddVolume(ldPreloadVolume)
	mutator.AddVolumeMount(corev1.VolumeMount{
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
	if !IsLanguageSupported(cfg.Language) {
		return MutationResult{
			Status:  MutationStatusError,
			Message: fmt.Sprintf("language %s is not supported", cfg.Language),
		}
	}

	mutator := NewPodMutator(pod, p.cfg)

	// CSI volume for the library (uses DatadogLibrary type to mount OCI image contents)
	volumeName := fmt.Sprintf("dd-lib-%s", cfg.Language)
	libraryVolume := corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			CSI: &corev1.CSIVolumeSource{
				Driver:   CSIDriverName,
				ReadOnly: pointer.Ptr(true),
				VolumeAttributes: map[string]string{
					CSIVolumeAttributeType:   CSIVolumeTypeLibrary,
					CSIVolumeAttributeImage:  cfg.Image,
					CSIVolumeAttributeSource: CSILibrarySourcePath,
				},
			},
		},
	}
	mutator.AddVolume(libraryVolume)

	// Volume mount for application containers
	mutator.AddVolumeMount(corev1.VolumeMount{
		Name:      volumeName,
		MountPath: AsAbsPath(LibraryPackagesDir) + "/" + cfg.Language,
		ReadOnly:  true,
	})

	return MutationResult{
		Status: MutationStatusInjected,
	}
}

// Verify that CSIProvider implements LibraryInjectionProvider.
var _ LibraryInjectionProvider = (*CSIProvider)(nil)
