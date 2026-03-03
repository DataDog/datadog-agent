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

// ContainerInjectionValidator validates container-level injection.
type ContainerInjectionValidator interface {
	RequireContainerInjection(t *testing.T, container *ContainerValidator)
	RequireNoContainerInjection(t *testing.T, container *ContainerValidator)
}

// ContainerValidator provides a test friendly structure to run assertions on container states for SSI.
type ContainerValidator struct {
	raw          *corev1.Container
	image        *ImageValidator
	envs         map[string]string
	commandline  string
	volumeMounts map[string]corev1.VolumeMount
	injection    ContainerInjectionValidator
}

// NewContainerValidator initializes a container validator and converts the Kubernetes spec into a test friendly struct.
func NewContainerValidator(container *corev1.Container, injection ContainerInjectionValidator) *ContainerValidator {
	return &ContainerValidator{
		raw:          container,
		envs:         newEnvMap(container.Env),
		image:        NewImageValidator(container.Image),
		commandline:  parseCommandline(container),
		volumeMounts: newVolumeMountMap(container.VolumeMounts),
		injection:    injection,
	}
}

// RequireInjection ensures a container was injected for SSI. It's a high level function that should be modified if
// the implementation or meaning of injection changes over time.
func (v *ContainerValidator) RequireInjection(t *testing.T) {
	// Validate common env vars
	expectedEnvs := map[string]string{
		"LD_PRELOAD":            "/opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so",
		"DD_INJECT_SENDER_TYPE": "k8s",
	}
	v.RequireEnvs(t, expectedEnvs)

	// Validate mode-specific volume mounts
	if v.injection != nil {
		v.injection.RequireContainerInjection(t, v)
	}
}

// RequireNoInjection ensures a container was not injected for SSI.
func (v *ContainerValidator) RequireNoInjection(t *testing.T) {
	// Validate common env vars are not set
	unsetEnvs := []string{
		"LD_PRELOAD",
		"DD_INJECT_SENDER_TYPE",
		"DD_INSTRUMENTATION_INSTALL_TYPE",
	}
	v.RequireMissingEnvs(t, unsetEnvs)

	// Validate mode-specific volume mounts are not present
	if v.injection != nil {
		v.injection.RequireNoContainerInjection(t, v)
	}
}

// RequireVolumeMounts ensures the list of provided volume mounts exist in the container.
func (v *ContainerValidator) RequireVolumeMounts(t *testing.T, expected []corev1.VolumeMount) {
	for _, mount := range expected {
		actual, exists := v.volumeMounts[mount.MountPath]
		require.True(t, exists, "volume mount with name %s does not exist", mount.Name)
		require.Equal(t, mount, actual)
	}
}

// RequireMissingVolumeMounts ensures the list of provided volume mounts do not exist in the container.
func (v *ContainerValidator) RequireMissingVolumeMounts(t *testing.T, missing []corev1.VolumeMount) {
	for _, mount := range missing {
		_, exists := v.volumeMounts[mount.MountPath]
		require.False(t, exists, "volume mount with name %s should not exist", mount.Name)
	}
}

// RequireCommand ensures the provided command line is the one present in the container. The validator converts the
// command with args list into a string for your convenience.
func (v *ContainerValidator) RequireCommand(t *testing.T, expected string) {
	require.Equal(t, expected, v.commandline, "command line does not match expected")
}

// RequireSecurityContext ensures the provided security context matches the security context on the container.
func (v *ContainerValidator) RequireSecurityContext(t *testing.T, expected *corev1.SecurityContext) {
	require.Equal(t, expected, v.raw.SecurityContext, "security context not match expected")
}

// RequireResourceRequirements ensures the resource requirements for the container match the expected.
func (v *ContainerValidator) RequireResourceRequirements(t *testing.T, expected *corev1.ResourceRequirements) {
	if expected == nil {
		expected = &corev1.ResourceRequirements{
			Limits:   corev1.ResourceList{},
			Requests: corev1.ResourceList{},
		}
	}

	require.Zero(t, expected.Requests.Memory().Cmp(*v.raw.Resources.Requests.Memory()), "expected memory request: %s, actual: %s", expected.Requests.Memory().String(), v.raw.Resources.Requests.Memory().String())
	require.Zero(t, expected.Limits.Memory().Cmp(*v.raw.Resources.Limits.Memory()), "expected memory limit: %s, actual: %s", expected.Limits.Memory().String(), v.raw.Resources.Limits.Memory().String())
	require.Zero(t, expected.Requests.Cpu().Cmp(*v.raw.Resources.Requests.Cpu()), "expected cpu request: %s, actual: %s", expected.Requests.Cpu().String(), v.raw.Resources.Requests.Cpu().String())
	require.Zero(t, expected.Limits.Cpu().Cmp(*v.raw.Resources.Limits.Cpu()), "expected cpu limit: %s, actual: %s", expected.Limits.Cpu().String(), v.raw.Resources.Limits.Cpu().String())
}

// RequireNoVolumeMounts ensures the container has no volume mounts.
func (v *ContainerValidator) RequireNoVolumeMounts(t *testing.T) {
	require.Empty(t, v.raw.VolumeMounts, "container should not have additional volume mounts")
}

// RequireEnvs ensures the map of key/value pairs both exist and are set to the expected value.
func (v *ContainerValidator) RequireEnvs(t *testing.T, expected map[string]string) {
	for key, expectedValue := range expected {
		actualValue, found := v.envs[key]
		require.True(t, found, "could not find expected env %s in environment", key)
		require.Equal(t, expectedValue, actualValue, "environment values do not match for container")
	}
}

// RequireMissingEnvs ensures the list of keys are missing in the container.
func (v *ContainerValidator) RequireMissingEnvs(t *testing.T, missing []string) {
	for _, key := range missing {
		actualValue, found := v.envs[key]
		require.False(t, found, "found %s in environment set to %s when it was not expected to be set", key, actualValue)
	}
}

// RequireUnmutated ensures no Datadog-related environment variables exist in the container.
// This includes any env var starting with "DD_" or "LD_PRELOAD".
func (v *ContainerValidator) RequireUnmutated(t *testing.T) {
	// These env vars are injected by other webhooks (not SSI), so they're allowed.
	// See pkg/clusteragent/admission/mutate/config/config.go
	allowedEnvVars := map[string]bool{
		"DD_AGENT_HOST":       true,
		"DD_ENTITY_ID":        true,
		"DD_INTERNAL_POD_UID": true,
		"DD_EXTERNAL_ENV":     true,
	}

	for key, value := range v.envs {
		if allowedEnvVars[key] {
			continue
		}
		if strings.HasPrefix(key, "DD_") || key == "LD_PRELOAD" {
			require.Fail(t, "found unexpected Datadog env var", "env var %s=%s should not be present in container", key, value)
		}
	}
}

func parseCommandline(container *corev1.Container) string {
	command := strings.Join(container.Command, " ")
	args := strings.Join(container.Args, " ")
	return strings.Join([]string{command, args}, " ")
}

func newVolumeMountMap(in []corev1.VolumeMount) map[string]corev1.VolumeMount {
	volumeMounts := make(map[string]corev1.VolumeMount, len(in))
	for _, mount := range in {
		// The mount path is more likely to be unique then the name.
		volumeMounts[mount.MountPath] = mount
	}
	return volumeMounts
}
