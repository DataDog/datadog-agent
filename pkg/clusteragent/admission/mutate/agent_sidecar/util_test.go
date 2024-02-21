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
			expectIsSupported: false,
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
		name          string
		provider      string
		baseContainer *corev1.Container
		// assertions assume the order of overrides is deterministic
		// changing the order will cause the tests to fail
		expectedContainerAfterOverride *corev1.Container
	}{
		{
			name:                           "nil container should be skipped",
			provider:                       "fargate",
			baseContainer:                  nil,
			expectedContainerAfterOverride: nil,
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
		},
		{
			name:                           "unsupported provider",
			provider:                       "foo-provider",
			baseContainer:                  &corev1.Container{},
			expectedContainerAfterOverride: &corev1.Container{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			mockConfig.SetWithoutSource("admission_controller.agent_sidecar.provider", test.provider)
			applyProviderOverrides(test.baseContainer)

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
		})
	}
}
