// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/config"
	apicommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const commonRegistry = "gcr.io/datadoghq"

func TestInjectAgentSidecar(t *testing.T) {
	tests := []struct {
		Name                      string
		Pod                       *corev1.Pod
		provider                  string
		profilesJSON              string
		ExpectError               bool
		ExpectInjection           bool
		ExpectedPodAfterInjection func() *corev1.Pod
	}{
		{
			Name:                      "should return error for nil pod",
			Pod:                       nil,
			provider:                  "",
			profilesJSON:              "",
			ExpectError:               true,
			ExpectInjection:           false,
			ExpectedPodAfterInjection: func() *corev1.Pod { return nil },
		},
		{
			Name: "should inject sidecar if no sidecar present, no provider set",
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-name",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "container-name"},
					},
				},
			},
			provider:        "",
			profilesJSON:    "[]",
			ExpectError:     false,
			ExpectInjection: true,
			ExpectedPodAfterInjection: func() *corev1.Pod {
				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-name",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "container-name"},
							*getDefaultSidecarTemplate(commonRegistry),
						},
					},
				}
			},
		},
		{
			Name: "idempotency test: should not inject sidecar if sidecar already exists",
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-name",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "container-name"},
						{Name: agentSidecarContainerName},
					},
				},
			},
			provider:        "",
			profilesJSON:    "[]",
			ExpectError:     false,
			ExpectInjection: false,
			ExpectedPodAfterInjection: func() *corev1.Pod {
				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-name",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "container-name"},
							{Name: agentSidecarContainerName},
						},
					},
				}
			},
		},
		{
			Name: "should skip injecting sidecar when sidecar already exists",
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-name",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "container-name"},
						*getDefaultSidecarTemplate(commonRegistry),
					},
				},
			},
			provider:        "",
			profilesJSON:    "[]",
			ExpectError:     false,
			ExpectInjection: false,
			ExpectedPodAfterInjection: func() *corev1.Pod {
				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-name",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "container-name"},
							*getDefaultSidecarTemplate(commonRegistry),
						},
					},
				}
			},
		},
		{
			Name: "should inject sidecar if no sidecar present, with supported provider",
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-name",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "container-name"},
					},
				},
			},
			provider:        "fargate",
			profilesJSON:    "[]",
			ExpectError:     false,
			ExpectInjection: true,
			ExpectedPodAfterInjection: func() *corev1.Pod {
				sidecar := *getDefaultSidecarTemplate(commonRegistry)
				_, _ = withEnvOverrides(
					&sidecar,
					corev1.EnvVar{
						Name:  "DD_EKS_FARGATE",
						Value: "true",
					},
					corev1.EnvVar{
						Name:  "DD_APM_RECEIVER_SOCKET",
						Value: "/var/run/datadog/apm.socket",
					},
					corev1.EnvVar{
						Name:  "DD_DOGSTATSD_SOCKET",
						Value: "/var/run/datadog/dsd.socket",
					},
				)

				sidecar.VolumeMounts = []corev1.VolumeMount{
					{
						Name:      "ddsockets",
						MountPath: "/var/run/datadog",
						ReadOnly:  false,
					},
				}

				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-name",
					},
					Spec: corev1.PodSpec{
						ShareProcessNamespace: pointer.Ptr(true),
						Containers: []corev1.Container{
							{
								Name: "container-name",
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
							sidecar,
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
				}
			},
		},
		{
			Name: "should inject sidecar if no sidecar present, with supported provider, and profile overrides should apply",
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-name",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "container-name"},
					},
				},
			},
			provider: "fargate",
			profilesJSON: `[{
        "env": [
            {"name": "ENV_VAR_1", "valueFrom": {"secretKeyRef": {"name": "my-secret", "key": "secret-key"}}},
            {"name": "ENV_VAR_2", "value": "value2"}
        ],
        "resources": {
            "limits": {
                "cpu": "1",
                "memory": "512Mi"
            },
            "requests": {
                "cpu": "0.5",
                "memory": "256Mi"
            }
        }
    }]`,
			ExpectError:     false,
			ExpectInjection: true,
			ExpectedPodAfterInjection: func() *corev1.Pod {
				sidecar := *getDefaultSidecarTemplate(commonRegistry)

				_, _ = withEnvOverrides(
					&sidecar,
					corev1.EnvVar{
						Name:  "DD_EKS_FARGATE",
						Value: "true",
					},
					corev1.EnvVar{
						Name:  "DD_APM_RECEIVER_SOCKET",
						Value: "/var/run/datadog/apm.socket",
					},
					corev1.EnvVar{
						Name:  "DD_DOGSTATSD_SOCKET",
						Value: "/var/run/datadog/dsd.socket",
					},
					corev1.EnvVar{
						Name: "ENV_VAR_1",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								Key:                  "secret-key",
								LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
							},
						},
					},
					corev1.EnvVar{
						Name:  "ENV_VAR_2",
						Value: "value2",
					},
				)

				_ = withResourceLimits(&sidecar, corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("1"), "memory": resource.MustParse("512Mi")},
					Requests: corev1.ResourceList{"cpu": resource.MustParse("0.5"), "memory": resource.MustParse("256Mi")},
				})

				sidecar.VolumeMounts = []corev1.VolumeMount{
					{
						Name:      "ddsockets",
						MountPath: "/var/run/datadog",
						ReadOnly:  false,
					},
				}

				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-name",
					},
					Spec: corev1.PodSpec{
						ShareProcessNamespace: pointer.Ptr(true),
						Containers: []corev1.Container{
							{
								Name: "container-name",
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
							sidecar,
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
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(tt *testing.T) {
			mockConfig := config.Mock(t)
			mockConfig.SetWithoutSource("admission_controller.agent_sidecar.provider", test.provider)
			mockConfig.SetWithoutSource("admission_controller.agent_sidecar.profiles", test.profilesJSON)

			webhook := NewWebhook()

			injected, err := webhook.injectAgentSidecar(test.Pod, "", nil)

			if test.ExpectError {
				assert.Error(tt, err, "expected non-nil error to be returned")
			} else {
				assert.NoError(tt, err, "expected returned error to be nil")
			}

			if test.ExpectInjection {
				assert.True(t, injected)
			} else {
				assert.False(t, injected)
			}

			expectedPod := test.ExpectedPodAfterInjection()
			if expectedPod == nil {

				assert.Nil(tt, test.Pod)
			} else {
				assert.NotNil(tt, test.Pod)
				assert.Truef(
					tt,
					reflect.DeepEqual(*expectedPod, *test.Pod),
					"expected %v, found %v",
					*expectedPod,
					*test.Pod,
				)
			}

		})
	}

}

func TestDefaultSidecarTemplateAgentImage(t *testing.T) {
	tests := []struct {
		name              string
		setConfig         func()
		containerRegistry string
		expectedImage     string
	}{
		{
			name:              "no configuration set",
			setConfig:         func() {},
			containerRegistry: commonRegistry,
			expectedImage:     fmt.Sprintf("%s/agent:latest", commonRegistry),
		},
		{
			name:              "setting custom registry, image and tag",
			containerRegistry: "my-registry",
			setConfig: func() {
				mockConfig := config.Mock(t)
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.container_registry", "my-registry")
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.image_name", "my-image")
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.image_tag", "my-tag")
			},
			expectedImage: "my-registry/my-image:my-tag",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			test.setConfig()
			sidecar := getDefaultSidecarTemplate(test.containerRegistry)
			assert.Equal(tt, test.expectedImage, sidecar.Image)
		})
	}
}

func TestDefaultSidecarTemplateClusterAgentEnvVars(t *testing.T) {

	tests := []struct {
		name              string
		setConfig         func()
		expectedEnvVars   []corev1.EnvVar
		unexpectedEnvVars []string
	}{
		{
			name: "cluster agent not enabled",
			setConfig: func() {
				mockConfig := config.Mock(t)
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.cluster_agent.enabled", false)
			},
			unexpectedEnvVars: []string{
				"DD_CLUSTER_AGENT_ENABLED",
				"DD_CLUSTER_AGENT_AUTH_TOKEN",
				"DD_CLUSTER_AGENT_URL",
				"DD_ORCHESTRATOR_EXPLORER_ENABLED",
			},
		},
		{
			name: "cluster agent enabled with default values",
			setConfig: func() {
				mockConfig := config.Mock(t)
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.cluster_agent.enabled", true)
			},
			expectedEnvVars: []corev1.EnvVar{
				{
					Name:  "DD_CLUSTER_AGENT_ENABLED",
					Value: "true",
				},
				{
					Name: "DD_CLUSTER_AGENT_AUTH_TOKEN",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							Key: "token",
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "datadog-secret",
							},
						},
					},
				},
				{
					Name:  "DD_CLUSTER_AGENT_URL",
					Value: fmt.Sprintf("https://datadog-cluster-agent.%s.svc.cluster.local:5005", apicommon.GetMyNamespace()),
				},
				{
					Name:  "DD_ORCHESTRATOR_EXPLORER_ENABLED",
					Value: "true",
				},
			},
		},
		{
			name: "cluster agent enabled with custom values",
			setConfig: func() {
				mockConfig := config.Mock(t)
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.cluster_agent.enabled", true)
				mockConfig.SetWithoutSource("cluster_agent.cmd_port", 12345)
				mockConfig.SetWithoutSource("cluster_agent.kubernetes_service_name", "test-service-name")
			},
			expectedEnvVars: []corev1.EnvVar{
				{
					Name:  "DD_CLUSTER_AGENT_ENABLED",
					Value: "true",
				},
				{
					Name: "DD_CLUSTER_AGENT_AUTH_TOKEN",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							Key: "token",
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "datadog-secret",
							},
						},
					},
				},
				{
					Name:  "DD_CLUSTER_AGENT_URL",
					Value: fmt.Sprintf("https://test-service-name.%s.svc.cluster.local:12345", apicommon.GetMyNamespace()),
				},
				{
					Name:  "DD_ORCHESTRATOR_EXPLORER_ENABLED",
					Value: "true",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			test.setConfig()
			sidecar := getDefaultSidecarTemplate(commonRegistry)
			envVarsMap := make(map[string]corev1.EnvVar)
			for _, envVar := range sidecar.Env {
				envVarsMap[envVar.Name] = envVar
			}

			for _, expectedVar := range test.expectedEnvVars {
				_, exist := envVarsMap[expectedVar.Name]
				assert.True(t, exist)
				assert.Equal(tt, expectedVar, envVarsMap[expectedVar.Name])
			}

			for _, unexpectedVar := range test.unexpectedEnvVars {
				_, exist := envVarsMap[unexpectedVar]
				assert.False(t, exist)
			}
		})
	}
}
