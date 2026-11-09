// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package sidecar

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

func baseUDSConfig() appsecconfig.Sidecar {
	return appsecconfig.Sidecar{
		Image:      "img",
		ImageTag:   "tag",
		HealthPort: 8081,
		UDSPath:    "/var/run/datadog/extproc.sock",
		RunAsUser:  65532,
	}
}

func TestBuildExtProcProcessorContainerUDS_Name(t *testing.T) {
	c := BuildExtProcProcessorContainerUDS(baseUDSConfig())
	assert.Equal(t, SidecarContainerName, c.Name)
}

func TestBuildExtProcProcessorContainerUDS_Image(t *testing.T) {
	c := BuildExtProcProcessorContainerUDS(baseUDSConfig())
	assert.Equal(t, "img:tag", c.Image)
}

func TestBuildExtProcProcessorContainerUDS_EnvUDSPath(t *testing.T) {
	c := BuildExtProcProcessorContainerUDS(baseUDSConfig())
	assert.Equal(t, "/var/run/datadog/extproc.sock", envVal(c.Env, "DD_SERVICE_EXTENSION_UDS_PATH"))
}

func TestBuildExtProcProcessorContainerUDS_EnvTLS(t *testing.T) {
	c := BuildExtProcProcessorContainerUDS(baseUDSConfig())
	assert.Equal(t, "false", envVal(c.Env, "DD_SERVICE_EXTENSION_TLS"))
}

func TestBuildExtProcProcessorContainerUDS_NoTCPPort(t *testing.T) {
	c := BuildExtProcProcessorContainerUDS(baseUDSConfig())
	for _, e := range c.Env {
		assert.NotEqual(t, "DD_SERVICE_EXTENSION_PORT", e.Name, "UDS container must not set DD_SERVICE_EXTENSION_PORT")
	}
}

func TestBuildExtProcProcessorContainerUDS_HealthPort(t *testing.T) {
	c := BuildExtProcProcessorContainerUDS(baseUDSConfig())
	require.Len(t, c.Ports, 1)
	assert.Equal(t, "health", c.Ports[0].Name)
	assert.Equal(t, int32(8081), c.Ports[0].ContainerPort)
	assert.Equal(t, corev1.ProtocolTCP, c.Ports[0].Protocol)
}

func TestBuildExtProcProcessorContainerUDS_VolumeMount(t *testing.T) {
	c := BuildExtProcProcessorContainerUDS(baseUDSConfig())
	require.Len(t, c.VolumeMounts, 1)
	assert.Equal(t, SharedSocketVolumeName, c.VolumeMounts[0].Name)
	assert.Equal(t, "/var/run/datadog", c.VolumeMounts[0].MountPath)
}

func TestBuildExtProcProcessorContainerUDS_SecurityContext(t *testing.T) {
	c := BuildExtProcProcessorContainerUDS(baseUDSConfig())
	require.NotNil(t, c.SecurityContext)
	require.NotNil(t, c.SecurityContext.RunAsUser)
	assert.Equal(t, int64(65532), *c.SecurityContext.RunAsUser)
	require.NotNil(t, c.SecurityContext.RunAsGroup)
	assert.Equal(t, int64(65532), *c.SecurityContext.RunAsGroup)
	require.NotNil(t, c.SecurityContext.AllowPrivilegeEscalation)
	assert.False(t, *c.SecurityContext.AllowPrivilegeEscalation)
	require.NotNil(t, c.SecurityContext.RunAsNonRoot)
	assert.True(t, *c.SecurityContext.RunAsNonRoot)
}

func TestBuildExtProcProcessorContainerUDS_Probes(t *testing.T) {
	c := BuildExtProcProcessorContainerUDS(baseUDSConfig())
	require.NotNil(t, c.ReadinessProbe)
	require.NotNil(t, c.ReadinessProbe.HTTPGet)
	assert.Equal(t, "/", c.ReadinessProbe.HTTPGet.Path)
	require.NotNil(t, c.LivenessProbe)
	require.NotNil(t, c.LivenessProbe.HTTPGet)
	assert.Equal(t, "/", c.LivenessProbe.HTTPGet.Path)
}

func TestBuildExtProcProcessorContainerUDS_BodyParsingSizeLimit(t *testing.T) {
	cfg := baseUDSConfig()
	cfg.BodyParsingSizeLimit = "5000000"
	c := BuildExtProcProcessorContainerUDS(cfg)
	assert.Equal(t, "5000000", envVal(c.Env, "DD_APPSEC_BODY_PARSING_SIZE_LIMIT"))

	cfgNoLimit := baseUDSConfig()
	c2 := BuildExtProcProcessorContainerUDS(cfgNoLimit)
	for _, e := range c2.Env {
		assert.NotEqual(t, "DD_APPSEC_BODY_PARSING_SIZE_LIMIT", e.Name)
	}
}

func TestEnsureSharedSocketVolume_Idempotent(t *testing.T) {
	pod := &corev1.Pod{}
	name1 := EnsureSharedSocketVolume(pod)
	name2 := EnsureSharedSocketVolume(pod)
	assert.Equal(t, SharedSocketVolumeName, name1)
	assert.Equal(t, SharedSocketVolumeName, name2)
	count := 0
	for _, v := range pod.Spec.Volumes {
		if v.Name == SharedSocketVolumeName {
			count++
		}
	}
	assert.Equal(t, 1, count, "calling EnsureSharedSocketVolume twice must not duplicate the volume")
}

func TestEnsureSharedSocketVolume_EmptyDir(t *testing.T) {
	pod := &corev1.Pod{}
	EnsureSharedSocketVolume(pod)
	require.Len(t, pod.Spec.Volumes, 1)
	assert.NotNil(t, pod.Spec.Volumes[0].EmptyDir)
}

func TestMountSocketIntoContainer_Success(t *testing.T) {
	pod := podWithContainer("envoy")
	err := MountSocketIntoContainer(pod, "envoy", SharedSocketVolumeName, "/var/run/datadog")
	require.NoError(t, err)
	mounts := pod.Spec.Containers[0].VolumeMounts
	require.Len(t, mounts, 1)
	assert.Equal(t, SharedSocketVolumeName, mounts[0].Name)
	assert.Equal(t, "/var/run/datadog", mounts[0].MountPath)
}

func TestMountSocketIntoContainer_Idempotent(t *testing.T) {
	pod := podWithContainer("envoy")
	require.NoError(t, MountSocketIntoContainer(pod, "envoy", SharedSocketVolumeName, "/var/run/datadog"))
	require.NoError(t, MountSocketIntoContainer(pod, "envoy", SharedSocketVolumeName, "/var/run/datadog"))
	assert.Len(t, pod.Spec.Containers[0].VolumeMounts, 1, "calling MountSocketIntoContainer twice must not duplicate the mount")
}

func TestMountSocketIntoContainer_ContainerNotFound(t *testing.T) {
	pod := podWithContainer("envoy")
	err := MountSocketIntoContainer(pod, "nonexistent", SharedSocketVolumeName, "/var/run/datadog")
	require.Error(t, err)
}

func TestMountSocketIntoContainer_MountPathConflict(t *testing.T) {
	pod := podWithContainer("envoy")
	pod.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{Name: "other-vol", MountPath: "/var/run/datadog"}}
	err := MountSocketIntoContainer(pod, "envoy", SharedSocketVolumeName, "/var/run/datadog")
	require.Error(t, err, "a different volume already at the mount path must be a conflict, not a duplicate append")
	require.Len(t, pod.Spec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, "other-vol", pod.Spec.Containers[0].VolumeMounts[0].Name)
}

func TestEnsureSocketFSGroup_NilSecurityContext(t *testing.T) {
	pod := &corev1.Pod{}
	EnsureSocketFSGroup(pod, 65532)
	require.NotNil(t, pod.Spec.SecurityContext)
	require.NotNil(t, pod.Spec.SecurityContext.FSGroup)
	assert.Equal(t, int64(65532), *pod.Spec.SecurityContext.FSGroup)
	require.NotNil(t, pod.Spec.SecurityContext.FSGroupChangePolicy)
	assert.Equal(t, corev1.FSGroupChangeOnRootMismatch, *pod.Spec.SecurityContext.FSGroupChangePolicy)
}

func TestEnsureSocketFSGroup_Idempotent(t *testing.T) {
	pod := &corev1.Pod{}
	EnsureSocketFSGroup(pod, 65532)
	EnsureSocketFSGroup(pod, 65532)
	assert.Equal(t, int64(65532), *pod.Spec.SecurityContext.FSGroup)
}

func TestEnsureSocketFSGroup_DoesNotClobber(t *testing.T) {
	existing := int64(1000)
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{
				FSGroup: &existing,
			},
		},
	}
	EnsureSocketFSGroup(pod, 65532)
	assert.Equal(t, int64(1000), *pod.Spec.SecurityContext.FSGroup, "EnsureSocketFSGroup must not clobber a pre-existing FSGroup")
}

func envVal(env []corev1.EnvVar, name string) string {
	for _, e := range env {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

func podWithContainer(name string) *corev1.Pod {
	return &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: name},
			},
		},
	}
}
