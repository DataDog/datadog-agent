// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// InjectionMode represents the method used to inject APM libraries into pods.
type InjectionMode string

const (
	// InjectionModeAuto uses init containers (the current default injection method).
	InjectionModeAuto InjectionMode = "auto"
	// InjectionModeInitContainer uses init containers to copy library files.
	InjectionModeInitContainer InjectionMode = "init_container"
	// InjectionModeCSI uses the Datadog CSI driver to mount library files.
	InjectionModeCSI InjectionMode = "csi"
	// InjectionModeImageVolume uses image volumes to mount library files.
	InjectionModeImageVolume InjectionMode = "image_volume"
)

// Annotation keys written by the admission webhook to record the outcome of an
// injection attempt. They are duplicated here (rather than imported) because
// the source of truth in
// pkg/clusteragent/admission/mutate/autoinstrumentation/annotation is built
// behind the "kubeapiserver" build tag, which these test utilities must not
// require.
const (
	// EffectiveInjectionModeAnnotation records the injection mode actually used by the webhook.
	EffectiveInjectionModeAnnotation = "internal.apm.datadoghq.com/effective-injection-mode"
	// CSIDriverStatusAnnotation records the observed Datadog CSI driver state; only set when CSI detection is active.
	CSIDriverStatusAnnotation = "internal.apm.datadoghq.com/csi-driver-status"
	// InjectionStatusAnnotation records the overall outcome of the injection attempt.
	InjectionStatusAnnotation = "internal.apm.datadoghq.com/injection-status"
	// InjectedLibrariesAnnotation records the JSON array of components the webhook attempted to inject.
	InjectedLibrariesAnnotation = "internal.apm.datadoghq.com/injected-libraries"
)

// CSIDriverStatus annotation values (kept in sync with the annotation package).
const (
	// CSIDriverStatusAPMEnabled means the Datadog CSI driver is installed and advertises APM SSI support.
	CSIDriverStatusAPMEnabled = "apm-enabled"
	// CSIDriverStatusAPMDisabled means the driver is installed but APM SSI support is not advertised.
	CSIDriverStatusAPMDisabled = "apm-disabled"
	// CSIDriverStatusNotInstalled means the Datadog CSI driver object was not found in the cluster.
	CSIDriverStatusNotInstalled = "not-installed"
)

// InjectionStatus annotation values (kept in sync with the annotation package).
const (
	// InjectionStatusInjected means the injector and all requested libraries were successfully injected.
	InjectionStatusInjected = "injected"
	// InjectionStatusPartial means the injector succeeded but at least one library was skipped.
	InjectionStatusPartial = "partial"
	// InjectionStatusSkipped means injection was skipped before it started.
	InjectionStatusSkipped = "skipped"
	// InjectionStatusError means a fatal error prevented the injector from running.
	InjectionStatusError = "error"
)

// InjectionValidator validates injection-specific aspects of a pod.
// It also implements ContainerInjectionValidator for container-level validation.
type InjectionValidator interface {
	ContainerInjectionValidator
	// RequireInjection validates mode-specific pod-level injection (volumes, annotations, init containers).
	RequireInjection(t *testing.T)
	// RequireNoInjection validates that no mode-specific injection artifacts exist.
	RequireNoInjection(t *testing.T)
	// RequireLibraryVersions validates the injected library versions.
	RequireLibraryVersions(t *testing.T, expected map[string]string)
	// RequireInjectorVersion validates the injector version.
	RequireInjectorVersion(t *testing.T, expected string)
}

// PodValidator provides a test friendly structure to run assertions on pod states for SSI.
type PodValidator struct {
	raw                 *corev1.Pod
	initContainers      map[string]*ContainerValidator
	containers          map[string]*ContainerValidator
	allContainerNames   []string
	initContainerImages []string
	volumes             map[string]corev1.Volume
	injection           InjectionValidator
}

// NewPodValidator initializes a new PodValidator from a Kubernetes pod spec. It creates container validators for
// every container and init container in the pod.
func NewPodValidator(pod *corev1.Pod, mode InjectionMode) *PodValidator {
	v := &PodValidator{
		raw:                 pod,
		allContainerNames:   parseAllContainerNames(pod),
		initContainerImages: parseInitContainerImages(pod),
		volumes:             newVolumeMap(pod.Spec.Volumes),
	}

	// Set injection validator based on mode
	switch mode {
	case InjectionModeCSI:
		v.injection = newCSIInjectionValidator(v, pod)
	case InjectionModeImageVolume:
		v.injection = newImageVolumeInjectionValidator(v, pod)
	// Auto mode currently uses init containers as the default injection method
	case InjectionModeAuto, InjectionModeInitContainer:
		fallthrough
	default:
		v.injection = newInitContainerInjectionValidator(v, pod)
	}

	// Create container validators with injection validator
	v.containers = make(map[string]*ContainerValidator, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		v.containers[container.Name] = NewContainerValidator(&container, v.injection)
	}

	v.initContainers = make(map[string]*ContainerValidator, len(pod.Spec.InitContainers))
	for _, container := range pod.Spec.InitContainers {
		v.initContainers[container.Name] = NewContainerValidator(&container, v.injection)
	}

	return v
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
	v.validateContainersInjected(t, expectedContainers)

	// Delegate mode-specific validation
	v.injection.RequireInjection(t)
}

// RequireNoInjection is a high level function that ensures a pod was not injected for SSI,
// including KPI env vars.
func (v *PodValidator) RequireNoInjection(t *testing.T) {
	// Validate no container was injected, including KPI env vars.
	for _, containerValidator := range v.containers {
		containerValidator.RequireNoInjection(t)
	}

	// Delegate mode-specific validation.
	v.injection.RequireNoInjection(t)
}

// RequireNoInjectionArtifacts ensures a pod has no SSI injection artifacts.
func (v *PodValidator) RequireNoInjectionArtifacts(t *testing.T) {
	// Validate no container has SSI injection artifacts.
	for _, containerValidator := range v.containers {
		containerValidator.RequireNoInjectionArtifacts(t)
	}

	// Delegate mode-specific validation.
	v.injection.RequireNoInjection(t)
}

// RequireAnnotations ensures the pod has the expected annotations keys and that the values match the expected value.
func (v *PodValidator) RequireAnnotations(t *testing.T, expected map[string]string) {
	for key, expectedValue := range expected {
		actualValue, exists := v.raw.Annotations[key]
		require.True(t, exists, "annotation does not exist", key)
		require.Equal(t, expectedValue, actualValue, "annotation does not match expected value")
	}
}

// EffectiveAutoMode returns the value the webhook records in the
// effective-injection-mode annotation when "auto" mode resolves to the given
// concrete mode (e.g. EffectiveAutoMode(InjectionModeCSI) == "csi (auto)").
func EffectiveAutoMode(resolved InjectionMode) string {
	return string(resolved) + " (auto)"
}

// RequireMissingAnnotations ensures the pod does not carry any of the given annotation keys.
func (v *PodValidator) RequireMissingAnnotations(t *testing.T, keys []string) {
	for _, key := range keys {
		_, exists := v.raw.Annotations[key]
		require.False(t, exists, "annotation %s should not exist", key)
	}
}

// RequireEffectiveInjectionMode ensures the webhook recorded the given resolved injection mode on the pod.
// The expected value is the raw annotation value: the bare mode for explicit modes (e.g. "csi") and the
// resolved mode suffixed with " (auto)" when "auto" mode was used (use EffectiveAutoMode to build it).
func (v *PodValidator) RequireEffectiveInjectionMode(t *testing.T, expected string) {
	v.RequireAnnotations(t, map[string]string{EffectiveInjectionModeAnnotation: expected})
}

// RequireInjectionStatus ensures the webhook recorded the given overall injection status on the pod.
// Use the InjectionStatus* constants for the expected value.
func (v *PodValidator) RequireInjectionStatus(t *testing.T, expected string) {
	v.RequireAnnotations(t, map[string]string{InjectionStatusAnnotation: expected})
}

// RequireCSIDriverStatus ensures the webhook recorded the given Datadog CSI driver status on the pod.
// This annotation is only present when CSI driver detection is enabled on the cluster-agent.
// Use the CSIDriverStatus* constants for the expected value.
func (v *PodValidator) RequireCSIDriverStatus(t *testing.T, expected string) {
	v.RequireAnnotations(t, map[string]string{CSIDriverStatusAnnotation: expected})
}

// RequireInjectedLibraries ensures the injected-libraries annotation lists exactly the expected
// component-name -> status pairs (e.g. {"injector":"injected","python":"injected"}). Image references
// are intentionally not asserted as they depend on the resolved registry and digest.
func (v *PodValidator) RequireInjectedLibraries(t *testing.T, expected map[string]string) {
	raw, ok := v.raw.Annotations[InjectedLibrariesAnnotation]
	require.True(t, ok, "injected-libraries annotation does not exist")

	var entries []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &entries), "injected-libraries annotation is not valid JSON")

	got := make(map[string]string, len(entries))
	for _, e := range entries {
		got[e.Name] = e.Status
	}
	require.Equal(t, expected, got, "injected-libraries components/statuses do not match")
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

// RequireUnmutatedContainers ensures that the specified containers have no Datadog-related environment variables.
func (v *PodValidator) RequireUnmutatedContainers(t *testing.T, containerNames []string) {
	for _, name := range containerNames {
		validator, found := v.containers[name]
		require.True(t, found, "invalid test setup, expected container %s does not exist in the pod", name)
		validator.RequireUnmutated(t)
	}
}

// RequireInjectorVersion is a high level function to ensure the injector version found for the pod matches expected.
func (v *PodValidator) RequireInjectorVersion(t *testing.T, expected string) {
	v.injection.RequireInjectorVersion(t, expected)
}

// RequireLibraryVersions ensures the map of library name to version matches what is found in the pod. Ex. python -> v3.
func (v *PodValidator) RequireLibraryVersions(t *testing.T, expected map[string]string) {
	v.injection.RequireLibraryVersions(t, expected)
}

// RequireInitContainerImages ensures the list of init container image strings matches the expected list.
func (v *PodValidator) RequireInitContainerImages(t *testing.T, expected []string) {
	require.ElementsMatch(t, expected, v.initContainerImages, "init container images do not match expected")
}

// validateContainersInjected validates the expected containers are injected.
func (v *PodValidator) validateContainersInjected(t *testing.T, expectedContainers []string) {
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
