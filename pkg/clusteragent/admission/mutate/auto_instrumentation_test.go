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
		name           string
		pod            *corev1.Pod
		lang           language
		image          string
		expectedEnvKey string
		expectedEnvVal string
		wantErr        bool
	}{
		{
			name:           "nominal case: java",
			pod:            fakePod("java-pod"),
			lang:           "java",
			image:          "gcr.io/datadoghq/dd-lib-java-init:v1",
			expectedEnvKey: "JAVA_TOOL_OPTIONS",
			expectedEnvVal: " -javaagent:/datadog-lib/dd-java-agent.jar",
			wantErr:        false,
		},
		{
			name:           "JAVA_TOOL_OPTIONS not empty",
			pod:            fakePodWithEnvValue("java-pod", "JAVA_TOOL_OPTIONS", "predefined"),
			lang:           "java",
			image:          "gcr.io/datadoghq/dd-lib-java-init:v1",
			expectedEnvKey: "JAVA_TOOL_OPTIONS",
			expectedEnvVal: "predefined -javaagent:/datadog-lib/dd-java-agent.jar",
			wantErr:        false,
		},
		{
			name:           "nominal case: js",
			pod:            fakePod("js-pod"),
			lang:           "js",
			image:          "gcr.io/datadoghq/dd-lib-js-init:v1",
			expectedEnvKey: "NODE_OPTIONS",
			expectedEnvVal: " --require=/datadog-lib/node_modules/dd-trace/init",
			wantErr:        false,
		},
		{
			name:           "NODE_OPTIONS not empty",
			pod:            fakePodWithEnvValue("js-pod", "NODE_OPTIONS", "predefined"),
			lang:           "js",
			image:          "gcr.io/datadoghq/dd-lib-js-init:v1",
			expectedEnvKey: "NODE_OPTIONS",
			expectedEnvVal: "predefined --require=/datadog-lib/node_modules/dd-trace/init",
			wantErr:        false,
		},
		{
			name:           "nominal case: python",
			pod:            fakePod("python-pod"),
			lang:           "python",
			image:          "gcr.io/datadoghq/dd-lib-python-init:v1",
			expectedEnvKey: "PYTHONPATH",
			expectedEnvVal: "/datadog-lib/",
			wantErr:        false,
		},
		{
			name:           "PYTHONPATH not empty",
			pod:            fakePodWithEnvValue("python-pod", "PYTHONPATH", "predefined"),
			lang:           "python",
			image:          "gcr.io/datadoghq/dd-lib-python-init:v1",
			expectedEnvKey: "PYTHONPATH",
			expectedEnvVal: "/datadog-lib/:predefined",
			wantErr:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := injectAutoInstruConfig(tt.pod, tt.lang, tt.image)
			require.False(t, (err != nil) != tt.wantErr)
			assertLibConfig(t, tt.pod, tt.image, tt.expectedEnvKey, tt.expectedEnvVal)
		})
	}
}

func assertLibConfig(t *testing.T, pod *corev1.Pod, image, envKey, envVal string) {
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
		if container.Name == "datadog-lib-init" {
			require.Equal(t, image, container.Image)
			require.Equal(t, []string{"sh", "copy-lib.sh", "/datadog-lib"}, container.Command)
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
			require.Equal(t, envVal, env.Value)
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
		expectedLangauge     language
		expectedImage        string
		expectedShouldInject bool
	}{
		{
			name:                 "java",
			pod:                  fakePodWithAnnotation("admission.datadoghq.com/java-lib.version", "v1"),
			containerRegistry:    "registry",
			expectedLangauge:     "java",
			expectedImage:        "registry/dd-lib-java-init:v1",
			expectedShouldInject: true,
		},
		{
			name:                 "js",
			pod:                  fakePodWithAnnotation("admission.datadoghq.com/js-lib.version", "v1"),
			containerRegistry:    "registry",
			expectedLangauge:     "js",
			expectedImage:        "registry/dd-lib-js-init:v1",
			expectedShouldInject: true,
		},
		{
			name:                 "python",
			pod:                  fakePodWithAnnotation("admission.datadoghq.com/python-lib.version", "v1"),
			containerRegistry:    "registry",
			expectedLangauge:     "python",
			expectedImage:        "registry/dd-lib-python-init:v1",
			expectedShouldInject: true,
		},
		{
			name:                 "custom",
			pod:                  fakePodWithAnnotation("admission.datadoghq.com/java-lib.custom-image", "custom/image"),
			containerRegistry:    "registry",
			expectedLangauge:     "java",
			expectedImage:        "custom/image",
			expectedShouldInject: true,
		},
		{
			name:                 "unknown",
			pod:                  fakePodWithAnnotation("admission.datadoghq.com/unknown-lib.version", "v1"),
			containerRegistry:    "registry",
			expectedLangauge:     "",
			expectedImage:        "",
			expectedShouldInject: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang, image, shouldInject := extractLibInfo(tt.pod, tt.containerRegistry)
			require.Equal(t, tt.expectedLangauge, lang)
			require.Equal(t, tt.expectedImage, image)
			require.Equal(t, tt.expectedShouldInject, shouldInject)
		})
	}
}
