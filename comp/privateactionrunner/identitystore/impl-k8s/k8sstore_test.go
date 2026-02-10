// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package k8sstoreimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	identitystore "github.com/DataDog/datadog-agent/comp/privateactionrunner/identitystore/def"
	"github.com/DataDog/datadog-agent/pkg/config/mock"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func setupK8sTest(t *testing.T) (identitystore.Component, *fake.Clientset, config.Component) {
	// Create fake Kubernetes client
	fakeClient := fake.NewSimpleClientset()

	// Create mock config
	cfg := mock.New(t)
	cfg.SetWithoutSource(parIdentitySecretName, "test-identity")

	// Create mock logger
	logger := logmock.New(t)

	// Create k8s store with fake client
	store := &k8sStore{
		config:    cfg,
		log:       logger,
		client:    fakeClient,
		namespace: "test-namespace",
	}

	return store, fakeClient, cfg
}

func TestK8sStore_PersistAndGetIdentity(t *testing.T) {
	store, _, _ := setupK8sTest(t)
	ctx := context.Background()

	// Create test identity
	testIdentity := &identitystore.Identity{
		PrivateKey: "test-private-key-base64",
		URN:        "urn:dd:apps:on-prem-runner:us:123456789:runner-id-xyz",
	}

	// Persist identity
	err := store.PersistIdentity(ctx, testIdentity)
	require.NoError(t, err)

	// Retrieve identity
	retrievedIdentity, err := store.GetIdentity(ctx)
	require.NoError(t, err)
	require.NotNil(t, retrievedIdentity)

	// Verify identity matches
	assert.Equal(t, testIdentity.PrivateKey, retrievedIdentity.PrivateKey)
	assert.Equal(t, testIdentity.URN, retrievedIdentity.URN)
}

func TestK8sStore_GetIdentity_NotExists(t *testing.T) {
	store, _, _ := setupK8sTest(t)
	ctx := context.Background()

	// Try to get identity that doesn't exist
	identity, err := store.GetIdentity(ctx)
	require.NoError(t, err)
	assert.Nil(t, identity)
}

func TestK8sStore_PersistIdentity_Update(t *testing.T) {
	store, _, _ := setupK8sTest(t)
	ctx := context.Background()

	// Create and persist initial identity
	initialIdentity := &identitystore.Identity{
		PrivateKey: "initial-key",
		URN:        "urn:dd:apps:on-prem-runner:us:123:456",
	}
	err := store.PersistIdentity(ctx, initialIdentity)
	require.NoError(t, err)

	// Update with new identity
	updatedIdentity := &identitystore.Identity{
		PrivateKey: "updated-key",
		URN:        "urn:dd:apps:on-prem-runner:us:789:012",
	}
	err = store.PersistIdentity(ctx, updatedIdentity)
	require.NoError(t, err)

	// Retrieve and verify updated identity
	retrievedIdentity, err := store.GetIdentity(ctx)
	require.NoError(t, err)
	require.NotNil(t, retrievedIdentity)

	assert.Equal(t, updatedIdentity.PrivateKey, retrievedIdentity.PrivateKey)
	assert.Equal(t, updatedIdentity.URN, retrievedIdentity.URN)
	assert.NotEqual(t, initialIdentity.PrivateKey, retrievedIdentity.PrivateKey)
}

func TestK8sStore_GetIdentity_MissingPrivateKey(t *testing.T) {
	store, fakeClient, cfg := setupK8sTest(t)
	ctx := context.Background()

	// Create secret with missing private_key field
	secretName := cfg.GetString(parIdentitySecretName)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{
			urnField: []byte("urn:dd:apps:on-prem-runner:us:123:456"),
		},
	}
	_, err := fakeClient.CoreV1().Secrets("test-namespace").Create(ctx, secret, metav1.CreateOptions{})
	require.NoError(t, err)

	// Try to get identity
	identity, err := store.GetIdentity(ctx)
	assert.Error(t, err)
	assert.Nil(t, identity)
	assert.Contains(t, err.Error(), "private_key field is missing or empty")
}

func TestK8sStore_GetIdentity_MissingURN(t *testing.T) {
	store, fakeClient, cfg := setupK8sTest(t)
	ctx := context.Background()

	// Create secret with missing urn field
	secretName := cfg.GetString(parIdentitySecretName)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{
			privateKeyField: []byte("test-key"),
		},
	}
	_, err := fakeClient.CoreV1().Secrets("test-namespace").Create(ctx, secret, metav1.CreateOptions{})
	require.NoError(t, err)

	// Try to get identity
	identity, err := store.GetIdentity(ctx)
	assert.Error(t, err)
	assert.Nil(t, identity)
	assert.Contains(t, err.Error(), "urn field is missing or empty")
}

func TestK8sStore_DeleteIdentity(t *testing.T) {
	store, _, _ := setupK8sTest(t)
	ctx := context.Background()

	// Create and persist identity
	testIdentity := &identitystore.Identity{
		PrivateKey: "test-private-key",
		URN:        "urn:dd:apps:on-prem-runner:us:123:456",
	}
	err := store.PersistIdentity(ctx, testIdentity)
	require.NoError(t, err)

	// Verify identity exists
	identity, err := store.GetIdentity(ctx)
	require.NoError(t, err)
	assert.NotNil(t, identity)

	// Delete identity
	err = store.DeleteIdentity(ctx)
	require.NoError(t, err)

	// Verify identity is deleted
	identity, err = store.GetIdentity(ctx)
	require.NoError(t, err)
	assert.Nil(t, identity)
}

func TestK8sStore_DeleteIdentity_NotExists(t *testing.T) {
	store, _, _ := setupK8sTest(t)
	ctx := context.Background()

	// Delete identity that doesn't exist (should not error)
	err := store.DeleteIdentity(ctx)
	assert.NoError(t, err)
}

func TestK8sStore_SecretLabels(t *testing.T) {
	store, fakeClient, cfg := setupK8sTest(t)
	ctx := context.Background()

	// Create and persist identity
	testIdentity := &identitystore.Identity{
		PrivateKey: "test-key",
		URN:        "urn:dd:apps:on-prem-runner:us:123:456",
	}
	err := store.PersistIdentity(ctx, testIdentity)
	require.NoError(t, err)

	// Get the created secret
	secretName := cfg.GetString(parIdentitySecretName)
	secret, err := fakeClient.CoreV1().Secrets("test-namespace").Get(ctx, secretName, metav1.GetOptions{})
	require.NoError(t, err)

	// Verify labels
	expectedLabels := map[string]string{
		"app.kubernetes.io/name":       "datadog-cluster-agent",
		"app.kubernetes.io/component":  "private-action-runner",
		"app.kubernetes.io/managed-by": "datadog-cluster-agent",
	}
	assert.Equal(t, expectedLabels, secret.Labels)
}

func TestK8sStore_CustomSecretName(t *testing.T) {
	store, fakeClient, cfg := setupK8sTest(t)
	ctx := context.Background()

	// Set custom secret name
	customSecretName := "custom-par-identity"
	cfg.SetWithoutSource(parIdentitySecretName, customSecretName)

	// Create and persist identity
	testIdentity := &identitystore.Identity{
		PrivateKey: "test-key",
		URN:        "urn:dd:apps:on-prem-runner:us:123:456",
	}
	err := store.PersistIdentity(ctx, testIdentity)
	require.NoError(t, err)

	// Verify secret was created with custom name
	secret, err := fakeClient.CoreV1().Secrets("test-namespace").Get(ctx, customSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, customSecretName, secret.Name)
}

func TestK8sStore_GetIdentity_K8sAPIError(t *testing.T) {
	store, fakeClient, _ := setupK8sTest(t)
	ctx := context.Background()

	// Inject error into fake client
	fakeClient.PrependReactor("get", "secrets", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, k8serrors.NewInternalError(assert.AnError)
	})

	// Try to get identity
	identity, err := store.GetIdentity(ctx)
	assert.Error(t, err)
	assert.Nil(t, identity)
	assert.Contains(t, err.Error(), "failed to get identity secret")
}

func TestK8sStore_PersistIdentity_CreateError(t *testing.T) {
	store, fakeClient, _ := setupK8sTest(t)
	ctx := context.Background()

	// Inject error into fake client for create operation
	fakeClient.PrependReactor("create", "secrets", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, k8serrors.NewInternalError(assert.AnError)
	})

	// Try to persist identity
	testIdentity := &identitystore.Identity{
		PrivateKey: "test-key",
		URN:        "urn:dd:apps:on-prem-runner:us:123:456",
	}
	err := store.PersistIdentity(ctx, testIdentity)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create secret")
}

func TestK8sStore_PersistIdentity_UpdateError(t *testing.T) {
	store, fakeClient, _ := setupK8sTest(t)
	ctx := context.Background()

	// Create initial secret
	testIdentity := &identitystore.Identity{
		PrivateKey: "test-key",
		URN:        "urn:dd:apps:on-prem-runner:us:123:456",
	}
	err := store.PersistIdentity(ctx, testIdentity)
	require.NoError(t, err)

	// Inject error into fake client for update operation
	fakeClient.PrependReactor("update", "secrets", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, k8serrors.NewInternalError(assert.AnError)
	})

	// Try to update identity
	updatedIdentity := &identitystore.Identity{
		PrivateKey: "updated-key",
		URN:        "urn:dd:apps:on-prem-runner:us:789:012",
	}
	err = store.PersistIdentity(ctx, updatedIdentity)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update existing secret")
}

func TestK8sStore_DeleteIdentity_Error(t *testing.T) {
	store, fakeClient, _ := setupK8sTest(t)
	ctx := context.Background()

	// Create initial secret
	testIdentity := &identitystore.Identity{
		PrivateKey: "test-key",
		URN:        "urn:dd:apps:on-prem-runner:us:123:456",
	}
	err := store.PersistIdentity(ctx, testIdentity)
	require.NoError(t, err)

	// Inject error into fake client for delete operation
	fakeClient.PrependReactor("delete", "secrets", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		// Return a non-NotFound error
		return true, nil, k8serrors.NewForbidden(schema.GroupResource{}, "test", assert.AnError)
	})

	// Try to delete identity
	err = store.DeleteIdentity(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete identity secret")
}

func TestK8sStore_GetSecretName_Default(t *testing.T) {
	cfg := mock.New(t)
	logger := logmock.New(t)

	// Don't set custom secret name
	store := &k8sStore{
		config:    cfg,
		log:       logger,
		namespace: "test-namespace",
	}

	secretName := store.getSecretName()
	assert.Equal(t, defaultSecretName, secretName)
}

func TestK8sStore_GetSecretName_Custom(t *testing.T) {
	cfg := mock.New(t)
	logger := logmock.New(t)

	customName := "my-custom-secret"
	cfg.SetWithoutSource(parIdentitySecretName, customName)

	store := &k8sStore{
		config:    cfg,
		log:       logger,
		namespace: "test-namespace",
	}

	secretName := store.getSecretName()
	assert.Equal(t, customName, secretName)
}
