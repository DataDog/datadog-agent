// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestProviderIsSupported(t *testing.T) {

	tests := []struct {
		name              string
		provider          string
		expectIsSupported bool
	}{
		{
			name:              "supported provider",
			provider:          "fargate",
			expectIsSupported: true,
		},
		{
			name:              "unsupported provider",
			provider:          "foo-provider",
			expectIsSupported: false,
		},
		{
			name:              "empty provider",
			provider:          "",
			expectIsSupported: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			isSupported := providerIsSupported(test.provider)
			if test.expectIsSupported {
				assert.True(tt, isSupported)
			} else {
				assert.False(tt, isSupported)
			}
		})
	}
}

func TestApplyProviderOverrides(t *testing.T) {
	mockConfig := config.Mock(t)
	hostPathType := corev1.HostPathDirectoryOrCreate

	tests := []struct {
		name                     string
		provider                 string
		basePod                  *corev1.Pod
		expectedPodAfterOverride *corev1.Pod
		expectError              bool
		expectMutated            bool
	}{
		{
			name:                     "nil pod should be skipped",
			provider:                 "fargate",
			basePod:                  nil,
			expectedPodAfterOverride: nil,
			expectError:              true,
			expectMutated:            false,
		},
		{
			name:                     "empty provider",
			provider:                 "",
			basePod:                  &corev1.Pod{},
			expectedPodAfterOverride: &corev1.Pod{},
			expectError:              false,
			expectMutated:            false,
		},
		{
			name:     "fargate provider",
			provider: "fargate",
			basePod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app-container",
						},
						{
							Name: agentSidecarContainerName,
						},
					},
				},
			},
			expectedPodAfterOverride: &corev1.Pod{
				Spec: corev1.PodSpec{
					ShareProcessNamespace: pointer.Ptr(true),
					Containers: []corev1.Container{
						{
							Name: "app-container",
							Env: []corev1.EnvVar{
								{
									Name:  "DD_TRACE_AGENT_URL",
									Value: "unix:///var/run/datadog/apm.socket",
								},
								{
									Name:  "DD_DOGSTATSD_URL",
									Value: "unix:///var/run/datadog/dsd.socket",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "ddsockets",
									MountPath: "/var/run/datadog",
									ReadOnly:  false,
								},
							},
						},
						{
							Name: agentSidecarContainerName,
							Env: []corev1.EnvVar{
								{
									Name:  "DD_EKS_FARGATE",
									Value: "true",
								},
								{
									Name:  "DD_APM_RECEIVER_SOCKET",
									Value: "/var/run/datadog/apm.socket",
								},
								{
									Name:  "DD_DOGSTATSD_SOCKET",
									Value: "/var/run/datadog/dsd.socket",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "ddsockets",
									MountPath: "/var/run/datadog",
									ReadOnly:  false,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "ddsockets",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
			expectError:   false,
			expectMutated: true,
		},
		{
			// This test checks that the volume and volume mounts set by the
			// config webhook are replaced by ones that works on Fargate.
			name:     "fargate provider - with volume set by the config webhook",
			provider: "fargate",
			basePod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app-container",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "datadog",
									MountPath: "/var/run/datadog",
									ReadOnly:  false,
								},
							},
						},
						{
							Name: agentSidecarContainerName,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "datadog",
									MountPath: "/var/run/datadog",
									ReadOnly:  false,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "datadog",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Type: &hostPathType,
									Path: "/var/run/datadog",
								},
							},
						},
					},
				},
			},
			expectedPodAfterOverride: &corev1.Pod{
				Spec: corev1.PodSpec{
					ShareProcessNamespace: pointer.Ptr(true),
					Containers: []corev1.Container{
						{
							Name: "app-container",
							Env: []corev1.EnvVar{
								{
									Name:  "DD_TRACE_AGENT_URL",
									Value: "unix:///var/run/datadog/apm.socket",
								},
								{
									Name:  "DD_DOGSTATSD_URL",
									Value: "unix:///var/run/datadog/dsd.socket",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "ddsockets",
									MountPath: "/var/run/datadog",
									ReadOnly:  false,
								},
							},
						},
						{
							Name: agentSidecarContainerName,
							Env: []corev1.EnvVar{
								{
									Name:  "DD_EKS_FARGATE",
									Value: "true",
								},
								{
									Name:  "DD_APM_RECEIVER_SOCKET",
									Value: "/var/run/datadog/apm.socket",
								},
								{
									Name:  "DD_DOGSTATSD_SOCKET",
									Value: "/var/run/datadog/dsd.socket",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "ddsockets",
									MountPath: "/var/run/datadog",
									ReadOnly:  false,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "ddsockets",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
			expectError:   false,
			expectMutated: true,
		},
		{
			name:                     "unsupported provider",
			provider:                 "foo-provider",
			basePod:                  &corev1.Pod{},
			expectedPodAfterOverride: &corev1.Pod{},
			expectError:              true,
			expectMutated:            false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			mockConfig.SetWithoutSource("admission_controller.agent_sidecar.provider", test.provider)
			mutated, err := applyProviderOverrides(test.basePod)

			if test.expectError {
				assert.Error(tt, err)
				return
			}

			require.NoError(tt, err)
			assert.Equal(tt, test.expectMutated, mutated)
			assert.True(
				tt,
				cmp.Equal(test.expectedPodAfterOverride, test.basePod),
				"overrides not applied as expected. diff: %s",
				cmp.Diff(test.expectedPodAfterOverride, test.basePod),
			)
		})
	}
}
