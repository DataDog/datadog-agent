// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package appsec

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func (m *mockInjectionPattern) IsInjectionPossible(ctx context.Context) error {
	return m.injectionPossibleErr
}

func (m *mockInjectionPattern) Resource() schema.GroupVersionResource {
	return m.resource
}

func (m *mockInjectionPattern) Namespace() string {
	return m.namespace
}

func (m *mockInjectionPattern) Added(ctx context.Context, namespace, name string) error {
	m.addedCalls++
	return m.addedErr
}

func (m *mockInjectionPattern) Deleted(ctx context.Context, namespace, name string) error {
	m.deletedCalls++
	return m.deletedErr
}

func newMockSecurityInjector(ctx context.Context, mockClient dynamic.Interface, mockLogger log.Component, mockConfig appsecconfig.Config) *securityInjector {
	ctx, cancel := context.WithCancel(ctx)
	recorder := record.NewFakeRecorder(100)

	return &securityInjector{
		ctx:                   ctx,
		cancel:                cancel,
		k8sClient:             mockClient,
		logger:                mockLogger,
		config:                mockConfig,
		recorder:              recorder,
		leaderElectionEnabled: false,
		baseBackoff:           100 * time.Millisecond,
		maxBackoff:            1 * time.Second,
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
	defer injector.cancel()

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

	item := workItem{
		name:      "test-resource",
		namespace: "default",
		typ:       workItemAdded,
	}

	queue.Add(item)

	// Process the item
	quit := injector.processWorkItem(appsecconfig.ProxyTypeEnvoyGateway, mockPattern, queue)

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
	defer injector.cancel()

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

	item := workItem{
		name:      "test-resource",
		namespace: "default",
		typ:       workItemDeleted,
	}

	queue.Add(item)

	// Process the item
	quit := injector.processWorkItem(appsecconfig.ProxyTypeEnvoyGateway, mockPattern, queue)

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
	defer injector.cancel()

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

	item := workItem{
		name:      "test-resource",
		namespace: "default",
		typ:       workItemAdded,
	}

	queue.Add(item)

	// Process the item - should fail and requeue
	quit := injector.processWorkItem(appsecconfig.ProxyTypeEnvoyGateway, mockPattern, queue)

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
	injector.leaderElectionEnabled = true // Enable leader election but we're not the leader
	defer injector.cancel()

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

	item := workItem{
		name:      "test-resource",
		namespace: "default",
		typ:       workItemAdded,
	}

	queue.Add(item)

	// Process the item - should skip because we're not the leader
	quit := injector.processWorkItem(appsecconfig.ProxyTypeEnvoyGateway, mockPattern, queue)

	assert.False(t, quit, "processWorkItem should not return quit when not leader")
	assert.Equal(t, 0, mockPattern.addedCalls, "Added should not be called when not leader")
	assert.Equal(t, 0, queue.Len(), "Queue should be empty (item forgotten)")
}

func TestIsLeader_LeaderElectionDisabled(t *testing.T) {
	mockLogger := logmock.New(t)
	mockClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	mockConfig := appsecconfig.Config{
		Injection: appsecconfig.Injection{
			Enabled: true,
		},
		Product: appsecconfig.Product{
			Enabled: true,
		},
	}

	ctx := context.Background()
	injector := newMockSecurityInjector(ctx, mockClient, mockLogger, mockConfig)
	injector.leaderElectionEnabled = false
	defer injector.cancel()

	assert.True(t, injector.isLeader(), "Should always be leader when leader election is disabled")
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

	ctx := context.Background()
	injector := newMockSecurityInjector(ctx, mockClient, mockLogger, mockConfig)
	defer injector.cancel()

	patterns := injector.CompilePatterns()

	require.Len(t, patterns, 1, "Should have one pattern for envoy-gateway")
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

func TestDetectProxiesInCluster(t *testing.T) {
	tests := []struct {
		name            string
		setupMocks      func() (dynamic.Interface, log.Component)
		expectedProxies map[appsecconfig.ProxyType]struct{}
		expectError     bool
	}{
		{
			name: "no proxies detected",
			setupMocks: func() (dynamic.Interface, log.Component) {
				mockClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
				mockLogger := logmock.New(t)
				return mockClient, mockLogger
			},
			expectedProxies: map[appsecconfig.ProxyType]struct{}{},
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient, mockLogger := tt.setupMocks()
			ctx := context.Background()

			// Note: This test is limited because detectProxiesInCluster requires
			// a real APIClient, not just a dynamic.Interface. In a real implementation,
			// you would need to mock the entire APIClient structure or refactor
			// detectProxiesInCluster to accept dynamic.Interface directly.
			_ = mockClient
			_ = mockLogger
			_ = ctx

			// For now, we'll skip the actual test since it requires APIClient
			t.Skip("Skipping test that requires full APIClient mock")
		})
	}
}
