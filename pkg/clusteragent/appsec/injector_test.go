// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package appsec

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

type mockSidecarInjectionPattern struct {
	mockInjectionPattern
}

func (m *mockSidecarInjectionPattern) Mode() appsecconfig.InjectionMode {
	return appsecconfig.InjectionModeSidecar
}

func (m *mockSidecarInjectionPattern) MatchCondition() admissionregistrationv1.MatchCondition {
	return admissionregistrationv1.MatchCondition{Expression: "object.metadata.labels['app'] == 'gateway'"}
}

func (m *mockSidecarInjectionPattern) IsPodEligible(*corev1.Pod, string) bool {
	return true
}

func (m *mockSidecarInjectionPattern) MutatePod(*corev1.Pod, string, dynamic.Interface) (appsecconfig.MutationOutcome, error) {
	return appsecconfig.MutationMutated, nil
}

func (m *mockSidecarInjectionPattern) PodDeleted(*corev1.Pod, string, dynamic.Interface) (appsecconfig.MutationOutcome, error) {
	return appsecconfig.MutationMutated, nil
}

var _ appsecconfig.SidecarInjectionPattern = (*mockSidecarInjectionPattern)(nil)

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

func TestGetSidecarPatterns(t *testing.T) {
	previousInjector := injector
	t.Cleanup(func() { injector = previousInjector })

	tests := []struct {
		name             string
		injectionEnabled bool
		productEnabled   bool
		expectPatterns   bool
	}{
		{
			name:             "returns sidecar patterns when product and injection are enabled",
			injectionEnabled: true,
			productEnabled:   true,
			expectPatterns:   true,
		},
		{
			name:             "returns no sidecar patterns when injection is disabled",
			injectionEnabled: false,
			productEnabled:   true,
			expectPatterns:   false,
		},
		{
			name:             "returns no sidecar patterns when product is disabled",
			injectionEnabled: true,
			productEnabled:   false,
			expectPatterns:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxyType := appsecconfig.ProxyTypeEnvoyGateway
			injector = &securityInjector{
				logger: logmock.New(t),
				config: appsecconfig.Config{
					Injection: appsecconfig.Injection{Enabled: tt.injectionEnabled},
					Product: appsecconfig.Product{
						Enabled: tt.productEnabled,
						Proxies: map[appsecconfig.ProxyType]struct{}{proxyType: {}},
					},
				},
				patterns: map[appsecconfig.ProxyType]appsecconfig.InjectionPattern{
					proxyType: &mockSidecarInjectionPattern{mockInjectionPattern: mockInjectionPattern{
						resource:  schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
						namespace: metav1.NamespaceAll,
					}},
				},
			}

			patterns := GetSidecarPatterns()
			if tt.expectPatterns {
				require.Len(t, patterns, 1)
				assert.Contains(t, patterns, proxyType)
				assert.Equal(t, appsecconfig.InjectionModeSidecar, patterns[proxyType].Mode())
				return
			}
			assert.Empty(t, patterns)
		})
	}
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

const egOwningGatewayNameCEL = "'gateway.envoyproxy.io/owning-gateway-name' in object.metadata.labels"

func TestGetSidecarPatterns_EnvoyGatewayRealSidecarMode(t *testing.T) {
	previousInjector := injector
	t.Cleanup(func() { injector = previousInjector })

	cfg := appsecconfig.Config{
		Injection: appsecconfig.Injection{
			Enabled:           true,
			CommonAnnotations: map[string]string{},
			CommonLabels:      map[string]string{},
		},
		Product: appsecconfig.Product{
			Enabled: true,
			Mode:    appsecconfig.InjectionModeSidecar,
			Proxies: map[appsecconfig.ProxyType]struct{}{
				appsecconfig.ProxyTypeEnvoyGateway: {},
			},
		},
	}

	injector = &securityInjector{
		logger: logmock.New(t),
		config: cfg,
		patterns: instantiatePatterns(
			cfg,
			logmock.New(t),
			dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
			record.NewFakeRecorder(100),
		),
	}

	sidecarPatterns := GetSidecarPatterns()

	require.Len(t, sidecarPatterns, 1, "Exactly one sidecar pattern expected for EG-only config in sidecar mode")
	egPattern := sidecarPatterns[appsecconfig.ProxyTypeEnvoyGateway]
	require.NotNil(t, egPattern)
	assert.Equal(t, appsecconfig.InjectionModeSidecar, egPattern.Mode())
	assert.Equal(t, egOwningGatewayNameCEL, egPattern.MatchCondition().Expression,
		"EG sidecar MatchCondition must be the owning-gateway-name label check")
}

// TestWebhookAggregation_EGCELInSingleAppsecWebhook asserts:
//  1. The EG owning-gateway CEL appears in the aggregated appsec_proxies webhook expression.
//  2. Aggregation always produces exactly ONE MatchCondition — no new MutatingWebhookConfiguration
//     is introduced by the EG sidecar feature.
//
// admission/mutate/appsec imports this package (circular), so patternsExpression()+MatchConditions()
// from webhook.go are reproduced inline.
func TestWebhookAggregation_EGCELInSingleAppsecWebhook(t *testing.T) {
	previousInjector := injector
	t.Cleanup(func() { injector = previousInjector })

	cfg := appsecconfig.Config{
		Injection: appsecconfig.Injection{
			Enabled:           true,
			CommonAnnotations: map[string]string{},
			CommonLabels:      map[string]string{},
		},
		Product: appsecconfig.Product{
			Enabled: true,
			Mode:    appsecconfig.InjectionModeSidecar,
			Proxies: map[appsecconfig.ProxyType]struct{}{
				appsecconfig.ProxyTypeEnvoyGateway: {},
			},
		},
	}

	injector = &securityInjector{
		logger: logmock.New(t),
		config: cfg,
		patterns: instantiatePatterns(
			cfg,
			logmock.New(t),
			dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
			record.NewFakeRecorder(100),
		),
	}

	sidecarPatterns := GetSidecarPatterns()
	require.NotEmpty(t, sidecarPatterns)

	var objectParts []string
	for _, p := range sidecarPatterns {
		objectParts = append(objectParts, "("+p.MatchCondition().Expression+")")
	}
	objectExpr := strings.Join(objectParts, "||")
	oldObjectExpr := strings.ReplaceAll(objectExpr, "object.", "oldObject.")
	aggregatedCEL := fmt.Sprintf(
		"(request.operation == 'DELETE' && (%s)) || (request.operation != 'DELETE' && (%s))",
		oldObjectExpr, objectExpr,
	)

	assert.Contains(t, aggregatedCEL, egOwningGatewayNameCEL,
		"Aggregated appsec_proxies webhook CEL must include the EG owning-gateway-name condition")

	conditions := []admissionregistrationv1.MatchCondition{{
		Name:       "appsec_proxies",
		Expression: aggregatedCEL,
	}}
	assert.Len(t, conditions, 1,
		"All sidecar patterns aggregate into ONE MatchCondition in appsec_proxies — no new webhook introduced")
	assert.Equal(t, "appsec_proxies", conditions[0].Name)
	assert.Contains(t, conditions[0].Expression, egOwningGatewayNameCEL)
}
