// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// imageVolumeInjectionValidator validates Image Volume-based injection.
type imageVolumeInjectionValidator struct {
	pod             *PodValidator
	imageVolumes    []corev1.Volume
	injectorVersion string
	libraryVersions map[string]string
}

func imageVolumeVolumeMounts(languages map[string]string) []corev1.VolumeMount {
	// image_volume injection mounts:
	// - injector image volume at /opt/datadog-packages/datadog-apm-inject (read-only, subPath)
	// - /etc/ld.so.preload from an EmptyDir via SubPath (read-only)
	// - one image volume per injected language under /opt/datadog/apm/library/<lang> (read-only, subPath)
	expected := []corev1.VolumeMount{
		{
			Name:      "datadog-auto-instrumentation",
			MountPath: "/opt/datadog-packages/datadog-apm-inject",
			SubPath:   "opt/datadog-packages/datadog-apm-inject",
			ReadOnly:  true,
		},
		{
			Name:      "datadog-auto-instrumentation-etc",
			MountPath: "/etc/ld.so.preload",
			SubPath:   "ld.so.preload",
			ReadOnly:  true,
		},
	}

	for lang := range languages {
		expected = append(expected, corev1.VolumeMount{
			Name:      "dd-lib-" + lang,
			MountPath: "/opt/datadog/apm/library/" + lang,
			SubPath:   "datadog-init/package",
			ReadOnly:  true,
		})
	}

	return expected
}

// newImageVolumeInjectionValidator creates a validator for Image Volume injection.
func newImageVolumeInjectionValidator(podValidator *PodValidator, pod *corev1.Pod) *imageVolumeInjectionValidator {
	imageVolumes := parseImageVolumes(pod)
	return &imageVolumeInjectionValidator{
		pod:             podValidator,
		imageVolumes:    imageVolumes,
		injectorVersion: parseInjectorVersionFromImageVolumes(imageVolumes),
		libraryVersions: parseLibraryVersionsFromImageVolumes(imageVolumes),
	}
}

// RequireContainerInjection validates container has Image Volume mounts.
func (v *imageVolumeInjectionValidator) RequireContainerInjection(t *testing.T, container *ContainerValidator) {
	container.RequireVolumeMounts(t, imageVolumeVolumeMounts(v.libraryVersions))
}

// RequireNoContainerInjection validates container does not have Image Volume mounts.
func (v *imageVolumeInjectionValidator) RequireNoContainerInjection(t *testing.T, container *ContainerValidator) {
	container.RequireMissingVolumeMounts(t, imageVolumeVolumeMounts(v.libraryVersions))
}

// RequireInjection validates Image Volume-based injection.
func (v *imageVolumeInjectionValidator) RequireInjection(t *testing.T) {
	// Validate pod-level volumes exist.
	// - injector image volume
	// - /etc emptydir volume
	// - one image volume per injected library
	expectedVolumeNames := []string{
		"datadog-auto-instrumentation",
		"datadog-auto-instrumentation-etc",
	}
	for lang := range v.libraryVersions {
		expectedVolumeNames = append(expectedVolumeNames, "dd-lib-"+lang)
	}
	v.pod.RequireVolumeNames(t, expectedVolumeNames)

	// Validate expected init containers:
	// image_volume mode uses a single "micro" init container to copy ld.so.preload, and should not use
	// the injector-copy init container or per-language library init containers.
	_, ok := v.pod.initContainers["datadog-apm-inject-preload"]
	require.True(t, ok, "could not find datadog preload init container")
	for name := range v.pod.initContainers {
		require.NotEqual(t, "datadog-init-apm-inject", name, "image_volume injection should not use injector-copy init container")
		require.False(t, strings.HasPrefix(name, "datadog-lib-"), "image_volume injection should not use library init containers")
	}
}

// RequireNoInjection validates that no Image Volume injection artifacts exist.
func (v *imageVolumeInjectionValidator) RequireNoInjection(t *testing.T) {
	// Validate no image volumes exist.
	require.Empty(t, v.imageVolumes, "image volumes should not exist for no injection")

	// Validate no known Datadog volumes exist.
	v.pod.RequireMissingVolumeNames(t, []string{
		"datadog-auto-instrumentation",
		"datadog-auto-instrumentation-etc",
	})
	for name := range v.pod.volumes {
		require.False(t, strings.HasPrefix(name, "dd-lib-"), "library image volume %s should not exist for no injection", name)
	}

	// Validate no datadog init containers were added.
	for name := range v.pod.initContainers {
		require.False(t, strings.HasPrefix(name, "datadog-"),
			"init container %s should not exist for no injection", name)
	}
}

// RequireLibraryVersions validates the injected library versions.
func (v *imageVolumeInjectionValidator) RequireLibraryVersions(t *testing.T, expected map[string]string) {
	require.Equal(t, expected, v.libraryVersions, "the injected library versions do not match the expected")
}

// RequireInjectorVersion validates the injector version.
func (v *imageVolumeInjectionValidator) RequireInjectorVersion(t *testing.T, expected string) {
	require.Equal(t, expected, v.injectorVersion, "the injector version does not match the expected")
}

// parseImageVolumes extracts Image Volumes from the pod.
func parseImageVolumes(pod *corev1.Pod) []corev1.Volume {
	var imageVolumes []corev1.Volume
	for _, vol := range pod.Spec.Volumes {
		if vol.VolumeSource.Image != nil {
			imageVolumes = append(imageVolumes, vol)
		}
	}
	return imageVolumes
}

func parseInjectorVersionFromImageVolumes(imageVolumes []corev1.Volume) string {
	for _, vol := range imageVolumes {
		if vol.Name != "datadog-auto-instrumentation" || vol.VolumeSource.Image == nil {
			continue
		}
		// Example: gcr.io/datadoghq/apm-inject:0.54.0
		parts := strings.Split(vol.VolumeSource.Image.Reference, ":")
		if len(parts) != 2 {
			return ""
		}
		return parts[1]
	}
	return ""
}

func parseLibraryVersionsFromImageVolumes(imageVolumes []corev1.Volume) map[string]string {
	versions := map[string]string{}
	for _, vol := range imageVolumes {
		if !strings.HasPrefix(vol.Name, "dd-lib-") || vol.VolumeSource.Image == nil {
			continue
		}
		lang := strings.TrimPrefix(vol.Name, "dd-lib-")

		// Example: gcr.io/datadoghq/dd-lib-python-init:v3.18.1
		parts := strings.Split(vol.VolumeSource.Image.Reference, ":")
		if len(parts) != 2 {
			continue
		}
		versions[lang] = parts[1]
	}
	return versions
}

// Verify imageVolumeInjectionValidator implements InjectionValidator.
var _ InjectionValidator = (*imageVolumeInjectionValidator)(nil)
