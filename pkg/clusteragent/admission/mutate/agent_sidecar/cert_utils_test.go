// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestEnsureCACertConfigMapInNamespace(t *testing.T) {
	caCertData := map[string]string{"ca.crt": "cert-data"}
	updatedCACertData := map[string]string{"ca.crt": "new-cert-data"}

	tests := []struct {
		name          string
		existingCM    *corev1.ConfigMap
		injectReactor func(fakeClient *fake.Clientset)
		caCertData    map[string]string
		expectError   bool
		expectCMData  map[string]string
	}{
		{
			name:         "creates ConfigMap when it does not exist",
			existingCM:   nil,
			caCertData:   caCertData,
			expectError:  false,
			expectCMData: caCertData,
		},
		{
			name: "no-op when ConfigMap is up-to-date",
			existingCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapCAName,
					Namespace: "test-ns",
				},
				Data: caCertData,
			},
			caCertData:   caCertData,
			expectError:  false,
			expectCMData: caCertData,
		},
		{
			name: "updates ConfigMap when data is stale",
			existingCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapCAName,
					Namespace: "test-ns",
				},
				Data: caCertData,
			},
			caCertData:   updatedCACertData,
			expectError:  false,
			expectCMData: updatedCACertData,
		},
		{
			// Simulates the race condition where multiple concurrent admission requests
			// for the same namespace each observe NotFound, then one Create succeeds
			// while others receive AlreadyExists. The function should treat AlreadyExists
			// as success so TLS injection proceeds for all pods.
			name:       "returns nil when Create races and receives AlreadyExists",
			existingCM: nil,
			injectReactor: func(fakeClient *fake.Clientset) {
				fakeClient.Fake.PrependReactor("create", "configmaps", func(_ k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewAlreadyExists(corev1.Resource("configmaps"), configMapCAName)
				})
			},
			caCertData:  caCertData,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objs []runtime.Object
			if tt.existingCM != nil {
				objs = append(objs, tt.existingCM)
			}
			fakeClient := fake.NewSimpleClientset(objs...)
			if tt.injectReactor != nil {
				tt.injectReactor(fakeClient)
			}

			err := ensureCACertConfigMapInNamespace("test-ns", tt.caCertData, fakeClient)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectCMData != nil {
				cm, getErr := fakeClient.CoreV1().ConfigMaps("test-ns").Get(context.TODO(), configMapCAName, metav1.GetOptions{})
				require.NoError(t, getErr)
				assert.Equal(t, tt.expectCMData, cm.Data)
			}
		})
	}
}
