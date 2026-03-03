// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// K8sAutoscalerSafeToEvictVolumesAnnotation is the annotation used by the
// Kubernetes cluster-autoscaler to mark a volume as safe to evict.
const K8sAutoscalerSafeToEvictVolumesAnnotation = "cluster-autoscaler.kubernetes.io/safe-to-evict-local-volumes"

// initContainerInjectionValidator validates init container-based injection.
type initContainerInjectionValidator struct {
	pod             *PodValidator
	injectorVersion string
	libraryVersions map[string]string
}

// newInitContainerInjectionValidator creates a validator for init container injection.
func newInitContainerInjectionValidator(podValidator *PodValidator, pod *corev1.Pod) *initContainerInjectionValidator {
	return &initContainerInjectionValidator{
		pod:             podValidator,
		injectorVersion: parseInjectorVersionFromInitContainers(pod),
		libraryVersions: parseLibraryVersionsFromInitContainers(pod),
	}
}

// initContainerVolumeMounts returns the expected volume mounts for init container injection.
func initContainerVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "datadog-auto-instrumentation",
			MountPath: "/opt/datadog-packages/datadog-apm-inject",
			SubPath:   "opt/datadog-packages/datadog-apm-inject",
		},
		{
			Name:      "datadog-auto-instrumentation-etc",
			MountPath: "/etc/ld.so.preload",
			SubPath:   "ld.so.preload",
			ReadOnly:  true,
		},
		{
			Name:      "datadog-auto-instrumentation",
			MountPath: "/opt/datadog/apm/library",
			SubPath:   "opt/datadog/apm/library",
		},
	}
}

// RequireContainerInjection validates container volume mounts for init container injection.
func (v *initContainerInjectionValidator) RequireContainerInjection(t *testing.T, container *ContainerValidator) {
	container.RequireVolumeMounts(t, initContainerVolumeMounts())
}

// RequireNoContainerInjection validates container does not have init container injection volume mounts.
func (v *initContainerInjectionValidator) RequireNoContainerInjection(t *testing.T, container *ContainerValidator) {
	container.RequireMissingVolumeMounts(t, initContainerVolumeMounts())
}

// RequireInjection validates init container-based injection.
func (v *initContainerInjectionValidator) RequireInjection(t *testing.T) {
	// Validate pod level volumes are created.
	expectedVolumeNames := []string{
		"datadog-auto-instrumentation",
		"datadog-auto-instrumentation-etc",
	}
	v.pod.RequireVolumeNames(t, expectedVolumeNames)

	// Validate pod annotations.
	expectedAnnotations := map[string]string{
		K8sAutoscalerSafeToEvictVolumesAnnotation: "datadog-auto-instrumentation,datadog-auto-instrumentation-etc",
	}
	v.pod.RequireAnnotations(t, expectedAnnotations)

	// Validate injector init container.
	validator, ok := v.pod.initContainers["datadog-init-apm-inject"]
	require.True(t, ok, "could not find datadog inject init container")
	expectedVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "datadog-auto-instrumentation",
			MountPath: "/datadog-inject",
			SubPath:   "opt/datadog-packages/datadog-apm-inject",
		},
		{
			Name:      "datadog-auto-instrumentation-etc",
			MountPath: "/datadog-etc",
			SubPath:   "",
		},
	}
	validator.RequireVolumeMounts(t, expectedVolumeMounts)
	validator.RequireCommand(t, "/bin/sh -c -- cp -r /opt/datadog-packages/datadog-apm-inject/* /datadog-inject && echo /opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so > /datadog-etc/ld.so.preload && echo $(date +%s) >> /datadog-inject/c-init-time.datadog-init-apm-inject")

	// Validate library init containers.
	for lang := range v.libraryVersions {
		validator, ok := v.pod.initContainers[fmt.Sprintf("datadog-lib-%s-init", lang)]
		require.True(t, ok, "could not find datadog library init container", lang)
		expectedVolumeMounts := []corev1.VolumeMount{
			{
				Name:      "datadog-auto-instrumentation",
				MountPath: "/datadog-lib",
				SubPath:   "opt/datadog/apm/library/" + lang,
			},
			{
				Name:      "datadog-auto-instrumentation",
				MountPath: "/opt/datadog-packages/datadog-apm-inject",
				SubPath:   "opt/datadog-packages/datadog-apm-inject",
			},
		}
		validator.RequireVolumeMounts(t, expectedVolumeMounts)
		validator.RequireCommand(t, "/bin/sh -c -- sh copy-lib.sh /datadog-lib && echo $(date +%s) >> /opt/datadog-packages/datadog-apm-inject/c-init-time.datadog-lib-"+lang+"-init")
	}
}

// RequireNoInjection validates that no init container injection artifacts exist.
func (v *initContainerInjectionValidator) RequireNoInjection(t *testing.T) {
	// Validate no init container volumes were added.
	missingVolumeNames := []string{
		"datadog-auto-instrumentation",
		"datadog-auto-instrumentation-etc",
	}
	v.pod.RequireMissingVolumeNames(t, missingVolumeNames)

	// Validate no datadog init containers were added.
	for name := range v.pod.initContainers {
		require.False(t, strings.HasPrefix(name, "datadog-"),
			"init container %s should not exist for no injection", name)
	}
}

// RequireLibraryVersions validates the injected library versions.
func (v *initContainerInjectionValidator) RequireLibraryVersions(t *testing.T, expected map[string]string) {
	require.Equal(t, expected, v.libraryVersions, "the injected library versions do not match the expected")
}

// RequireInjectorVersion validates the injector version.
func (v *initContainerInjectionValidator) RequireInjectorVersion(t *testing.T, expected string) {
	require.Equal(t, expected, v.injectorVersion, "the injector version does not match the expected")
}

// parseInjectorVersionFromInitContainers extracts the injector version from init containers.
func parseInjectorVersionFromInitContainers(pod *corev1.Pod) string {
	for _, container := range pod.Spec.InitContainers {
		if container.Name == "datadog-init-apm-inject" {
			parts := strings.Split(container.Image, ":")
			if len(parts) != 2 {
				continue
			}
			return parts[1]
		}
	}
	return ""
}

// parseLibraryVersionsFromInitContainers extracts library versions from init container images.
func parseLibraryVersionsFromInitContainers(pod *corev1.Pod) map[string]string {
	injectedVersions := map[string]string{}

	for _, container := range pod.Spec.InitContainers {
		// gcr.io/datadoghq/dd-lib-java-init:v1
		parts := strings.Split(container.Image, ":")
		if len(parts) != 2 {
			continue
		}
		fullImage := parts[0]
		version := parts[1]

		// gcr.io/datadoghq/dd-lib-java-init
		parts = strings.Split(fullImage, "/")
		if len(parts) < 1 {
			continue
		}
		image := parts[len(parts)-1]

		// dd-lib-java-init
		parts = strings.Split(image, "-")
		if len(parts) != 4 {
			continue
		}

		// java
		lib := parts[2]
		injectedVersions[lib] = version
	}

	return injectedVersions
}

// Verify initContainerInjectionValidator implements InjectionValidator.
var _ InjectionValidator = (*initContainerInjectionValidator)(nil)
