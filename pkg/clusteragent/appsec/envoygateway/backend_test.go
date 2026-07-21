// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package envoygateway

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

const testSocketPath = "/var/run/datadog/extproc.sock"

func newTestPatternWithBackendSupport(t *testing.T) (*envoyGatewayInjectionPattern, *dynamicfake.FakeDynamicClient) {
	t.Helper()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			backendGVR: "BackendList",
		},
	)
	fakeRecorder := record.NewFakeRecorder(100)
	eventRec := eventRecorder{recorder: fakeRecorder}
	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
		Injection: appsecconfig.Injection{
			CommonLabels:      map[string]string{"app": "datadog"},
			CommonAnnotations: map[string]string{"managed-by": "datadog"},
		},
	}
	pattern := &envoyGatewayInjectionPattern{
		client:        client,
		logger:        logger,
		config:        config,
		eventRecorder: eventRec,
		grantManager: grantManager{
			client:            client,
			logger:            logger,
			eventRecorder:     eventRec,
			serviceName:       config.Processor.ServiceName,
			namespace:         config.Processor.Namespace,
			commonLabels:      config.CommonLabels,
			commonAnnotations: config.CommonAnnotations,
		},
	}
	return pattern, client
}

func TestCreateBackend_CreatesObjectWithExpectedFields(t *testing.T) {
	ctx := context.Background()
	pattern, client := newTestPatternWithBackendSupport(t)

	err := pattern.createBackend(ctx, "test-ns", testSocketPath)
	require.NoError(t, err)

	got, err := client.Resource(backendGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, "Backend", got.GetKind())
	assert.Equal(t, "gateway.envoyproxy.io/v1alpha1", got.GetAPIVersion())
	assert.Equal(t, extProcName, got.GetName())
	assert.Equal(t, "test-ns", got.GetNamespace())

	endpoints, found, err := unstructured.NestedSlice(got.Object, "spec", "endpoints")
	require.NoError(t, err)
	require.True(t, found, "spec.endpoints must be present")
	require.Len(t, endpoints, 1)

	ep, ok := endpoints[0].(map[string]any)
	require.True(t, ok)

	path, found, err := unstructured.NestedString(map[string]any{"unix": ep["unix"]}, "unix", "path")
	require.NoError(t, err)
	require.True(t, found, "spec.endpoints[0].unix.path must be present")
	assert.Equal(t, testSocketPath, path)
}

func TestCreateBackend_Idempotent(t *testing.T) {
	ctx := context.Background()
	pattern, client := newTestPatternWithBackendSupport(t)

	require.NoError(t, pattern.createBackend(ctx, "test-ns", testSocketPath))
	require.NoError(t, pattern.createBackend(ctx, "test-ns", testSocketPath), "second call must return nil")

	list, err := client.Resource(backendGVR).Namespace("test-ns").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, list.Items, 1, "exactly one backend must exist after two creates")
}

func TestCreateBackend_ReturnsNonAlreadyExistsError(t *testing.T) {
	ctx := context.Background()
	pattern, client := newTestPatternWithBackendSupport(t)
	boom := stderrors.New("create failed")
	client.PrependReactor("create", "backends", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, boom
	})

	err := pattern.createBackend(ctx, "test-ns", testSocketPath)
	require.ErrorIs(t, err, boom)
}

func TestDeleteBackend_AbsentIsNoOp(t *testing.T) {
	ctx := context.Background()
	pattern, _ := newTestPatternWithBackendSupport(t)

	err := pattern.deleteBackend(ctx, "test-ns")
	assert.NoError(t, err, "deleting a non-existent backend must return nil")
}

func TestDeleteBackend_ReturnsNonNotFoundError(t *testing.T) {
	ctx := context.Background()
	pattern, client := newTestPatternWithBackendSupport(t)
	boom := stderrors.New("delete failed")
	client.PrependReactor("delete", "backends", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, boom
	})

	err := pattern.deleteBackend(ctx, "test-ns")
	require.ErrorIs(t, err, boom)
}

func TestDeleteBackend_CreateThenDelete(t *testing.T) {
	ctx := context.Background()
	pattern, client := newTestPatternWithBackendSupport(t)

	require.NoError(t, pattern.createBackend(ctx, "test-ns", testSocketPath))
	require.NoError(t, pattern.deleteBackend(ctx, "test-ns"))

	_, err := client.Resource(backendGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.Error(t, err)
	assert.True(t, apierrors.IsNotFound(err), "Get after delete must return IsNotFound")
}
