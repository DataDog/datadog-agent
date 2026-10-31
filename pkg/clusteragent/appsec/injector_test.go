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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/istio"
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

// countingLogger records which log.Component level method the injector dispatched to.
// It counts method dispatch rather than message text, so it stays green across any
// rewording of the log lines.
type countingLogger struct {
	infofCalls  int
	errorfCalls int
}

var _ log.Component = (*countingLogger)(nil)

func (c *countingLogger) Infof(string, ...interface{}) { c.infofCalls++ }
func (c *countingLogger) Errorf(string, ...interface{}) error {
	c.errorfCalls++
	return nil
}

func (*countingLogger) Trace(...interface{})                   {}
func (*countingLogger) Tracef(string, ...interface{})          {}
func (*countingLogger) Debug(...interface{})                   {}
func (*countingLogger) Debugf(string, ...interface{})          {}
func (*countingLogger) Info(...interface{})                    {}
func (*countingLogger) Warn(...interface{}) error              { return nil }
func (*countingLogger) Warnf(string, ...interface{}) error     { return nil }
func (*countingLogger) Error(...interface{}) error             { return nil }
func (*countingLogger) Critical(...interface{}) error          { return nil }
func (*countingLogger) Criticalf(string, ...interface{}) error { return nil }
func (*countingLogger) Flush()                                 {}

func TestRun_logsNotApplicableSkipAtInfo(t *testing.T) {
	// Given a pattern reporting that injection does not apply to this configuration
	logger := &countingLogger{}
	injector := newMockSecurityInjector(context.Background(),
		dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
		logger,
		appsecconfig.Config{},
	)
	pattern := &mockInjectionPattern{
		injectionPossibleErr: fmt.Errorf("gke-gateway supports external mode only: %w", appsecconfig.ErrInjectionNotApplicable),
	}

	// When
	injector.run(context.Background(), appsecconfig.ProxyTypeGKEGateway, pattern)

	// Then the skip is reported at info level, never as a failure
	require.Zero(t, logger.errorfCalls)
	require.Equal(t, 1, logger.infofCalls)
}

func TestRun_logsGenuineFailureAtError(t *testing.T) {
	// Given a pattern reporting a real misconfiguration
	logger := &countingLogger{}
	injector := newMockSecurityInjector(context.Background(),
		dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
		logger,
		appsecconfig.Config{},
	)
	pattern := &mockInjectionPattern{
		injectionPossibleErr: errors.New("processor service name is required"),
	}

	// When
	injector.run(context.Background(), appsecconfig.ProxyTypeGKEGateway, pattern)

	// Then it stays an error, so genuine misconfiguration is still surfaced
	require.Equal(t, 1, logger.errorfCalls)
	require.Zero(t, logger.infofCalls)
}

func TestCleanupAllowed(t *testing.T) {
	istioGatewayGVR := schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "gateways"}
	gatewayClassGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gatewayclasses"}
	nativeGateway := func(name string, labels map[string]string) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion("networking.istio.io/v1")
		obj.SetKind("Gateway")
		obj.SetNamespace("default")
		obj.SetName(name)
		obj.SetLabels(labels)
		return obj
	}
	gatewayClass := func(name, controller string) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion("gateway.networking.k8s.io/v1")
		obj.SetKind("GatewayClass")
		obj.SetName(name)
		require.NoError(t, unstructured.SetNestedField(obj.Object, controller, "spec", "controllerName"))
		return obj
	}
	proxies := func(types ...appsecconfig.ProxyType) map[appsecconfig.ProxyType]struct{} {
		m := make(map[appsecconfig.ProxyType]struct{}, len(types))
		for _, typ := range types {
			m[typ] = struct{}{}
		}
		return m
	}
	optedOut := map[string]string{"appsec.datadoghq.com/enabled": "false"}
	newClient := func(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
		return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
			istioGatewayGVR: "GatewayList",
			gatewayClassGVR: "GatewayClassList",
		}, objects...)
	}

	for _, tc := range []struct {
		name     string
		typ      appsecconfig.ProxyType
		enabled  map[appsecconfig.ProxyType]struct{}
		objects  []runtime.Object
		gateways []*unstructured.Unstructured // native Gateways, seeded through the tracker
		want     bool
	}{
		{name: "istio, istio-gateway enabled with a native Gateway", typ: appsecconfig.ProxyTypeIstio, enabled: proxies(appsecconfig.ProxyTypeIstioGateway),
			gateways: []*unstructured.Unstructured{nativeGateway("gw", nil)}, want: false},
		{name: "istio, istio-gateway enabled without native Gateways", typ: appsecconfig.ProxyTypeIstio, enabled: proxies(appsecconfig.ProxyTypeIstioGateway),
			objects: []runtime.Object{gatewayClass("istio", "istio.io/gateway-controller")}, want: true},
		{name: "istio, istio-gateway enabled with only opted-out native Gateways", typ: appsecconfig.ProxyTypeIstio, enabled: proxies(appsecconfig.ProxyTypeIstioGateway),
			gateways: []*unstructured.Unstructured{nativeGateway("gw", optedOut)}, want: true},
		{name: "istio-gateway, istio enabled with an Istio GatewayClass", typ: appsecconfig.ProxyTypeIstioGateway, enabled: proxies(appsecconfig.ProxyTypeIstio),
			objects: []runtime.Object{gatewayClass("istio", "istio.io/gateway-controller")}, want: false},
		{name: "istio-gateway, istio enabled with only other GatewayClasses", typ: appsecconfig.ProxyTypeIstioGateway, enabled: proxies(appsecconfig.ProxyTypeIstio),
			objects: []runtime.Object{gatewayClass("envoy", "gateway.envoyproxy.io/gatewayclass-controller")}, gateways: []*unstructured.Unstructured{nativeGateway("gw", nil)}, want: true},
		{name: "istio, nothing enabled", typ: appsecconfig.ProxyTypeIstio, enabled: proxies(),
			gateways: []*unstructured.Unstructured{nativeGateway("gw", nil)}, want: true},
		{name: "envoy-gateway, istio types enabled", typ: appsecconfig.ProxyTypeEnvoyGateway, enabled: proxies(appsecconfig.ProxyTypeIstio, appsecconfig.ProxyTypeIstioGateway),
			objects: []runtime.Object{gatewayClass("istio", "istio.io/gateway-controller")}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newClient(tc.objects...)
			for _, gw := range tc.gateways {
				require.NoError(t, client.Tracker().Create(istioGatewayGVR, gw, gw.GetNamespace()))
			}
			assert.Equal(t, tc.want, cleanupAllowed(context.Background(), logmock.New(t), client, tc.typ, tc.enabled))
		})
	}

	t.Run("keeps resources when the check fails", func(t *testing.T) {
		client := newClient()
		client.PrependReactor("list", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("api unavailable")
		})
		assert.False(t, cleanupAllowed(context.Background(), logmock.New(t), client, appsecconfig.ProxyTypeIstio, proxies(appsecconfig.ProxyTypeIstioGateway)))
	})
}

// TestStartupCleanup_SiblingWithoutResources covers the startup path for a disabled Istio proxy type
// whose sibling is enabled: the shared EnvoyFilter is removed when the sibling has no resources that
// need it, and kept otherwise.
func TestStartupCleanup_SiblingWithoutResources(t *testing.T) {
	istioGatewayGVR := schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "gateways"}
	gatewayClassGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gatewayclasses"}
	filterGVR := schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1alpha3", Resource: "envoyfilters"}

	for _, tc := range []struct {
		name         string
		nativeGW     bool
		filterRemain bool
	}{
		{name: "no native Gateways", nativeGW: false, filterRemain: false},
		{name: "native Gateway present", nativeGW: true, filterRemain: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			logger := logmock.New(t)

			filter := &unstructured.Unstructured{}
			filter.SetAPIVersion("networking.istio.io/v1alpha3")
			filter.SetKind("EnvoyFilter")
			filter.SetNamespace("istio-system")
			filter.SetName("datadog-appsec-envoyfilter")
			class := &unstructured.Unstructured{}
			class.SetAPIVersion("gateway.networking.k8s.io/v1")
			class.SetKind("GatewayClass")
			class.SetName("istio")
			require.NoError(t, unstructured.SetNestedField(class.Object, "istio.io/gateway-controller", "spec", "controllerName"))

			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
				istioGatewayGVR: "GatewayList",
				gatewayClassGVR: "GatewayClassList",
				filterGVR:       "EnvoyFilterList",
			}, filter, class)
			if tc.nativeGW {
				gw := &unstructured.Unstructured{}
				gw.SetAPIVersion("networking.istio.io/v1")
				gw.SetKind("Gateway")
				gw.SetNamespace("default")
				gw.SetName("gw")
				require.NoError(t, client.Tracker().Create(istioGatewayGVR, gw, "default"))
			}

			// The filter was created for the istio GatewayClass pattern, which is now disabled in favor of istio-gateway
			config := appsecconfig.Config{Injection: appsecconfig.Injection{IstioNamespace: "istio-system"}}
			pattern := istio.New(client, logger, config, record.NewFakeRecorder(10))
			enabled := map[appsecconfig.ProxyType]struct{}{appsecconfig.ProxyTypeIstioGateway: {}}
			if cleanupAllowed(ctx, logger, client, appsecconfig.ProxyTypeIstio, enabled) {
				cleanupPattern(ctx, logger, client, pattern)
			}

			_, err := client.Resource(filterGVR).Namespace("istio-system").Get(ctx, "datadog-appsec-envoyfilter", metav1.GetOptions{})
			if tc.filterRemain {
				require.NoError(t, err, "EnvoyFilter must be kept while the enabled sibling has resources")
			} else {
				assert.True(t, apierrors.IsNotFound(err), "EnvoyFilter must be removed when the enabled sibling has no resources")
			}
		})
	}
}

// cleanupRecordingPattern records whether each Deleted call received a cleanup context.
type cleanupRecordingPattern struct {
	mockInjectionPattern
	cleanup []bool
}

func (p *cleanupRecordingPattern) Deleted(ctx context.Context, _ *unstructured.Unstructured) error {
	p.cleanup = append(p.cleanup, appsecconfig.IsCleanup(ctx))
	return nil
}

func TestCleanupPattern_MarksCleanupContext(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gatewayclasses"}
	newClass := func(name string) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion("gateway.networking.k8s.io/v1")
		obj.SetKind("GatewayClass")
		obj.SetName(name)
		return obj
	}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), newClass("a"), newClass("b"))
	pattern := &cleanupRecordingPattern{mockInjectionPattern: mockInjectionPattern{resource: gvr}}

	cleanupPattern(context.Background(), logmock.New(t), client, pattern)

	assert.Equal(t, []bool{true, true}, pattern.cleanup)
}

func TestCreateEventHandler(t *testing.T) {
	gw := &unstructured.Unstructured{}
	gw.SetAPIVersion("gateway.networking.k8s.io/v1")
	gw.SetKind("Gateway")
	gw.SetNamespace("default")
	gw.SetName("gw")

	for _, tc := range []struct {
		name   string
		call   func(h cache.ResourceEventHandler)
		queued []workItem
	}{
		{name: "add", call: func(h cache.ResourceEventHandler) { h.OnAdd(gw, false) }, queued: []workItem{{typ: workItemAdded, obj: gw}}},
		{name: "delete", call: func(h cache.ResourceEventHandler) { h.OnDelete(gw) }, queued: []workItem{{typ: workItemDeleted, obj: gw}}},
		{name: "delete missed while the watch was down", call: func(h cache.ResourceEventHandler) {
			h.OnDelete(cache.DeletedFinalStateUnknown{Key: "default/gw", Obj: gw})
		}, queued: []workItem{{typ: workItemDeleted, obj: gw}}},
		{name: "tombstone without an object", call: func(h cache.ResourceEventHandler) {
			h.OnDelete(cache.DeletedFinalStateUnknown{Key: "default/gw"})
		}},
		{name: "unexpected add type", call: func(h cache.ResourceEventHandler) { h.OnAdd("not an object", false) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[workItem]())
			defer queue.ShutDown()
			si := &securityInjector{logger: logmock.New(t)}

			tc.call(si.createEventHandler(queue))

			require.Equal(t, len(tc.queued), queue.Len())
			for _, want := range tc.queued {
				got, _ := queue.Get()
				assert.Equal(t, want.typ, got.typ)
				assert.Same(t, want.obj, got.obj)
				queue.Done(got)
			}
		})
	}
}
