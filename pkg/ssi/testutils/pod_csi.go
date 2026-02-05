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

// csiDriverName is the name of the Datadog CSI driver.
const csiDriverName = "k8s.csi.datadoghq.com"

// csiInjectionValidator validates CSI-based injection.
type csiInjectionValidator struct {
	pod             *PodValidator
	csiVolumes      []corev1.Volume
	injectorVersion string
	libraryVersions map[string]string
}

// newCSIInjectionValidator creates a validator for CSI injection.
func newCSIInjectionValidator(podValidator *PodValidator, pod *corev1.Pod) *csiInjectionValidator {
	csiVolumes := parseCSIVolumes(pod)
	return &csiInjectionValidator{
		pod:             podValidator,
		csiVolumes:      csiVolumes,
		injectorVersion: parseInjectorVersionFromCSI(csiVolumes),
		libraryVersions: parseLibraryVersionsFromCSI(csiVolumes),
	}
}

// RequireContainerInjection validates container has CSI volume mounts.
func (v *csiInjectionValidator) RequireContainerInjection(t *testing.T, container *ContainerValidator) {
	hasCSIMount := false
	for _, mount := range container.raw.VolumeMounts {
		if v.isCSIVolume(mount.Name) {
			hasCSIMount = true
			break
		}
	}
	require.True(t, hasCSIMount, "container %s should have CSI volume mounts", container.raw.Name)
}

// RequireNoContainerInjection validates container does not have CSI volume mounts.
func (v *csiInjectionValidator) RequireNoContainerInjection(t *testing.T, container *ContainerValidator) {
	for _, mount := range container.raw.VolumeMounts {
		require.False(t, v.isCSIVolume(mount.Name),
			"container %s should not have CSI volume mount %s", container.raw.Name, mount.Name)
	}
}

// isCSIVolume checks if a volume name is a CSI volume.
func (v *csiInjectionValidator) isCSIVolume(name string) bool {
	for _, vol := range v.csiVolumes {
		if vol.Name == name {
			return true
		}
	}
	return false
}

// csiExpectedVolume represents an expected CSI volume with its attributes.
type csiExpectedVolume struct {
	name        string
	volType     string
	packageName string
}

// csiExpectedVolumes returns the expected CSI volumes for injection.
func csiExpectedVolumes() []csiExpectedVolume {
	return []csiExpectedVolume{
		{
			name:        "datadog-auto-instrumentation",
			volType:     "DatadogLibrary",
			packageName: "apm-inject",
		},
		{
			name:    "datadog-auto-instrumentation-etc",
			volType: "DatadogInjectorPreload",
		},
	}
}

// RequireInjection validates CSI-based injection.
func (v *csiInjectionValidator) RequireInjection(t *testing.T) {
	// Validate expected CSI volumes exist with correct attributes
	for _, expected := range csiExpectedVolumes() {
		vol := v.findCSIVolumeByName(expected.name)
		require.NotNil(t, vol, "expected CSI volume %s", expected.name)
		require.NotNil(t, vol.CSI.VolumeAttributes, "expected volume attributes on %s", expected.name)
		require.Equal(t, expected.volType, vol.CSI.VolumeAttributes["type"],
			"%s should have type=%s", expected.name, expected.volType)
		if expected.packageName != "" {
			require.Contains(t, vol.CSI.VolumeAttributes["dd.csi.datadog.com/library.package"], expected.packageName,
				"%s should have dd.csi.datadog.com/library.package containing %s", expected.name, expected.packageName)
		}
	}

	// Validate no init containers for library injection (CSI doesn't need them)
	for name := range v.pod.initContainers {
		require.NotContains(t, name, "datadog-lib-",
			"CSI injection should not use init containers for library injection")
		require.NotEqual(t, "datadog-init-apm-inject", name,
			"CSI injection should not use init container for injector")
	}
}

// RequireNoInjection validates that no CSI injection artifacts exist.
func (v *csiInjectionValidator) RequireNoInjection(t *testing.T) {
	// Validate expected CSI volumes do not exist
	for _, expected := range csiExpectedVolumes() {
		vol := v.findCSIVolumeByName(expected.name)
		require.Nil(t, vol, "CSI volume %s should not exist", expected.name)
	}
}

// findCSIVolumeByName finds a CSI volume by name.
func (v *csiInjectionValidator) findCSIVolumeByName(name string) *corev1.Volume {
	for i := range v.csiVolumes {
		if v.csiVolumes[i].Name == name {
			return &v.csiVolumes[i]
		}
	}
	return nil
}

// RequireLibraryVersions validates the injected library versions.
func (v *csiInjectionValidator) RequireLibraryVersions(t *testing.T, expected map[string]string) {
	require.Equal(t, expected, v.libraryVersions, "the injected library versions do not match the expected")
}

// RequireInjectorVersion validates the injector version.
func (v *csiInjectionValidator) RequireInjectorVersion(t *testing.T, expected string) {
	require.Equal(t, expected, v.injectorVersion, "the injector version does not match the expected")
}

// parseCSIVolumes extracts CSI volumes from the pod that use the Datadog CSI driver.
func parseCSIVolumes(pod *corev1.Pod) []corev1.Volume {
	var csiVolumes []corev1.Volume
	for _, vol := range pod.Spec.Volumes {
		if vol.CSI != nil && vol.CSI.Driver == csiDriverName {
			csiVolumes = append(csiVolumes, vol)
		}
	}
	return csiVolumes
}

// parseLibraryVersionsFromCSI extracts library versions from CSI volume attributes.
func parseLibraryVersionsFromCSI(csiVolumes []corev1.Volume) map[string]string {
	versions := map[string]string{}
	for _, vol := range csiVolumes {
		if vol.CSI == nil || vol.CSI.VolumeAttributes == nil {
			continue
		}
		attrs := vol.CSI.VolumeAttributes
		if attrs["type"] != "DatadogLibrary" {
			continue
		}
		// Extract language from volume name (e.g., "dd-lib-python" -> "python")
		if strings.HasPrefix(vol.Name, "dd-lib-") {
			lang := strings.TrimPrefix(vol.Name, "dd-lib-")
			if version, ok := attrs["dd.csi.datadog.com/library.version"]; ok {
				versions[lang] = version
			}
		}
	}
	return versions
}

// parseInjectorVersionFromCSI extracts the injector version from CSI volume attributes.
func parseInjectorVersionFromCSI(csiVolumes []corev1.Volume) string {
	for _, vol := range csiVolumes {
		if vol.CSI == nil || vol.CSI.VolumeAttributes == nil {
			continue
		}
		attrs := vol.CSI.VolumeAttributes
		// The injector volume has the DatadogLibrary type and contains the injector package
		if attrs["type"] == "DatadogLibrary" {
			if pkg, ok := attrs["dd.csi.datadog.com/library.package"]; ok {
				if strings.Contains(pkg, "apm-inject") {
					if version, ok := attrs["dd.csi.datadog.com/library.version"]; ok {
						return version
					}
				}
			}
		}
	}
	return ""
}

// Verify csiInjectionValidator implements InjectionValidator.
var _ InjectionValidator = (*csiInjectionValidator)(nil)
