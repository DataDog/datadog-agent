// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"testing"
)

func withEnvOverrides(container *corev1.Container, extraEnv ...corev1.EnvVar) {
	for _, envVarOverride := range extraEnv {
		// Check if the environment variable already exists in the container
		var found bool
		for i, envVar := range container.Env {
			if envVar.Name == envVarOverride.Name {
				// Override the existing environment variable value
				container.Env[i] = envVarOverride
				found = true
				break
			}
		}
		// If the environment variable doesn't exist, add it to the container
		if !found {
			container.Env = append(container.Env, envVarOverride)
		}
	}
}

func withResourceLimits(container *corev1.Container, resourceLimits corev1.ResourceRequirements) {
	container.Resources = resourceLimits
}

func TestInjectAgentSidecar(t *testing.T) {
	mockConfig := config.Mock(t)

	tests := []struct {
		Name                      string
		Pod                       *corev1.Pod
		provider                  string
		profilesJSON              string
		ExpectError               bool
		ExpectedPodAfterInjection func() *corev1.Pod
	}{
		{
			Name:                      "should return error for nil pod",
			Pod:                       nil,
			provider:                  "",
			profilesJSON:              "",
			ExpectError:               true,
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
			provider:     "",
			profilesJSON: "[]",
			ExpectError:  false,
			ExpectedPodAfterInjection: func() *corev1.Pod {
				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-name",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "container-name"},
							*getDefaultSidecarTemplate(),
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
						*getDefaultSidecarTemplate(),
					},
				},
			},
			provider:     "",
			profilesJSON: "[]",
			ExpectError:  false,
			ExpectedPodAfterInjection: func() *corev1.Pod {
				return &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-name",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "container-name"},
							*getDefaultSidecarTemplate(),
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
			provider:     "fargate",
			profilesJSON: "[]",
			ExpectError:  false,
			ExpectedPodAfterInjection: func() *corev1.Pod {
				sidecar := *getDefaultSidecarTemplate()
				withEnvOverrides(&sidecar, corev1.EnvVar{
					Name:  "DD_EKS_FARGATE",
					Value: "true",
				})

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
			ExpectError: false,
			ExpectedPodAfterInjection: func() *corev1.Pod {
				sidecar := *getDefaultSidecarTemplate()

				withEnvOverrides(&sidecar, corev1.EnvVar{
					Name:  "DD_EKS_FARGATE",
					Value: "true",
				}, corev1.EnvVar{Name: "ENV_VAR_1", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{Key: "secret-key", LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}},
				}},
					corev1.EnvVar{Name: "ENV_VAR_2", Value: "value2"})

				withResourceLimits(&sidecar, corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("1"), "memory": resource.MustParse("512Mi")},
					Requests: corev1.ResourceList{"cpu": resource.MustParse("0.5"), "memory": resource.MustParse("256Mi")},
				})

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
	}

	for _, test := range tests {
		t.Run(test.Name, func(tt *testing.T) {
			mockConfig.SetWithoutSource("admission_controller.agent_sidecar.provider", test.provider)
			mockConfig.SetWithoutSource("admission_controller.agent_sidecar.profiles", test.profilesJSON)

			err := injectAgentSidecar(test.Pod, "", nil)

			if test.ExpectError {
				assert.Error(tt, err, "expected non-nil error to be returned")
			} else {
				assert.NoError(tt, err, "expected returned error to be nil")
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
