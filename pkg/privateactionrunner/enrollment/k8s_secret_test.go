// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package enrollment

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const testPollInterval = 10 * time.Millisecond

func makeTestSecret(ns, name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			privateKeyField: []byte("test-key"),
			urnField:        []byte("urn:test"),
		},
	}
}

func newServerTimeoutError() *k8serrors.StatusError {
	return k8serrors.NewServerTimeout(
		schema.GroupResource{Group: "", Resource: "secrets"},
		"get",
		5,
	)
}

func TestWaitForLeaderAndSecret_RetriesTransientThenSucceeds(t *testing.T) {
	secret := makeTestSecret("default", "test-secret")

	client := fake.NewSimpleClientset()
	var callCount atomic.Int32
	client.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		n := callCount.Add(1)
		if n <= 3 {
			return true, nil, newServerTimeoutError()
		}
		return true, secret, nil
	})

	leadershipChange := make(chan struct{})
	isLeader := func() bool { return false }

	ctx := context.Background()
	result, err := waitForLeaderAndSecret(
		ctx, leadershipChange, isLeader, client, "default", "test-secret",
		testPollInterval,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "test-secret", result.Name)
	assert.GreaterOrEqual(t, callCount.Load(), int32(4))
}

// Tests that alternating between transient errors and NotFound responses
// doesn't break the function — it continues waiting and eventually succeeds.
func TestWaitForLeaderAndSecret_HandlesAlternatingTransientAndNotFound(t *testing.T) {
	secret := makeTestSecret("default", "test-secret")

	client := fake.NewSimpleClientset()
	var callCount atomic.Int32
	client.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		n := callCount.Add(1)
		switch {
		case n <= 2:
			return true, nil, newServerTimeoutError()
		case n <= 4:
			return true, nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, "test-secret")
		case n == 5:
			return true, nil, newServerTimeoutError()
		default:
			return true, secret, nil
		}
	})

	leadershipChange := make(chan struct{})
	isLeader := func() bool { return false }

	ctx := context.Background()
	result, err := waitForLeaderAndSecret(
		ctx, leadershipChange, isLeader, client, "default", "test-secret",
		testPollInterval,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "test-secret", result.Name)
}

func TestWaitForLeaderAndSecret_ExitsOnContextCancel(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, newServerTimeoutError()
	})

	leadershipChange := make(chan struct{})
	isLeader := func() bool { return false }

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := waitForLeaderAndSecret(
		ctx, leadershipChange, isLeader, client, "default", "test-secret",
		testPollInterval,
	)

	require.Error(t, err)
	require.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestWaitForLeaderAndSecret_ReturnsNilWhenBecomesLeader(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, "test-secret")
	})

	leadershipChange := make(chan struct{}, 1)
	var leader atomic.Bool
	isLeader := func() bool { return leader.Load() }

	go func() {
		time.Sleep(30 * time.Millisecond)
		leader.Store(true)
		leadershipChange <- struct{}{}
	}()

	ctx := context.Background()
	result, err := waitForLeaderAndSecret(
		ctx, leadershipChange, isLeader, client, "default", "test-secret",
		testPollInterval,
	)

	require.NoError(t, err)
	require.Nil(t, result)
}

func TestWaitForLeaderAndSecret_ReturnsNilIfAlreadyLeader(t *testing.T) {
	client := fake.NewSimpleClientset()

	leadershipChange := make(chan struct{})
	isLeader := func() bool { return true }

	ctx := context.Background()
	result, err := waitForLeaderAndSecret(
		ctx, leadershipChange, isLeader, client, "default", "test-secret",
		testPollInterval,
	)

	require.NoError(t, err)
	require.Nil(t, result)
}

func TestWaitForLeaderAndSecret_FailsImmediatelyOnNonTransientError(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewForbidden(
			schema.GroupResource{Resource: "secrets"}, "test-secret", errors.New("RBAC denied"))
	})

	leadershipChange := make(chan struct{})
	isLeader := func() bool { return false }

	ctx := context.Background()
	result, err := waitForLeaderAndSecret(
		ctx, leadershipChange, isLeader, client, "default", "test-secret",
		testPollInterval,
	)

	require.Error(t, err)
	require.Nil(t, result)
	assert.True(t, k8serrors.IsForbidden(errors.Unwrap(err)))
}

func TestIsNonTransientK8sError(t *testing.T) {
	gr := schema.GroupResource{Resource: "secrets"}

	nonTransient := []struct {
		name string
		err  error
	}{
		{"Forbidden", k8serrors.NewForbidden(gr, "s", errors.New("denied"))},
		{"Unauthorized", k8serrors.NewUnauthorized("bad token")},
		{"BadRequest", k8serrors.NewBadRequest("malformed")},
		{"MethodNotSupported", k8serrors.NewMethodNotSupported(gr, "PATCH")},
		{"Gone", k8serrors.NewGone("expired")},
	}
	for _, tc := range nonTransient {
		t.Run(tc.name, func(t *testing.T) {
			assert.True(t, isNonTransientK8sError(tc.err), "expected %s to be non-transient", tc.name)
		})
	}

	transient := []struct {
		name string
		err  error
	}{
		{"ServerTimeout", k8serrors.NewServerTimeout(gr, "get", 5)},
		{"TooManyRequests", k8serrors.NewTooManyRequests("slow down", 1)},
		{"ServiceUnavailable", k8serrors.NewServiceUnavailable("down")},
		{"InternalError", k8serrors.NewInternalError(errors.New("oops"))},
		{"NotFound", k8serrors.NewNotFound(gr, "s")},
		{"NetworkError", errors.New("connection refused")},
	}
	for _, tc := range transient {
		t.Run(tc.name, func(t *testing.T) {
			assert.False(t, isNonTransientK8sError(tc.err), "expected %s to be transient", tc.name)
		})
	}
}
