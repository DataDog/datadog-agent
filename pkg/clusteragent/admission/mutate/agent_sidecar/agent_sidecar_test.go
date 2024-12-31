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

	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	apicommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const commonRegistry = "gcr.io/datadoghq"

func TestInjectAgentSidecar(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("admission_controller.agent_sidecar.container_registry", commonRegistry)
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
			provider: "",
			profilesJSON: `[{
				"securityContext": {
					"readOnlyRootFilesystem": true
				}
			}]`,
			ExpectError:     false,
			ExpectInjection: true,
			ExpectedPodAfterInjection: func() *corev1.Pod {
				webhook := NewWebhook(mockConfig)
				sidecar := webhook.getDefaultSidecarTemplate()
				webhook.addSecurityConfigToAgent(sidecar)
				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-name",
					},
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{*webhook.getSecurityInitTemplate()},
						Containers: []corev1.Container{
							{Name: "container-name"},
							*sidecar,
						},
						Volumes: *webhook.getSecurityVolumeTemplates(),
					},
				}
			},
		},
		{
			Name: "should inject sidecar, no security features if default overridden to false",
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
			provider: "",
			profilesJSON: `[{
				"securityContext": {
					"readOnlyRootFilesystem": false
				}
			}]`,
			ExpectError:     false,
			ExpectInjection: true,
			ExpectedPodAfterInjection: func() *corev1.Pod {
				sidecar := *NewWebhook(mockConfig).getDefaultSidecarTemplate()
				// Records the false readOnlyRootFilesystem but doesn't add the initContainers, volumes and mounts
				sidecar.SecurityContext = &corev1.SecurityContext{
					ReadOnlyRootFilesystem: pointer.Ptr(false),
				}
				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-name",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "container-name"},
							sidecar,
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
						*NewWebhook(mockConfig).getDefaultSidecarTemplate(),
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
							*NewWebhook(mockConfig).getDefaultSidecarTemplate(),
						},
					},
				}
			},
		},
		{
			Name: "should inject sidecar if no sidecar present, no provider set, owned by Job",
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-name",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "batch/v1",
							Kind:       "Job",
						},
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "container-name"},
					},
				},
			},
			provider: "",
			profilesJSON: `[{
				"securityContext": {
					"readOnlyRootFilesystem": false
				}
			}]`,
			ExpectError:     false,
			ExpectInjection: true,
			ExpectedPodAfterInjection: func() *corev1.Pod {
				defaultContainer := *NewWebhook(mockConfig).getDefaultSidecarTemplate()
				// Update envvar when pod owned by Job
				defaultContainer.Env = append(defaultContainer.Env, corev1.EnvVar{
					Name:  "DD_AUTO_EXIT_NOPROCESS_ENABLED",
					Value: "true",
				})
				defaultContainer.SecurityContext = &corev1.SecurityContext{
					ReadOnlyRootFilesystem: pointer.Ptr(false),
				}
				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-name",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "batch/v1",
								Kind:       "Job",
							},
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "container-name"},
							defaultContainer,
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
			provider: "fargate",
			profilesJSON: `[{
				"securityContext": {
					"readOnlyRootFilesystem": true
				}
			}]`,
			ExpectError:     false,
			ExpectInjection: true,
			ExpectedPodAfterInjection: func() *corev1.Pod {
				webhook := NewWebhook(mockConfig)
				sidecar := webhook.getDefaultSidecarTemplate()
				webhook.addSecurityConfigToAgent(sidecar)
				_, _ = withEnvOverrides(
					sidecar,
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

				sidecar.VolumeMounts = append(sidecar.VolumeMounts, []corev1.VolumeMount{
					{
						Name:      "ddsockets",
						MountPath: "/var/run/datadog",
						ReadOnly:  false,
					},
				}...)

				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-name",
						Annotations: map[string]string{
							mutatecommon.K8sAutoscalerSafeToEvictVolumesAnnotation: "ddsockets",
						},
					},
					Spec: corev1.PodSpec{
						ShareProcessNamespace: pointer.Ptr(true),
						InitContainers:        []corev1.Container{*webhook.getSecurityInitTemplate()},
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
							*sidecar,
						},
						Volumes: append(*webhook.getSecurityVolumeTemplates(),
							corev1.Volume{
								Name: "ddsockets",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						),
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
		    },
			"securityContext": {
				"readOnlyRootFilesystem": true
			}
		}]`,
			ExpectError:     false,
			ExpectInjection: true,
			ExpectedPodAfterInjection: func() *corev1.Pod {
				webhook := NewWebhook(mockConfig)
				sidecar := webhook.getDefaultSidecarTemplate()
				webhook.addSecurityConfigToAgent(sidecar)

				_, _ = withEnvOverrides(
					sidecar,
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

				_ = withResourceLimits(sidecar, corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("1"), "memory": resource.MustParse("512Mi")},
					Requests: corev1.ResourceList{"cpu": resource.MustParse("0.5"), "memory": resource.MustParse("256Mi")},
				})

				sidecar.VolumeMounts = append(sidecar.VolumeMounts, []corev1.VolumeMount{
					{
						Name:      "ddsockets",
						MountPath: "/var/run/datadog",
						ReadOnly:  false,
					},
				}...)

				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-name",
						Annotations: map[string]string{
							mutatecommon.K8sAutoscalerSafeToEvictVolumesAnnotation: "ddsockets",
						},
					},
					Spec: corev1.PodSpec{
						ShareProcessNamespace: pointer.Ptr(true),
						InitContainers:        []corev1.Container{*webhook.getSecurityInitTemplate()},
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
							*sidecar,
						},
						Volumes: append(*webhook.getSecurityVolumeTemplates(),
							corev1.Volume{
								Name: "ddsockets",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						),
					},
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(tt *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("admission_controller.agent_sidecar.provider", test.provider)
			mockConfig.SetWithoutSource("admission_controller.agent_sidecar.profiles", test.profilesJSON)

			webhook := NewWebhook(mockConfig)

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
		setConfig         func() model.Config
		containerRegistry string
		expectedImage     string
	}{
		{
			name:              "no configuration set",
			setConfig:         func() model.Config { return configmock.New(t) },
			containerRegistry: commonRegistry,
			expectedImage:     fmt.Sprintf("%s/agent:latest", commonRegistry),
		},
		{
			name:              "setting custom registry, image and tag",
			containerRegistry: "my-registry",
			setConfig: func() model.Config {
				mockConfig := configmock.New(t)
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.container_registry", "my-registry")
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.image_name", "my-image")
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.image_tag", "my-tag")
				return mockConfig
			},
			expectedImage: "my-registry/my-image:my-tag",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			mockConfig := test.setConfig()
			sidecar := NewWebhook(mockConfig).getDefaultSidecarTemplate()
			assert.Equal(tt, test.expectedImage, sidecar.Image)
		})
	}
}

func TestDefaultSidecarTemplateClusterAgentEnvVars(t *testing.T) {

	tests := []struct {
		name              string
		setConfig         func() model.Config
		expectedEnvVars   []corev1.EnvVar
		unexpectedEnvVars []string
	}{
		{
			name: "cluster agent not enabled",
			setConfig: func() model.Config {
				mockConfig := configmock.New(t)
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.cluster_agent.enabled", false)
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.container_registry", commonRegistry)
				return mockConfig
			},
			expectedEnvVars: []corev1.EnvVar{
				{
					Name:  "DD_LANGUAGE_DETECTION_ENABLED",
					Value: "false",
				},
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
			setConfig: func() model.Config {
				mockConfig := configmock.New(t)
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.cluster_agent.enabled", true)
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.container_registry", commonRegistry)
				return mockConfig
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
				{
					Name:  "DD_LANGUAGE_DETECTION_ENABLED",
					Value: "false",
				},
			},
		},
		{
			name: "cluster agent enabled with language derection enabled",
			setConfig: func() model.Config {
				mockConfig := configmock.New(t)
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.cluster_agent.enabled", true)
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.container_registry", commonRegistry)
				mockConfig.SetWithoutSource("language_detection.enabled", true)
				return mockConfig
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
				{
					Name:  "DD_LANGUAGE_DETECTION_ENABLED",
					Value: "true",
				},
			},
		},
		{
			name: "cluster agent enabled with custom values",
			setConfig: func() model.Config {
				mockConfig := configmock.New(t)
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.cluster_agent.enabled", true)
				mockConfig.SetWithoutSource("admission_controller.agent_sidecar.container_registry", commonRegistry)
				mockConfig.SetWithoutSource("cluster_agent.cmd_port", 12345)
				mockConfig.SetWithoutSource("cluster_agent.kubernetes_service_name", "test-service-name")
				mockConfig.SetWithoutSource("language_detection.enabled", "false")
				return mockConfig
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
				{
					Name:  "DD_LANGUAGE_DETECTION_ENABLED",
					Value: "false",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			mockConfig := test.setConfig()
			sidecar := NewWebhook(mockConfig).getDefaultSidecarTemplate()
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

func TestIsReadOnlyRootFilesystem(t *testing.T) {
	tests := []struct {
		name     string
		profile  string
		expected bool
	}{
		{
			name:     "no profile",
			profile:  "",
			expected: false,
		},
		{
			name:     "empty or default profile",
			profile:  "[]",
			expected: false,
		},
		{
			name: "profile without security context",
			profile: `[{
				"env": [
					{"name": "ENV_VAR_2", "value": "value2"}
				],
			}]`,
			expected: false,
		},
		{
			name: "profile with security context, readOnlyRootFilesystem empty",
			profile: `[{
				"securityContext": {}
			}]`,
			expected: false,
		},
		{
			name: "profile with security context, readOnlyRootFilesystem true",
			profile: `[{
				"securityContext": {
					"readOnlyRootFilesystem": true
				}
			}]`,
			expected: true,
		},
		{
			name: "profile with security context, readOnlyRootFilesystem false",
			profile: `[{
				"securityContext": {
					"readOnlyRootFilesystem": false
				}
			}]`,
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("admission_controller.agent_sidecar.profiles", test.profile)
			webhook := NewWebhook(mockConfig)
			sidecar := webhook.getDefaultSidecarTemplate()

			// Webhook properly parses profile config
			assert.Equal(tt, test.expected, webhook.isReadOnlyRootFilesystem())

			if test.expected {
				// Webhook properly applies the security context to the sidecar
				webhook.addSecurityConfigToAgent(sidecar)
				assert.NotNil(t, sidecar.SecurityContext)
				assert.NotNil(t, sidecar.SecurityContext.ReadOnlyRootFilesystem)
				assert.Equal(t, test.expected, *sidecar.SecurityContext.ReadOnlyRootFilesystem)
			} else {
				assert.Nil(t, sidecar.SecurityContext)
				profile, _ := loadSidecarProfiles(test.profile)
				applyProfileOverrides(sidecar, profile)
			}
		})
	}
}
