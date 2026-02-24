// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package appsec

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

// mockInjectionPattern implements the InjectionPattern interface for testing
type mockInjectionPattern struct {
	resource             schema.GroupVersionResource
	namespace            string
	injectionPossibleErr error
	addedErr             error
	deletedErr           error
	addedCalls           int
	deletedCalls         int
}

func (m *mockInjectionPattern) Mode() appsecconfig.InjectionMode {
	return appsecconfig.InjectionModeExternal
}

func (m *mockInjectionPattern) IsInjectionPossible(context.Context) error {
	return m.injectionPossibleErr
}

func (m *mockInjectionPattern) Resource() schema.GroupVersionResource {
	return m.resource
}

func (m *mockInjectionPattern) Namespace() string {
	return m.namespace
}

func (m *mockInjectionPattern) Added(context.Context, *unstructured.Unstructured) error {
	m.addedCalls++
	return m.addedErr
}

func (m *mockInjectionPattern) Deleted(context.Context, *unstructured.Unstructured) error {
	m.deletedCalls++
	return m.deletedErr
}

func newMockSecurityInjector(_ context.Context, mockClient dynamic.Interface, mockLogger log.Component, mockConfig appsecconfig.Config) *securityInjector {
	recorder := record.NewFakeRecorder(100)

	if mockConfig.BaseBackoff == 0 {
		mockConfig.BaseBackoff = 100 * time.Millisecond
	}

	if mockConfig.MaxBackoff == 0 {
		mockConfig.MaxBackoff = 1 * time.Second
	}

	return &securityInjector{
		k8sClient: mockClient,
		logger:    mockLogger,
		config:    mockConfig,
		recorder:  recorder,
		leaderSub: func() (<-chan struct{}, func() bool) {
			ch := make(chan struct{})
			return ch, func() bool { return true }
		},
	}
}

func TestProcessWorkItem_Added(t *testing.T) {
	mockLogger := logmock.New(t)
	mockClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	mockConfig := appsecconfig.Config{
		Injection: appsecconfig.Injection{
			Enabled: true,
		},
		Product: appsecconfig.Product{
			Enabled: true,
			Proxies: map[appsecconfig.ProxyType]struct{}{
				appsecconfig.ProxyTypeEnvoyGateway: {},
			},
		},
	}

	ctx := context.Background()
	injector := newMockSecurityInjector(ctx, mockClient, mockLogger, mockConfig)

	mockPattern := &mockInjectionPattern{
		resource:  schema.GroupVersionResource{Group: "test", Version: "v1", Resource: "testresources"},
		namespace: "default",
	}

	queue := workqueue.NewTypedRateLimitingQueue[workItem](
		workqueue.NewTypedItemExponentialFailureRateLimiter[workItem](
			100*time.Millisecond,
			1*time.Second,
		),
	)
	defer queue.ShutDown()

	testObj := &unstructured.Unstructured{}
	testObj.SetName("test-resource")
	testObj.SetNamespace("default")
	testObj.SetGroupVersionKind(mockPattern.resource.GroupVersion().WithKind("TestResource"))

	item := workItem{
		obj: testObj,
		typ: workItemAdded,
	}

	queue.Add(item)

	// Process the item
	quit := injector.processWorkItem(ctx, appsecconfig.ProxyTypeEnvoyGateway, mockPattern, queue)

	assert.False(t, quit, "processWorkItem should not return quit on successful processing")
	assert.Equal(t, 1, mockPattern.addedCalls, "Added should be called once")
	assert.Equal(t, 0, mockPattern.deletedCalls, "Deleted should not be called")
	assert.Equal(t, 0, queue.Len(), "Queue should be empty after processing")
}

func TestProcessWorkItem_Deleted(t *testing.T) {
	mockLogger := logmock.New(t)
	mockClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	mockConfig := appsecconfig.Config{
		Injection: appsecconfig.Injection{
			Enabled: true,
		},
		Product: appsecconfig.Product{
			Enabled: true,
			Proxies: map[appsecconfig.ProxyType]struct{}{
				appsecconfig.ProxyTypeEnvoyGateway: {},
			},
		},
	}

	ctx := context.Background()
	injector := newMockSecurityInjector(ctx, mockClient, mockLogger, mockConfig)

	mockPattern := &mockInjectionPattern{
		resource:  schema.GroupVersionResource{Group: "test", Version: "v1", Resource: "testresources"},
		namespace: "default",
	}

	queue := workqueue.NewTypedRateLimitingQueue[workItem](
		workqueue.NewTypedItemExponentialFailureRateLimiter[workItem](
			100*time.Millisecond,
			1*time.Second,
		),
	)
	defer queue.ShutDown()

	testObj := &unstructured.Unstructured{}
	testObj.SetName("test-resource")
	testObj.SetNamespace("default")
	testObj.SetGroupVersionKind(mockPattern.resource.GroupVersion().WithKind("TestResource"))

	item := workItem{
		obj: testObj,
		typ: workItemDeleted,
	}

	queue.Add(item)

	// Process the item
	quit := injector.processWorkItem(ctx, appsecconfig.ProxyTypeEnvoyGateway, mockPattern, queue)

	assert.False(t, quit, "processWorkItem should not return quit on successful processing")
	assert.Equal(t, 0, mockPattern.addedCalls, "Added should not be called")
	assert.Equal(t, 1, mockPattern.deletedCalls, "Deleted should be called once")
	assert.Equal(t, 0, queue.Len(), "Queue should be empty after processing")
}

func TestProcessWorkItem_ErrorRetry(t *testing.T) {
	mockLogger := logmock.New(t)
	mockClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	mockConfig := appsecconfig.Config{
		Injection: appsecconfig.Injection{
			Enabled: true,
		},
		Product: appsecconfig.Product{
			Enabled: true,
			Proxies: map[appsecconfig.ProxyType]struct{}{
				appsecconfig.ProxyTypeEnvoyGateway: {},
			},
		},
	}

	ctx := context.Background()
	injector := newMockSecurityInjector(ctx, mockClient, mockLogger, mockConfig)

	mockPattern := &mockInjectionPattern{
		resource:  schema.GroupVersionResource{Group: "test", Version: "v1", Resource: "testresources"},
		namespace: "default",
		addedErr:  errors.New("test error"),
	}

	queue := workqueue.NewTypedRateLimitingQueue[workItem](
		workqueue.NewTypedItemExponentialFailureRateLimiter[workItem](
			100*time.Millisecond,
			1*time.Second,
		),
	)
	defer queue.ShutDown()

	testObj := &unstructured.Unstructured{}
	testObj.SetName("test-resource")
	testObj.SetNamespace("default")
	testObj.SetGroupVersionKind(mockPattern.resource.GroupVersion().WithKind("TestResource"))

	item := workItem{
		obj: testObj,
		typ: workItemAdded,
	}

	queue.Add(item)

	// Process the item - should fail and requeue
	quit := injector.processWorkItem(ctx, appsecconfig.ProxyTypeEnvoyGateway, mockPattern, queue)

	assert.False(t, quit, "processWorkItem should not return quit on error")
	assert.Equal(t, 1, mockPattern.addedCalls, "Added should be called once")
	// Note: We check NumRequeues instead of Len because AddRateLimited uses rate limiting
	// and the item may not be immediately available in the queue
	assert.Equal(t, 1, queue.NumRequeues(item), "Item should have 1 requeue")
}

func TestProcessWorkItem_NotLeader(t *testing.T) {
	mockLogger := logmock.New(t)
	mockClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	mockConfig := appsecconfig.Config{
		Injection: appsecconfig.Injection{
			Enabled: true,
		},
		Product: appsecconfig.Product{
			Enabled: true,
			Proxies: map[appsecconfig.ProxyType]struct{}{
				appsecconfig.ProxyTypeEnvoyGateway: {},
			},
		},
	}

	ctx := context.Background()
	injector := newMockSecurityInjector(ctx, mockClient, mockLogger, mockConfig)
	// Note: leaderSub is not used by processWorkItem directly - leadership checks happen in runLeader
	injector.leaderSub = func() (<-chan struct{}, func() bool) {
		ch := make(chan struct{})
		return ch, func() bool { return false }
	}

	mockPattern := &mockInjectionPattern{
		resource:  schema.GroupVersionResource{Group: "test", Version: "v1", Resource: "testresources"},
		namespace: "default",
	}

	queue := workqueue.NewTypedRateLimitingQueue[workItem](
		workqueue.NewTypedItemExponentialFailureRateLimiter[workItem](
			100*time.Millisecond,
			1*time.Second,
		),
	)
	defer queue.ShutDown()

	testObj := &unstructured.Unstructured{}
	testObj.SetName("test-resource")
	testObj.SetNamespace("default")
	testObj.SetGroupVersionKind(mockPattern.resource.GroupVersion().WithKind("TestResource"))

	item := workItem{
		obj: testObj,
		typ: workItemAdded,
	}

	queue.Add(item)

	// Process the item - processWorkItem doesn't check leadership, it just processes items from the queue
	// Leadership checking happens in the runLeader loop
	quit := injector.processWorkItem(ctx, appsecconfig.ProxyTypeEnvoyGateway, mockPattern, queue)

	assert.False(t, quit, "processWorkItem should not quit for normal items")
	assert.Equal(t, 1, mockPattern.addedCalls, "Added should be called once")
	assert.Equal(t, 0, queue.Len(), "Queue should be empty after processing")
}

func TestCompilePatterns(t *testing.T) {
	mockLogger := logmock.New(t)
	mockClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	mockConfig := appsecconfig.Config{
		Injection: appsecconfig.Injection{
			Enabled: true,
			CommonAnnotations: map[string]string{
				"test": "annotation",
			},
		},
		Product: appsecconfig.Product{
			Enabled: true,
			Proxies: map[appsecconfig.ProxyType]struct{}{
				appsecconfig.ProxyTypeEnvoyGateway: {},
			},
			Processor: appsecconfig.Processor{
				ServiceName: "test-service",
				Namespace:   "test-namespace",
				Port:        443,
			},
		},
	}

	patterns := instantiatePatterns(mockConfig, mockLogger, mockClient, record.NewFakeRecorder(100))

	require.Len(t, patterns, len(appsecconfig.AllProxyTypes), "Should have one pattern for envoy-gateway")
	assert.Contains(t, patterns, appsecconfig.ProxyTypeEnvoyGateway, "Should have envoy-gateway pattern")

	// Verify the pattern is configured correctly
	pattern := patterns[appsecconfig.ProxyTypeEnvoyGateway]
	assert.NotNil(t, pattern, "Pattern should not be nil")
}

// Note: TestNewSecurityInjector tests are skipped as they require full APIClient mock
// which is complex to set up. These would be better as integration tests.

func TestWorkItemType_String(t *testing.T) {
	tests := []struct {
		name     string
		itemType workItemType
		expected string
	}{
		{
			name:     "added",
			itemType: workItemAdded,
			expected: "added",
		},
		{
			name:     "deleted",
			itemType: workItemDeleted,
			expected: "deleted",
		},
		{
			name:     "unknown",
			itemType: workItemType(99),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.itemType.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}
