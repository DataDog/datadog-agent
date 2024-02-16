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
	"reflect"
	"testing"
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
			isSupported := ProviderIsSupported(test.provider)
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

	tests := []struct {
		name                           string
		provider                       string
		baseContainer                  *corev1.Container
		expectedContainerAfterOverride *corev1.Container
		expectError                    bool
	}{
		{
			name:                           "nil container should be skipped",
			provider:                       "fargate",
			baseContainer:                  nil,
			expectedContainerAfterOverride: nil,
			expectError:                    true,
		},
		{
			name:                           "empty provider",
			provider:                       "",
			baseContainer:                  &corev1.Container{},
			expectedContainerAfterOverride: &corev1.Container{},
			expectError:                    false,
		},
		{
			name:          "fargate provider",
			provider:      "fargate",
			baseContainer: &corev1.Container{},
			expectedContainerAfterOverride: &corev1.Container{
				Env: []corev1.EnvVar{
					{
						Name:  "DD_EKS_FARGATE",
						Value: "true",
					},
				},
			},
			expectError: false,
		},
		{
			name:                           "unsupported provider",
			provider:                       "foo-provider",
			baseContainer:                  &corev1.Container{},
			expectedContainerAfterOverride: &corev1.Container{},
			expectError:                    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			mockConfig.SetWithoutSource("admission_controller.agent_sidecar.provider", test.provider)
			err := applyProviderOverrides(test.baseContainer)

			if test.expectError {
				assert.Error(tt, err)
			} else {
				assert.NoError(tt, err)

				if test.expectedContainerAfterOverride == nil {
					assert.Nil(tt, test.baseContainer)
				} else {
					assert.NotNil(tt, test.baseContainer)
					assert.Truef(tt,
						reflect.DeepEqual(*test.baseContainer, *test.expectedContainerAfterOverride),
						"overrides not applied as expected. expected %v, but found %v",
						*test.expectedContainerAfterOverride,
						*test.baseContainer,
					)
				}
			}

		})
	}
}
