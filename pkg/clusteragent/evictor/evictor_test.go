// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package evictor

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func newFakeClient(objects ...*unstructured.Unstructured) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	var runtimeObjects []runtime.Object
	for _, o := range objects {
		runtimeObjects = append(runtimeObjects, o)
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			podGVR: "PodList",
		},
		runtimeObjects...,
	)
}

func newFakePod(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func TestEvictSuccess(t *testing.T) {
	client := newFakeClient(newFakePod("default", "my-pod"))
	c := NewClient(client, nil)

	result, err := c.Evict(context.Background(), "default", "my-pod")

	require.NoError(t, err)
	assert.Equal(t, Evicted, result)
}

func TestEvictPDBBlocked(t *testing.T) {
	client := newFakeClient(newFakePod("default", "my-pod"))
	client.PrependReactor("create", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, &k8serrors.StatusError{
			ErrStatus: metav1.Status{Code: http.StatusTooManyRequests},
		}
	})
	c := NewClient(client, nil)

	result, err := c.Evict(context.Background(), "default", "my-pod")

	require.NoError(t, err)
	assert.Equal(t, PDBBlocked, result)
}

func TestEvictError(t *testing.T) {
	client := newFakeClient(newFakePod("default", "my-pod"))
	client.PrependReactor("create", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})
	c := NewClient(client, nil)

	result, err := c.Evict(context.Background(), "default", "my-pod")

	assert.Error(t, err)
	assert.Equal(t, Error, result)
}

func TestEvictNotLeader(t *testing.T) {
	client := newFakeClient()
	c := NewClient(client, func() bool { return false })

	result, err := c.Evict(context.Background(), "default", "my-pod")

	require.NoError(t, err)
	assert.Equal(t, Skipped, result)
	assert.Empty(t, client.Actions(), "no API call should be made when not leader")
}

func TestEvictLeadershipChange(t *testing.T) {
	client := newFakeClient(newFakePod("default", "my-pod"))
	isLeader := false
	c := NewClient(client, func() bool { return isLeader })

	// Not leader
	result, err := c.Evict(context.Background(), "default", "my-pod")
	require.NoError(t, err)
	assert.Equal(t, Skipped, result)
	assert.Empty(t, client.Actions())

	// Become leader
	isLeader = true
	result, err = c.Evict(context.Background(), "default", "my-pod")
	require.NoError(t, err)
	assert.Equal(t, Evicted, result)
	assert.NotEmpty(t, client.Actions())
}
