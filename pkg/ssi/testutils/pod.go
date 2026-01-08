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

// PodValidator provides a test friendly structure to run assertions on pod states for SSI.
type PodValidator struct {
	raw                 *corev1.Pod
	initContainers      map[string]*ContainerValidator
	containers          map[string]*ContainerValidator
	allContainerNames   []string
	injectorVersion     string
	libraryVersions     map[string]string
	initContainerImages []string
	volumes             map[string]corev1.Volume
}

// NewPodValidator initializes a new PodValidator from a Kubernetes pod spec. It creates container validators for
// every container and init container in the pod.
func NewPodValidator(pod *corev1.Pod) *PodValidator {
	containers := make(map[string]*ContainerValidator, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		containers[container.Name] = NewContainerValidator(&container)
	}

	initContainers := make(map[string]*ContainerValidator, len(pod.Spec.InitContainers))
	for _, container := range pod.Spec.InitContainers {
		initContainers[container.Name] = NewContainerValidator(&container)
	}

	return &PodValidator{
		raw:                 pod,
		initContainers:      initContainers,
		containers:          containers,
		allContainerNames:   parseAllContainerNames(pod),
		injectorVersion:     parseInjectorVersion(pod),
		libraryVersions:     parseLibraryVersions(pod),
		initContainerImages: parseInitContainerImages(pod),
		volumes:             newVolumeMap(pod.Spec.Volumes),
	}
}

// RequireInitSecurityContext ensures all Datadog init containers in the pod have the expected security context.
func (v *PodValidator) RequireInitSecurityContext(t *testing.T, expected *corev1.SecurityContext) {
	for name, validator := range v.initContainers {
		if !strings.HasPrefix(name, "datadog-") {
			continue
		}
		validator.RequireSecurityContext(t, expected)
	}
}

// RequireInitResourceRequirements ensures all Datadog init containers in the pod have the expected resource requirements.
func (v *PodValidator) RequireInitResourceRequirements(t *testing.T, expected *corev1.ResourceRequirements) {
	for name, validator := range v.initContainers {
		if !strings.HasPrefix(name, "datadog-") {
			continue
		}

		validator.RequireResourceRequirements(t, expected)
	}
}

// RequireInjection is a high level function that ensures injection has occurred for the pod and expected containers.
// This can and should change when the definition of injection changes.
func (v *PodValidator) RequireInjection(t *testing.T, expectedContainers []string) {
	// Validate the containers are injected that are expected to be.
	for _, containerName := range expectedContainers {
		validator, found := v.containers[containerName]
		require.True(t, found, "invalid test setup, expected container %s does not exist in the pod", containerName)
		validator.RequireInjection(t)
	}

	// Validate the containers not expected to be injected are not.
	notExpected := difference(v.allContainerNames, expectedContainers)
	for _, containerName := range notExpected {
		validator, found := v.containers[containerName]
		require.True(t, found, "invalid test setup, expected container %s does not exist in the pod", containerName)
		validator.RequireNoInjection(t)
	}

	// Validate pod level volumes are created.
	expectedVolumeNames := []string{
		"datadog-auto-instrumentation",
		"datadog-auto-instrumentation-etc",
	}
	v.RequireVolumeNames(t, expectedVolumeNames)

	// Validate pod annotations.
	expectedAnnotations := map[string]string{
		K8sAutoscalerSafeToEvictVolumesAnnotation: "datadog-auto-instrumentation,datadog-auto-instrumentation-etc",
	}
	v.RequireAnnotations(t, expectedAnnotations)

	// Validate injector init container.
	validator, ok := v.initContainers["datadog-init-apm-inject"]
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
		validator, ok := v.initContainers[fmt.Sprintf("datadog-lib-%s-init", lang)]
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

// RequireNoInjection is a high level function that ensures a pod was not injected for SSI.
func (v *PodValidator) RequireNoInjection(t *testing.T) {
	// Validate no container was injected.
	for _, containerValidator := range v.containers {
		containerValidator.RequireNoInjection(t)
	}

	// Validate no volumes were added for injection.
	missingVolumeNames := []string{
		"datadog",
		"datadog-auto-instrumentation",
		"datadog-auto-instrumentation-etc",
	}
	v.RequireMissingVolumeNames(t, missingVolumeNames)
}

// RequireAnnotations ensures the pod has the expected annotations keys and that the values match the expected value.
func (v *PodValidator) RequireAnnotations(t *testing.T, expected map[string]string) {
	for key, expectedValue := range expected {
		actualValue, exists := v.raw.Annotations[key]
		require.True(t, exists, "annotation does not exist", key)
		require.Equal(t, expectedValue, actualValue, "annotation does not match expected value")
	}
}

// RequireVolumeNames ensures the list of volume names exist in the pod.
func (v *PodValidator) RequireVolumeNames(t *testing.T, expected []string) {
	for _, name := range expected {
		_, exists := v.volumes[name]
		require.True(t, exists, "expected volume with name %s does not exist in pod", name)
	}
}

// RequireVolumeNames ensures the list of volume names exist in the pod.
func (v *PodValidator) RequireMissingVolumeNames(t *testing.T, missing []string) {
	for _, name := range missing {
		_, exists := v.volumes[name]
		require.False(t, exists, "volume name %s should not exist in pod", name)
	}
}

// RequireEnvs ensures the expected env vars exist in the expected containers with the expected values.
func (v *PodValidator) RequireEnvs(t *testing.T, expected map[string]string, expectedContainers []string) {
	for _, name := range expectedContainers {
		validator, found := v.containers[name]
		require.True(t, found, "invalid test setup, expected container %s does not exist in the pod", name)
		validator.RequireEnvs(t, expected)
	}
}

// RequireMissingEnvs ensures that the list of missing keys do not exist in any of the containers provided.
func (v *PodValidator) RequireMissingEnvs(t *testing.T, missing []string, expectedContainers []string) {
	for _, name := range expectedContainers {
		validator, found := v.containers[name]
		require.True(t, found, "invalid test setup, expected container %s does not exist in the pod", name)
		validator.RequireMissingEnvs(t, missing)
	}
}

// RequireInjectorVersion is a high level function to ensure the injector version found for the pod matches expected.
func (v *PodValidator) RequireInjectorVersion(t *testing.T, expected string) {
	require.Equal(t, expected, v.injectorVersion, "the injector version does not match the expected")
}

// RequireLibraryVersions ensures the map of library name to version matches what is found in the pod. Ex. python -> v3.
func (v *PodValidator) RequireLibraryVersions(t *testing.T, expected map[string]string) {
	require.Equal(t, expected, v.libraryVersions, "the injected library versions do not match the expected")
}

// RequireInitContainerImages ensures the list of init container image strings matches the expected list.
func (v *PodValidator) RequireInitContainerImages(t *testing.T, expected []string) {
	require.ElementsMatch(t, expected, v.initContainerImages, "init container images do not match expected")
}

func newEnvMap(in []corev1.EnvVar) map[string]string {
	envVars := make(map[string]string, len(in))
	for _, env := range in {
		envVars[env.Name] = env.Value
	}
	return envVars
}

func newVolumeMap(in []corev1.Volume) map[string]corev1.Volume {
	volumes := make(map[string]corev1.Volume, len(in))
	for _, vol := range in {
		volumes[vol.Name] = vol
	}
	return volumes
}

func parseInjectorVersion(pod *corev1.Pod) string {
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

func difference(a, b []string) []string {
	setB := make(map[string]bool, len(b))
	for _, elem := range b {
		setB[elem] = true
	}

	diff := []string{}
	for _, elem := range a {
		if _, found := setB[elem]; !found {
			diff = append(diff, elem)
		}
	}

	return diff
}

func parseAllContainerNames(pod *corev1.Pod) []string {
	names := make([]string, len(pod.Spec.Containers))
	for i, container := range pod.Spec.Containers {
		names[i] = container.Name
	}
	return names
}

func parseInitContainerImages(pod *corev1.Pod) []string {
	images := make([]string, len(pod.Spec.InitContainers))
	for i, container := range pod.Spec.InitContainers {
		images[i] = container.Image
	}
	return images
}

func parseLibraryVersions(pod *corev1.Pod) map[string]string {
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
