// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestLoadSidecarProfiles(t *testing.T) {
	mockConfig := config.Mock(t)

	tests := []struct {
		name             string
		profilesJSON     string
		expectedProfiles []ProfileOverride
		expectError      bool
	}{
		{
			name:             "misconfigured profiles",
			profilesJSON:     "I am a misconfigurations (^_^)",
			expectedProfiles: nil,
			expectError:      true,
		},
		{
			name: "single valid profile",
			profilesJSON: `[
				{
					"env": [
						{"name": "ENV_VAR_1", "value": "value1"},
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
				}
			]`,
			expectedProfiles: []ProfileOverride{
				{
					EnvVars: []corev1.EnvVar{
						{Name: "ENV_VAR_1", Value: "value1"},
						{Name: "ENV_VAR_2", Value: "value2"},
					},
					ResourceRequirements: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"cpu":    resource.MustParse("1"),
							"memory": resource.MustParse("512Mi"),
						},
						Requests: corev1.ResourceList{
							"cpu":    resource.MustParse("0.5"),
							"memory": resource.MustParse("256Mi"),
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "multiple profiles",
			profilesJSON: `[
				{
					"env": [
						{"name": "ENV_VAR_1", "value": "value1"},
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
				},
				{
					"env": [
						{"name": "ENV_VAR_1", "value": "value1"},
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
				}
			]`,
			expectedProfiles: nil,
			expectError:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			mockConfig.SetWithoutSource("admission_controller.agent_sidecar.profiles", test.profilesJSON)
			profiles, err := loadSidecarProfiles()

			if test.expectError {
				assert.Error(tt, err)
				assert.Nil(tt, profiles)
			} else {
				assert.NoError(tt, err)
				assert.Truef(
					tt,
					reflect.DeepEqual(test.expectedProfiles, profiles),
					"expected %v, found %v",
					test.expectedProfiles,
					profiles,
				)
			}
		})
	}

}

func TestApplyProfileOverrides(t *testing.T) {
	mockConfig := config.Mock(t)

	tests := []struct {
		name              string
		profilesJSON      string
		baseContainer     *corev1.Container
		expectedContainer *corev1.Container
		expectError       bool
		expectMutated     bool
	}{
		{
			name:              "nil container should be skipped",
			profilesJSON:      "",
			baseContainer:     nil,
			expectedContainer: nil,
			expectError:       true,
			expectMutated:     false,
		},
		{
			name: "apply profile overrides",
			profilesJSON: `[
				{
					"env": [
						{"name": "ENV_VAR_1", "value": "value1"},
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
				}
			]`,
			baseContainer: &corev1.Container{},
			expectedContainer: &corev1.Container{
				Env: []corev1.EnvVar{
					{Name: "ENV_VAR_1", Value: "value1"},
					{Name: "ENV_VAR_2", Value: "value2"},
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cpu":    resource.MustParse("1"),
						"memory": resource.MustParse("512Mi"),
					},
					Requests: corev1.ResourceList{
						"cpu":    resource.MustParse("0.5"),
						"memory": resource.MustParse("256Mi"),
					},
				},
			},
			expectError:   false,
			expectMutated: true,
		},
		{
			name:         "no profile provided",
			profilesJSON: `[]`,
			baseContainer: &corev1.Container{
				Env: []corev1.EnvVar{{Name: "EXISTING_VAR", Value: "existing_value"}},
				Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("2"), "memory": resource.MustParse("1Gi")},
					Requests: corev1.ResourceList{"cpu": resource.MustParse("1"), "memory": resource.MustParse("512Mi")},
				},
			},
			expectedContainer: &corev1.Container{
				Env: []corev1.EnvVar{{Name: "EXISTING_VAR", Value: "existing_value"}},
				Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("2"), "memory": resource.MustParse("1Gi")},
					Requests: corev1.ResourceList{"cpu": resource.MustParse("1"), "memory": resource.MustParse("512Mi")},
				},
			},
			expectError:   false,
			expectMutated: false,
		},
		{
			name: "empty profile provided",
			profilesJSON: `[
				{}
			]`,
			baseContainer: &corev1.Container{
				Env: []corev1.EnvVar{{Name: "EXISTING_VAR", Value: "existing_value"}},
				Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("2"), "memory": resource.MustParse("1Gi")},
					Requests: corev1.ResourceList{"cpu": resource.MustParse("1"), "memory": resource.MustParse("512Mi")},
				},
			},
			expectedContainer: &corev1.Container{
				Env: []corev1.EnvVar{{Name: "EXISTING_VAR", Value: "existing_value"}},
				Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("2"), "memory": resource.MustParse("1Gi")},
					Requests: corev1.ResourceList{"cpu": resource.MustParse("1"), "memory": resource.MustParse("512Mi")},
				},
			},
			expectError:   false,
			expectMutated: false,
		},
		{
			name: "apply profile overrides with ValueFrom",
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
			baseContainer: &corev1.Container{},
			expectedContainer: &corev1.Container{
				Env: []corev1.EnvVar{
					{Name: "ENV_VAR_1", ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{Key: "secret-key", LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}},
					}},
					{Name: "ENV_VAR_2", Value: "value2"},
				},
				Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("1"), "memory": resource.MustParse("512Mi")},
					Requests: corev1.ResourceList{"cpu": resource.MustParse("0.5"), "memory": resource.MustParse("256Mi")},
				},
			},
			expectError:   false,
			expectMutated: true,
		},
		{
			name: "profile overrides should override any existing configuration",
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
			baseContainer: &corev1.Container{
				Env: []corev1.EnvVar{
					{Name: "EXISTING_VAR", Value: "existing_value"},
					{Name: "ENV_VAR_1", Value: "value-existing"},
				},
				Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("2"), "memory": resource.MustParse("1Gi")},
					Requests: corev1.ResourceList{"cpu": resource.MustParse("1"), "memory": resource.MustParse("512Mi")},
				},
			},

			expectedContainer: &corev1.Container{
				Env: []corev1.EnvVar{
					{Name: "EXISTING_VAR", Value: "existing_value"},
					{Name: "ENV_VAR_1", ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{Key: "secret-key", LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}},
					}},
					{Name: "ENV_VAR_2", Value: "value2"},
				},
				Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("1"), "memory": resource.MustParse("512Mi")},
					Requests: corev1.ResourceList{"cpu": resource.MustParse("0.5"), "memory": resource.MustParse("256Mi")},
				},
			},
			expectError:   false,
			expectMutated: true,
		},
		{
			name: "more than one profile provided",
			profilesJSON: `[
        {
            "env": [
                {"name": "ENV_VAR_1", "value": "value1"}
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
        },
        {
            "env": [
                {"name": "ENV_VAR_3", "value": "value3"}
            ],
            "resources": {
                "limits": {
                    "cpu": "2",
                    "memory": "1Gi"
                },
                "requests": {
                    "cpu": "1",
                    "memory": "512Mi"
                }
            }
        }
    ]`,
			baseContainer:     &corev1.Container{},
			expectedContainer: &corev1.Container{},
			expectError:       true,
			expectMutated:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			mockConfig.SetWithoutSource("admission_controller.agent_sidecar.profiles", test.profilesJSON)
			mutated, err := applyProfileOverrides(test.baseContainer)

			assert.Equal(tt, test.expectMutated, mutated)

			if test.expectError {
				assert.Error(tt, err)
			} else {
				assert.NoError(tt, err)
				if test.expectedContainer == nil {
					assert.Nil(tt, test.baseContainer)
				} else {
					assert.NotNil(tt, test.baseContainer)
					assert.Truef(tt,
						reflect.DeepEqual(*test.baseContainer, *test.expectedContainer),
						"overrides not applied as expected. expected %v, but found %v",
						*test.expectedContainer,
						*test.baseContainer,
					)
				}
			}

		})
	}
}
