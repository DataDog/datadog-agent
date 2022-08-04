// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package mutate

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestInjectAutoInstruConfig(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		language string
		image    string
		wantErr  bool
	}{
		{
			name:     "nominal case: java",
			pod:      fakePod("java-pod"),
			language: "java",
			image:    "gcr.io/datadoghq/dd-java-agent-init:v1",
			wantErr:  false,
		},
		{
			name:     "nominal case: node",
			pod:      fakePod("node-pod"),
			language: "node",
			image:    "gcr.io/datadoghq/dd-node-agent-init:v1",
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := injectAutoInstruConfig(tt.pod, tt.language, tt.image)
			require.False(t, (err != nil) != tt.wantErr)
			switch tt.language {
			case "java":
				assertLibConfig(t, tt.pod, tt.image, "JAVA_TOOL_OPTIONS", " -javaagent:/datadog-lib/dd-java-agent.jar", []string{"sh", "copy-lib.sh", "/datadog-lib"})
			case "node":
				assertLibConfig(t, tt.pod, tt.image, "NODE_OPTIONS", " --require=/autoinstrumentation/node_modules/dd-trace/init", []string{"sh", "copy-lib.sh", "/datadog-lib"})
			default:
				t.Fatalf("Unknown language %q", tt.language)
			}
		})
	}
}

func assertLibConfig(t *testing.T, pod *corev1.Pod, image, envKey, envVal string, cmd []string) {
	// Empty dir volume
	volumeFound := false
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == "datadog-auto-instrumentation" {
			require.NotNil(t, volume.VolumeSource.EmptyDir)
			volumeFound = true
			break
		}
	}
	require.True(t, volumeFound)

	// Init container
	initContainerFound := false
	for _, container := range pod.Spec.InitContainers {
		if container.Name == "datadog-tracer-init" {
			require.Equal(t, image, container.Image)
			require.Equal(t, cmd, container.Command)
			require.Equal(t, "datadog-auto-instrumentation", container.VolumeMounts[0].Name)
			require.Equal(t, "/datadog-lib", container.VolumeMounts[0].MountPath)
			initContainerFound = true
			break
		}
	}
	require.True(t, initContainerFound)

	// App container
	container := pod.Spec.Containers[0]
	require.Equal(t, "datadog-auto-instrumentation", container.VolumeMounts[0].Name)
	require.Equal(t, "/datadog-lib", container.VolumeMounts[0].MountPath)
	envFound := false
	for _, env := range container.Env {
		if env.Name == envKey {
			require.Contains(t, envVal, env.Value)
			envFound = true
			break
		}
	}
	require.True(t, envFound)
}

func TestExtractLibInfo(t *testing.T) {
	tests := []struct {
		name                 string
		pod                  *corev1.Pod
		containerRegistry    string
		expectedLangauge     string
		expectedImage        string
		expectedShouldInject bool
	}{
		{
			name:                 "java",
			pod:                  fakePodWithAnnotation("admission.datadoghq.com/java-tracer.version", "v1"),
			containerRegistry:    "registry",
			expectedLangauge:     "java",
			expectedImage:        "registry/dd-java-agent-init:v1",
			expectedShouldInject: true,
		},
		{
			name:                 "node",
			pod:                  fakePodWithAnnotation("admission.datadoghq.com/node-tracer.version", "v1"),
			containerRegistry:    "registry",
			expectedLangauge:     "node",
			expectedImage:        "registry/dd-node-agent-init:v1",
			expectedShouldInject: true,
		},
		{
			name:                 "custom",
			pod:                  fakePodWithAnnotation("admission.datadoghq.com/java-tracer.custom-image", "custom/image"),
			containerRegistry:    "registry",
			expectedLangauge:     "java",
			expectedImage:        "custom/image",
			expectedShouldInject: true,
		},
		{
			name:                 "unknown",
			pod:                  fakePodWithAnnotation("admission.datadoghq.com/unknown-tracer.version", "v1"),
			containerRegistry:    "registry",
			expectedLangauge:     "",
			expectedImage:        "",
			expectedShouldInject: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			language, image, shouldInject := extractLibInfo(tt.pod, tt.containerRegistry)
			require.Equal(t, tt.expectedLangauge, language)
			require.Equal(t, tt.expectedImage, image)
			require.Equal(t, tt.expectedShouldInject, shouldInject)
		})
	}
}
