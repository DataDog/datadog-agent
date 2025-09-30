// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package leaderelection

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	rl "k8s.io/client-go/tools/leaderelection/resourcelock"

	cmLock "github.com/DataDog/datadog-agent/internal/third_party/client-go/tools/leaderelection/resourcelock"
)

func TestCreateLeaderTokenIfNotExists(t *testing.T) {
	tokenNamespace := "default"
	tokenName := "test-lease"

	tests := []struct {
		name         string
		lockType     string
		setupFunc    func(*fake.Clientset) error
		expectsError bool
		verifyFunc   func(*testing.T, *fake.Clientset)
	}{
		{
			name:         "Leases - lease does not exist",
			lockType:     rl.LeasesResourceLock,
			expectsError: false,
			verifyFunc: func(t *testing.T, client *fake.Clientset) {
				_, err := client.CoordinationV1().Leases(tokenNamespace).Get(context.TODO(), tokenName, metav1.GetOptions{})
				require.NoError(t, err)

				// Regression test: check that it does not create a ConfigMap too
				_, err = client.CoreV1().ConfigMaps(tokenNamespace).Get(context.TODO(), tokenName, metav1.GetOptions{})
				require.True(t, errors.IsNotFound(err))
			},
		},
		{
			name:         "ConfigMap - configmap does not exist",
			lockType:     cmLock.ConfigMapsResourceLock,
			expectsError: false,
			verifyFunc: func(t *testing.T, client *fake.Clientset) {
				_, err := client.CoreV1().ConfigMaps(tokenNamespace).Get(context.TODO(), tokenName, metav1.GetOptions{})
				require.NoError(t, err)

				_, err = client.CoordinationV1().Leases(tokenNamespace).Get(context.TODO(), tokenName, metav1.GetOptions{})
				require.True(t, errors.IsNotFound(err))
			},
		},
		{
			name:     "Leases - lease already exists",
			lockType: rl.LeasesResourceLock,
			setupFunc: func(client *fake.Clientset) error {
				lease := &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tokenName,
						Namespace: tokenNamespace,
					},
					Spec: coordinationv1.LeaseSpec{},
				}
				_, err := client.CoordinationV1().Leases(tokenNamespace).Create(context.TODO(), lease, metav1.CreateOptions{})
				return err
			},
			expectsError: false,
			verifyFunc: func(t *testing.T, client *fake.Clientset) {
				_, err := client.CoordinationV1().Leases(tokenNamespace).Get(context.TODO(), tokenName, metav1.GetOptions{})
				require.NoError(t, err)
			},
		},
		{
			name:     "ConfigMap - configmap already exists",
			lockType: cmLock.ConfigMapsResourceLock,
			setupFunc: func(client *fake.Clientset) error {
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tokenName,
						Namespace: tokenNamespace,
					},
				}
				_, err := client.CoreV1().ConfigMaps(tokenNamespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
				return err
			},
			expectsError: false,
			verifyFunc: func(t *testing.T, client *fake.Clientset) {
				_, err := client.CoreV1().ConfigMaps(tokenNamespace).Get(context.TODO(), tokenName, metav1.GetOptions{})
				require.NoError(t, err)
			},
		},
		{
			name:     "Leases - lease created between check and create",
			lockType: rl.LeasesResourceLock,
			setupFunc: func(client *fake.Clientset) error {
				// Mock the Create method to return an "AlreadyExists" error. This
				// simulates another DCA creating the resource between the check and
				// the creation operations.
				client.PrependReactor("create", "leases", func(_ k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.NewAlreadyExists(coordinationv1.Resource("leases"), tokenName)
				})

				return nil
			},
			expectsError: false, // Should handle the race condition and not return error
		},
		{
			name:     "ConfigMap - configmap created between check and create",
			lockType: cmLock.ConfigMapsResourceLock,
			setupFunc: func(client *fake.Clientset) error {
				// Mock the Create method to return an "AlreadyExists" error. This
				// simulates another DCA creating the resource between the check and
				// the creation operations.
				client.PrependReactor("create", "configmaps", func(_ k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.NewAlreadyExists(corev1.Resource("configmaps"), tokenName)
				})

				return nil
			},
			expectsError: false, // Should handle the race condition and not return error
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := fake.NewClientset()

			le := &LeaderEngine{
				LeaseName:       tokenName,
				LeaderNamespace: tokenNamespace,
				coreClient:      client.CoreV1(),
				coordClient:     client.CoordinationV1(),
				lockType:        test.lockType,
			}

			if test.setupFunc != nil {
				err := test.setupFunc(client)
				require.NoError(t, err)
			}

			err := le.createLeaderTokenIfNotExists()

			if test.expectsError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			if test.verifyFunc != nil {
				test.verifyFunc(t, client)
			}
		})
	}
}
