// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package appsec

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	admcommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestNewWebhook(t *testing.T) {
	tests := []struct {
		name           string
		config         map[string]any
		expectDisabled bool
		expectEnabled  bool
		expectEndpoint string
	}{
		{
			name: "webhook nil when mode is external",
			config: map[string]any{
				"cluster_agent.appsec.injector.mode": "external",
			},
			expectDisabled: true,
		},
		{
			name: "webhook nil when mode is not sidecar",
			config: map[string]any{
				"cluster_agent.appsec.injector.mode": "invalid",
			},
			expectDisabled: true,
		},
		{
			name: "webhook created for sidecar mode",
			config: map[string]any{
				"cluster_agent.appsec.injector.mode": "sidecar",
			},
			expectDisabled: false,
			expectEnabled:  false, // Enabled depends on patterns available
			expectEndpoint: "/appsec-proxies",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := common.FakeConfigWithValues(t, tt.config)
			webhook := NewWebhook(mockConfig)

			if tt.expectDisabled {
				assert.False(t, webhook.isEnabled)
			} else {
				require.NotNil(t, webhook)
				assert.Equal(t, tt.expectEndpoint, webhook.Endpoint())
				assert.Equal(t, webhookName, webhook.Name())
			}
		})
	}
}

func TestWebhookMatchConditions(t *testing.T) {
	// Create webhook with mock pattern
	pattern := &mockPattern{
		matchExpression:   "object.metadata.labels['gateway'] == 'istio'",
		shouldMutate:      true,
		namespaceEligible: true,
	}

	webhook := &Webhook{
		name:     webhookName,
		patterns: []appsecconfig.SidecarInjectionPattern{pattern},
	}

	conditions := webhook.MatchConditions()

	// Should return at least one condition
	require.NotEmpty(t, conditions)

	// The condition should have a name and expression
	assert.Equal(t, webhookName, conditions[0].Name)
	assert.NotEmpty(t, conditions[0].Expression)

	// The expression should be valid CEL (at minimum it should not be empty)
	t.Logf("Generated CEL expression: %s", conditions[0].Expression)
}

func TestWebhookMatchConditions_WithMockPatterns(t *testing.T) {
	pattern1 := &mockPattern{
		matchExpression:   "object.metadata.labels['app'] == 'gateway'",
		shouldMutate:      true,
		namespaceEligible: true,
	}

	webhook := &Webhook{
		name:     webhookName,
		patterns: []appsecconfig.SidecarInjectionPattern{pattern1},
	}

	conditions := webhook.MatchConditions()

	// Should return exactly one condition
	require.Len(t, conditions, 1)

	// The condition should have a name and expression
	assert.Equal(t, webhookName, conditions[0].Name)
	assert.NotEmpty(t, conditions[0].Expression)

	// Expression should be wrapped in parentheses
	assert.Contains(t, conditions[0].Expression, "(")
	assert.Contains(t, conditions[0].Expression, ")")
	assert.Contains(t, conditions[0].Expression, "object.metadata.labels['app'] == 'gateway'")

	t.Logf("Generated CEL expression: %s", conditions[0].Expression)
}

func TestWebhook_Properties(t *testing.T) {
	// Create webhook with mock pattern
	pattern := &mockPattern{
		matchExpression:   "object.metadata.labels['app'] == 'gateway'",
		shouldMutate:      true,
		namespaceEligible: true,
	}

	webhook := &Webhook{
		name:       webhookName,
		isEnabled:  true,
		endpoint:   "/appsec-proxies",
		resources:  map[string][]string{"": {"pods"}},
		operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Delete},
		patterns:   []appsecconfig.SidecarInjectionPattern{pattern},
	}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, webhookName, webhook.Name())
	})

	t.Run("WebhookType", func(t *testing.T) {
		assert.Equal(t, admcommon.WebhookType(admcommon.MutatingWebhook), webhook.WebhookType())
	})

	t.Run("Endpoint", func(t *testing.T) {
		assert.Equal(t, "/appsec-proxies", webhook.Endpoint())
	})

	t.Run("Resources", func(t *testing.T) {
		resources := webhook.Resources()
		require.NotNil(t, resources)
		assert.Contains(t, resources, "")
		assert.Contains(t, resources[""], "pods")
	})

	t.Run("Operations", func(t *testing.T) {
		ops := webhook.Operations()
		require.Len(t, ops, 2)
		assert.Contains(t, ops, admissionregistrationv1.Create)
		assert.Contains(t, ops, admissionregistrationv1.Delete)
	})

	t.Run("LabelSelectors", func(t *testing.T) {
		nsSelector, objSelector := webhook.LabelSelectors(true)
		assert.Nil(t, nsSelector, "Namespace selector should be nil")
		assert.Nil(t, objSelector, "Object selector should be nil")
	})

	t.Run("Timeout", func(t *testing.T) {
		assert.Equal(t, int32(0), webhook.Timeout())
	})
}

func TestWebhook_MatchConditions_MultiplePatterns(t *testing.T) {
	// Create webhook with multiple mock patterns
	pattern1 := &mockPattern{
		matchExpression:   "object.metadata.labels['gateway-type'] == 'istio'",
		shouldMutate:      true,
		namespaceEligible: true,
	}
	pattern2 := &mockPattern{
		matchExpression:   "object.metadata.labels['gateway-type'] == 'envoy'",
		shouldMutate:      true,
		namespaceEligible: true,
	}
	pattern3 := &mockPattern{
		matchExpression:   "object.metadata.labels['app'] == 'gateway'",
		shouldMutate:      true,
		namespaceEligible: true,
	}

	webhook := &Webhook{
		name:     webhookName,
		patterns: []appsecconfig.SidecarInjectionPattern{pattern1, pattern2, pattern3},
	}

	// If there are multiple patterns, the expression should use OR logic (||)
	conditions := webhook.MatchConditions()
	require.NotEmpty(t, conditions)

	expression := conditions[0].Expression

	// Count patterns
	numPatterns := len(webhook.patterns)
	// Should have (numPatterns - 1) || operators
	assert.Contains(t, expression, "||", "Multiple patterns should be OR-ed together")
	t.Logf("Expression with %d patterns: %s", numPatterns, expression)

	// Verify all pattern expressions are in the final expression
	assert.Contains(t, expression, pattern1.matchExpression)
	assert.Contains(t, expression, pattern2.matchExpression)
	assert.Contains(t, expression, pattern3.matchExpression)

	// Expression should be wrapped in parentheses
	assert.Contains(t, expression, "(", "Patterns should be wrapped in parentheses")
	assert.Contains(t, expression, ")", "Patterns should be wrapped in parentheses")
}

// mockPattern is a test implementation of SidecarInjectionPattern
type mockPattern struct {
	matchExpression     string
	shouldMutate        bool
	namespaceEligible   bool
	mutatePodFunc       func(*corev1.Pod, string, dynamic.Interface) (bool, error)
	podDeletedFunc      func(*corev1.Pod, string, dynamic.Interface) (bool, error)
	mutatePodCallCount  int
	podDeletedCallCount int
}

func (m *mockPattern) MatchCondition() admissionregistrationv1.MatchCondition {
	return admissionregistrationv1.MatchCondition{
		Expression: m.matchExpression,
	}
}

func (m *mockPattern) ShouldMutatePod(*corev1.Pod) bool {
	return m.shouldMutate
}

func (m *mockPattern) IsNamespaceEligible(string) bool {
	return m.namespaceEligible
}

func (m *mockPattern) MutatePod(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	m.mutatePodCallCount++
	if m.mutatePodFunc != nil {
		return m.mutatePodFunc(pod, ns, dc)
	}
	return true, nil
}

func (m *mockPattern) PodDeleted(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	m.podDeletedCallCount++
	if m.podDeletedFunc != nil {
		return m.podDeletedFunc(pod, ns, dc)
	}
	return true, nil
}

func (m *mockPattern) Added(context.Context, *unstructured.Unstructured) error {
	return nil
}

func (m *mockPattern) Deleted(context.Context, *unstructured.Unstructured) error {
	return nil
}

func (m *mockPattern) Mode() appsecconfig.InjectionMode {
	return appsecconfig.InjectionModeSidecar
}

func (m *mockPattern) IsInjectionPossible(context.Context) error {
	return nil
}

func (m *mockPattern) Resource() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
}

func (m *mockPattern) Namespace() string {
	return metav1.NamespaceAll
}

func newTestPod(name, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "nginx:latest",
				},
			},
		},
	}
}

func TestWebhook_callPattern_NamespaceFiltering(t *testing.T) {
	tests := []struct {
		name              string
		podNamespace      string
		namespaceEligible bool
		shouldMutate      bool
		expectCalled      bool
		expectMutated     bool
	}{
		{
			name:              "pattern called when namespace eligible and should mutate",
			podNamespace:      "default",
			namespaceEligible: true,
			shouldMutate:      true,
			expectCalled:      true,
			expectMutated:     true,
		},
		{
			name:              "pattern not called when namespace not eligible",
			podNamespace:      "kube-system",
			namespaceEligible: false,
			shouldMutate:      true,
			expectCalled:      false,
			expectMutated:     false,
		},
		{
			name:              "pattern not called when should not mutate",
			podNamespace:      "default",
			namespaceEligible: true,
			shouldMutate:      false,
			expectCalled:      false,
			expectMutated:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPattern := &mockPattern{
				matchExpression:   "true",
				shouldMutate:      tt.shouldMutate,
				namespaceEligible: tt.namespaceEligible,
			}

			webhook := &Webhook{
				patterns: []appsecconfig.SidecarInjectionPattern{mockPattern},
			}

			pod := newTestPod("test-pod", tt.podNamespace)
			scheme := runtime.NewScheme()
			client := dynamicfake.NewSimpleDynamicClient(scheme)

			mutated, err := webhook.callPattern(pod, tt.podNamespace, client, appsecconfig.SidecarInjectionPattern.MutatePod)

			require.NoError(t, err)
			assert.Equal(t, tt.expectMutated, mutated)
			if tt.expectCalled {
				assert.Equal(t, 1, mockPattern.mutatePodCallCount, "Pattern should be called")
			} else {
				assert.Equal(t, 0, mockPattern.mutatePodCallCount, "Pattern should not be called")
			}
		})
	}
}

func TestWebhook_callPattern_MultiplePatterns(t *testing.T) {
	// First pattern doesn't match, second one does
	pattern1 := &mockPattern{
		matchExpression:   "pattern1",
		shouldMutate:      false,
		namespaceEligible: true,
	}
	pattern2 := &mockPattern{
		matchExpression:   "pattern2",
		shouldMutate:      true,
		namespaceEligible: true,
	}

	webhook := &Webhook{
		patterns: []appsecconfig.SidecarInjectionPattern{pattern1, pattern2},
	}

	pod := newTestPod("test-pod", "default")
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	mutated, err := webhook.callPattern(pod, "default", client, appsecconfig.SidecarInjectionPattern.MutatePod)

	require.NoError(t, err)
	assert.True(t, mutated)
	assert.Equal(t, 0, pattern1.mutatePodCallCount, "First pattern should not be called")
	assert.Equal(t, 1, pattern2.mutatePodCallCount, "Second pattern should be called")
}

func TestWebhook_callPattern_CallbackReceivesNamespace(t *testing.T) {
	var capturedNamespace string
	mockPattern := &mockPattern{
		matchExpression:   "true",
		shouldMutate:      true,
		namespaceEligible: true,
		mutatePodFunc: func(_ *corev1.Pod, ns string, _ dynamic.Interface) (bool, error) {
			capturedNamespace = ns
			return true, nil
		},
	}

	webhook := &Webhook{
		patterns: []appsecconfig.SidecarInjectionPattern{mockPattern},
	}

	pod := newTestPod("test-pod", "default")
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	mutated, err := webhook.callPattern(pod, "custom-namespace", client, appsecconfig.SidecarInjectionPattern.MutatePod)

	require.NoError(t, err)
	assert.True(t, mutated)
	assert.Equal(t, "custom-namespace", capturedNamespace, "Callback should receive correct namespace")
}

func TestWebhook_WebhookFunc_CreateOperation(t *testing.T) {
	mockPattern := &mockPattern{
		matchExpression:   "true",
		shouldMutate:      true,
		namespaceEligible: true,
		mutatePodFunc: func(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
			// Add a label to verify mutation happened
			if pod.Labels == nil {
				pod.Labels = make(map[string]string)
			}
			pod.Labels["mutated"] = "true"
			return true, nil
		},
	}

	webhook := &Webhook{
		name:          webhookName,
		patterns:      []appsecconfig.SidecarInjectionPattern{mockPattern},
		configMutator: &mockMutator{},
	}

	pod := newTestPod("test-pod", "default")
	podBytes, err := json.Marshal(pod)
	require.NoError(t, err)

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	request := &admission.Request{
		Operation:     admissionregistrationv1.Create,
		Namespace:     "default",
		Object:        podBytes,
		DynamicClient: client,
	}

	webhookFunc := webhook.WebhookFunc()
	response := webhookFunc(request)

	require.NotNil(t, response)
	assert.True(t, response.Allowed, "Request should be allowed")
	assert.NotNil(t, response.Patch, "Should have patch")
	assert.Equal(t, 1, mockPattern.mutatePodCallCount, "Pattern MutatePod should be called")
}

func TestWebhook_WebhookFunc_DeleteOperation(t *testing.T) {
	mockPattern := &mockPattern{
		matchExpression:   "true",
		shouldMutate:      true,
		namespaceEligible: true,
	}

	webhook := &Webhook{
		name:     webhookName,
		patterns: []appsecconfig.SidecarInjectionPattern{mockPattern},
	}

	pod := newTestPod("test-pod", "default")
	podBytes, err := json.Marshal(pod)
	require.NoError(t, err)

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	request := &admission.Request{
		Operation:     admissionregistrationv1.Delete,
		Namespace:     "default",
		OldObject:     podBytes,
		DynamicClient: client,
	}

	webhookFunc := webhook.WebhookFunc()
	response := webhookFunc(request)

	require.NotNil(t, response)
	assert.True(t, response.Allowed, "Request should be allowed")
	assert.Equal(t, 1, mockPattern.podDeletedCallCount, "Pattern PodDeleted should be called")
	assert.NotNil(t, response.Patch, "Should have empty patch")
}

func TestWebhook_WebhookFunc_InvalidJSON(t *testing.T) {
	webhook := &Webhook{
		name:     webhookName,
		patterns: []appsecconfig.SidecarInjectionPattern{},
	}

	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	request := &admission.Request{
		Operation:     admissionregistrationv1.Delete,
		Namespace:     "default",
		OldObject:     []byte("invalid json"),
		DynamicClient: client,
	}

	webhookFunc := webhook.WebhookFunc()
	response := webhookFunc(request)

	require.NotNil(t, response)
	// Note: MutationResponse returns Allowed: true even on error to not block pods
	assert.True(t, response.Allowed, "Request is allowed but carries error message")
	assert.NotNil(t, response.Result)
	assert.Contains(t, response.Result.Message, "failed to decode raw object")
}

// mockMutator is a test implementation of Mutator
type mockMutator struct {
	mutatePodFunc      func(*corev1.Pod, string, dynamic.Interface) (bool, error)
	mutatePodCallCount int
}

func (m *mockMutator) MutatePod(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	m.mutatePodCallCount++
	if m.mutatePodFunc != nil {
		return m.mutatePodFunc(pod, ns, dc)
	}
	return true, nil
}
