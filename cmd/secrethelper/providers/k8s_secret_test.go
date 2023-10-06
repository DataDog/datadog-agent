// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets

package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestReadKubernetesSecret(t *testing.T) {
	tests := []struct {
		name            string
		existingSecrets []v1.Secret
		secretPath      string
		expectedValue   string
		expectedError   string
	}{
		{
			name:            "invalid path format",
			existingSecrets: []v1.Secret{},
			secretPath:      "not_valid",
			expectedError:   "invalid format. Use: \"namespace/name/key\"",
		},
		{
			name:            "secret does not exist",
			existingSecrets: []v1.Secret{},
			secretPath:      "some_namespace/some_name/some_key",
			expectedError:   "secrets \"some_name\" not found",
		},
		{
			name: "secret exists, but the key does not",
			existingSecrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some_name",
						Namespace: "some_namespace",
					},
					Data: map[string][]byte{"another_key": []byte("some_value")},
				},
			},
			secretPath:    "some_namespace/some_name/some_key",
			expectedError: "key some_key not found in secret some_namespace/some_name",
		},
		{
			name: "secret and key exist",
			existingSecrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some_name",
						Namespace: "some_namespace",
					},
					Data: map[string][]byte{"some_key": []byte("some_value")},
				},
			},
			secretPath:    "some_namespace/some_name/some_key",
			expectedValue: "some_value",
			expectedError: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var kubeObjects []runtime.Object
			for _, secret := range test.existingSecrets {
				kubeObjects = append(kubeObjects, &secret)
			}
			kubeClient := fake.NewSimpleClientset(kubeObjects...)

			resolvedSecret := ReadKubernetesSecret(kubeClient, test.secretPath)

			if test.expectedError != "" {
				assert.Equal(t, test.expectedError, resolvedSecret.ErrorMsg)
			} else {
				assert.Equal(t, test.expectedValue, resolvedSecret.Value)
				assert.Empty(t, test.expectedError, resolvedSecret.ErrorMsg)
			}
		})
	}
}
