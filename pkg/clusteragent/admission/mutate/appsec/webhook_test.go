// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package appsec

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	admcommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	dto "github.com/prometheus/client_model/go"
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
		matchExpression: "object.metadata.labels['gateway'] == 'istio'",
		podEligible:     true,
	}

	webhook := &Webhook{
		name:     webhookName,
		patterns: newTestPatternMap(pattern),
	}

	conditions := webhook.MatchConditions()

	// Should return at least one condition
	require.NotEmpty(t, conditions)

	// The condition should have a name and expression
	assert.Equal(t, webhookName, conditions[0].Name)
	assert.NotEmpty(t, conditions[0].Expression)

	// The expression should be valid CEL (at minimum it should not be empty)
	assert.Contains(t, conditions[0].Expression, "request.operation == 'DELETE'")
	assert.Contains(t, conditions[0].Expression, "oldObject.metadata.labels['gateway'] == 'istio'")
	assert.Contains(t, conditions[0].Expression, "request.operation != 'DELETE'")
	assert.Contains(t, conditions[0].Expression, "object.metadata.labels['gateway'] == 'istio'")
	t.Logf("Generated CEL expression: %s", conditions[0].Expression)
}

func TestWebhookMatchConditions_WithMockPatterns(t *testing.T) {
	pattern1 := &mockPattern{
		matchExpression: "object.metadata.labels['app'] == 'gateway'",
		podEligible:     true,
	}

	webhook := &Webhook{
		name:     webhookName,
		patterns: newTestPatternMap(pattern1),
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
		matchExpression: "object.metadata.labels['app'] == 'gateway'",
		podEligible:     true,
	}

	webhook := &Webhook{
		name:       webhookName,
		isEnabled:  true,
		endpoint:   "/appsec-proxies",
		resources:  []admcommon.WebhookResourceRule{{APIGroup: "", APIVersion: "v1", Resources: []string{"pods"}}},
		operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Delete},
		patterns:   newTestPatternMap(pattern),
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
		require.Len(t, resources, 1)
		assert.Equal(t, "", resources[0].APIGroup)
		assert.Equal(t, "v1", resources[0].APIVersion)
		assert.Contains(t, resources[0].Resources, "pods")
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
		matchExpression: "object.metadata.labels['gateway-type'] == 'istio'",
		podEligible:     true,
	}
	pattern2 := &mockPattern{
		matchExpression: "object.metadata.labels['gateway-type'] == 'envoy'",
		podEligible:     true,
	}
	pattern3 := &mockPattern{
		matchExpression: "object.metadata.labels['app'] == 'gateway'",
		podEligible:     true,
	}

	webhook := &Webhook{
		name:     webhookName,
		patterns: newTestPatternMap(pattern1, pattern2, pattern3),
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
	assert.Contains(t, expression, "request.operation == 'DELETE'")
	assert.Contains(t, expression, "oldObject.metadata.labels['gateway-type'] == 'istio'")
	assert.Contains(t, expression, "oldObject.metadata.labels['gateway-type'] == 'envoy'")
	assert.Contains(t, expression, "request.operation != 'DELETE'")
	assert.Contains(t, expression, "object.metadata.labels['gateway-type'] == 'istio'")
	assert.Contains(t, expression, "object.metadata.labels['gateway-type'] == 'envoy'")

	assert.Contains(t, expression, "(", "Patterns should be wrapped in parentheses")
	assert.Contains(t, expression, ")", "Patterns should be wrapped in parentheses")
}

// mockPattern is a test implementation of SidecarInjectionPattern
type mockPattern struct {
	matchExpression     string
	podEligible         bool
	mutatePodFunc       func(*corev1.Pod, string, dynamic.Interface) (appsecconfig.MutationOutcome, error)
	podDeletedFunc      func(*corev1.Pod, string, dynamic.Interface) (appsecconfig.MutationOutcome, error)
	mutatePodCallCount  int
	podDeletedCallCount int
}

func (m *mockPattern) MatchCondition() admissionregistrationv1.MatchCondition {
	return admissionregistrationv1.MatchCondition{
		Expression: m.matchExpression,
	}
}

func (m *mockPattern) IsPodEligible(*corev1.Pod, string) bool {
	return m.podEligible
}

func (m *mockPattern) MutatePod(pod *corev1.Pod, ns string, dc dynamic.Interface) (appsecconfig.MutationOutcome, error) {
	m.mutatePodCallCount++
	if m.mutatePodFunc != nil {
		return m.mutatePodFunc(pod, ns, dc)
	}
	return appsecconfig.MutationMutated, nil
}

func (m *mockPattern) PodDeleted(pod *corev1.Pod, ns string, dc dynamic.Interface) (appsecconfig.MutationOutcome, error) {
	m.podDeletedCallCount++
	if m.podDeletedFunc != nil {
		return m.podDeletedFunc(pod, ns, dc)
	}
	return appsecconfig.MutationMutated, nil
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

func newTestPatternMap(patterns ...appsecconfig.SidecarInjectionPattern) map[appsecconfig.ProxyType]appsecconfig.SidecarInjectionPattern {
	patternMap := make(map[appsecconfig.ProxyType]appsecconfig.SidecarInjectionPattern, len(patterns))
	for i, pattern := range patterns {
		patternMap[appsecconfig.AllProxyTypes[i]] = pattern
	}
	return patternMap
}

const sidecarMutationsMetricName = "appsec_injector__sidecar_mutations"

type sidecarMutationLabels struct {
	proxyType appsecconfig.ProxyType
	outcome   string
	reason    string
}

func TestWebhook_WebhookFunc_CreateOperation_countsCanonicalMutationOutcomes(t *testing.T) {
	realErr := errors.New("boom")
	tests := []struct {
		name        string
		outcome     appsecconfig.MutationOutcome
		err         error
		wantOutcome string
		wantReason  string
	}{
		{
			name:        "mutated with nil error",
			outcome:     appsecconfig.MutationMutated,
			wantOutcome: "mutated",
			wantReason:  "none",
		},
		{
			name:        "skipped with bounded reason",
			outcome:     appsecconfig.MutationSkipped,
			err:         &appsecconfig.MutationSkippedReason{Reason: appsecconfig.SkipReasonAlreadySidecar},
			wantOutcome: "skipped",
			wantReason:  "already_sidecar",
		},
		{
			name:        "skipped with plain error",
			outcome:     appsecconfig.MutationSkipped,
			err:         realErr,
			wantOutcome: "error",
			wantReason:  "error",
		},
		{
			name:        "error with plain error",
			outcome:     appsecconfig.MutationError,
			err:         realErr,
			wantOutcome: "error",
			wantReason:  "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := &mockPattern{
				matchExpression: "true",
				podEligible:     true,
				mutatePodFunc: func(*corev1.Pod, string, dynamic.Interface) (appsecconfig.MutationOutcome, error) {
					return tt.outcome, tt.err
				},
			}
			webhook := &Webhook{
				name:          webhookName,
				patterns:      newTestPatternMap(pattern),
				configMutator: &mockMutator{},
			}
			labels := sidecarMutationLabels{
				proxyType: appsecconfig.ProxyTypeEnvoyGateway,
				outcome:   tt.wantOutcome,
				reason:    tt.wantReason,
			}
			request := newTestAdmissionRequest(t, admissionregistrationv1.Create)

			assertSidecarMutationCounterDelta(t, labels, 1, func() {
				response := webhook.WebhookFunc()(request)
				require.NotNil(t, response)
				require.True(t, response.Allowed)
			})

			assert.Equal(t, 1, pattern.mutatePodCallCount)
			assertNoSidecarMutationLabelContains(t, "boom")
		})
	}
}

func TestWebhook_WebhookFunc_DeleteOperation_doesNotCountSidecarMutations(t *testing.T) {
	pattern := &mockPattern{
		matchExpression: "true",
		podEligible:     true,
		podDeletedFunc: func(*corev1.Pod, string, dynamic.Interface) (appsecconfig.MutationOutcome, error) {
			return appsecconfig.MutationMutated, nil
		},
	}
	webhook := &Webhook{
		patterns: map[appsecconfig.ProxyType]appsecconfig.SidecarInjectionPattern{
			appsecconfig.ProxyTypeIngressNginx: pattern,
		},
	}
	request := newTestAdmissionRequest(t, admissionregistrationv1.Delete)
	before := totalSidecarMutationCount(t)

	response := webhook.WebhookFunc()(request)

	require.NotNil(t, response)
	require.True(t, response.Allowed)
	require.Nil(t, response.Result)
	assert.Equal(t, []byte("[]"), response.Patch)
	assert.Equal(t, 1, pattern.podDeletedCallCount)
	assert.Equal(t, before, totalSidecarMutationCount(t))
}

func TestWebhook_callPattern_doesNotCountWhenNoPatternOwnsPod(t *testing.T) {
	pattern := &mockPattern{matchExpression: "true", podEligible: false}
	webhook := &Webhook{patterns: newTestPatternMap(pattern)}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	before := totalSidecarMutationCount(t)

	matched, proxyType, outcome, err := webhook.callPattern(newTestPod("test-pod", "default"), "default", client, appsecconfig.SidecarInjectionPattern.MutatePod)

	require.NoError(t, err)
	assert.False(t, matched)
	assert.Equal(t, appsecconfig.ProxyType(""), proxyType)
	assert.Equal(t, appsecconfig.MutationOutcome(0), outcome)
	assert.Equal(t, 0, pattern.mutatePodCallCount)
	assert.Equal(t, before, totalSidecarMutationCount(t))
}

func assertSidecarMutationCounterDelta(t *testing.T, labels sidecarMutationLabels, want float64, action func()) {
	t.Helper()
	before := sidecarMutationCount(t, labels)
	action()
	after := sidecarMutationCount(t, labels)
	assert.Equal(t, want, after-before)
}

func sidecarMutationCount(t *testing.T, labels sidecarMutationLabels) float64 {
	t.Helper()

	families, err := telemetryimpl.GetCompatComponent().Gather(false)
	require.NoError(t, err)

	for _, family := range families {
		if family.GetName() != sidecarMutationsMetricName {
			continue
		}

		for _, metric := range family.GetMetric() {
			if sidecarMutationLabelsMatch(metricTags(metric.GetLabel()), labels) {
				return metric.GetCounter().GetValue()
			}
		}
	}

	return 0
}

func totalSidecarMutationCount(t *testing.T) float64 {
	t.Helper()

	families, err := telemetryimpl.GetCompatComponent().Gather(false)
	require.NoError(t, err)

	total := 0.0
	for _, family := range families {
		if family.GetName() != sidecarMutationsMetricName {
			continue
		}
		for _, metric := range family.GetMetric() {
			total += metric.GetCounter().GetValue()
		}
	}

	return total
}

func assertNoSidecarMutationLabelContains(t *testing.T, forbidden string) {
	t.Helper()

	families, err := telemetryimpl.GetCompatComponent().Gather(false)
	require.NoError(t, err)

	for _, family := range families {
		if family.GetName() != sidecarMutationsMetricName {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				assert.NotContains(t, label.GetValue(), forbidden)
			}
		}
	}
}

func metricTags(labels []*dto.LabelPair) map[string]string {
	tags := make(map[string]string, len(labels))
	for _, label := range labels {
		tags[label.GetName()] = label.GetValue()
	}
	return tags
}

func sidecarMutationLabelsMatch(tags map[string]string, labels sidecarMutationLabels) bool {
	return len(tags) == 3 &&
		tags["proxy_type"] == string(labels.proxyType) &&
		tags["outcome"] == labels.outcome &&
		tags["reason"] == labels.reason
}

func TestWebhook_callPattern_OwnershipFiltering(t *testing.T) {
	tests := []struct {
		name          string
		podNamespace  string
		podEligible   bool
		expectCalled  bool
		expectMatched bool
	}{
		{
			name:          "pattern called when pod is owned",
			podNamespace:  "default",
			podEligible:   true,
			expectCalled:  true,
			expectMatched: true,
		},
		{
			name:          "pattern not called when pod is not owned",
			podNamespace:  "kube-system",
			podEligible:   false,
			expectCalled:  false,
			expectMatched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPattern := &mockPattern{
				matchExpression: "true",
				podEligible:     tt.podEligible,
			}

			webhook := &Webhook{
				patterns: newTestPatternMap(mockPattern),
			}

			pod := newTestPod("test-pod", tt.podNamespace)
			scheme := runtime.NewScheme()
			client := dynamicfake.NewSimpleDynamicClient(scheme)

			matched, _, outcome, err := webhook.callPattern(pod, tt.podNamespace, client, appsecconfig.SidecarInjectionPattern.MutatePod)

			require.NoError(t, err)
			assert.Equal(t, tt.expectMatched, matched)
			if tt.expectMatched {
				assert.Equal(t, appsecconfig.MutationMutated, outcome)
			} else {
				assert.Equal(t, appsecconfig.MutationOutcome(0), outcome)
			}
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
		matchExpression: "pattern1",
		podEligible:     false,
	}
	pattern2 := &mockPattern{
		matchExpression: "pattern2",
		podEligible:     true,
	}

	webhook := &Webhook{
		patterns: newTestPatternMap(pattern1, pattern2),
	}

	pod := newTestPod("test-pod", "default")
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	matched, proxyType, outcome, err := webhook.callPattern(pod, "default", client, appsecconfig.SidecarInjectionPattern.MutatePod)

	require.NoError(t, err)
	assert.True(t, matched)
	assert.Equal(t, appsecconfig.AllProxyTypes[1], proxyType)
	assert.Equal(t, appsecconfig.MutationMutated, outcome)
	assert.Equal(t, 0, pattern1.mutatePodCallCount, "First pattern should not be called")
	assert.Equal(t, 1, pattern2.mutatePodCallCount, "Second pattern should be called")
}

func TestWebhook_callPattern_CallbackReceivesNamespace(t *testing.T) {
	var capturedNamespace string
	mockPattern := &mockPattern{
		matchExpression: "true",
		podEligible:     true,
		mutatePodFunc: func(_ *corev1.Pod, ns string, _ dynamic.Interface) (appsecconfig.MutationOutcome, error) {
			capturedNamespace = ns
			return appsecconfig.MutationMutated, nil
		},
	}

	webhook := &Webhook{
		patterns: newTestPatternMap(mockPattern),
	}

	pod := newTestPod("test-pod", "default")
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	matched, _, outcome, err := webhook.callPattern(pod, "custom-namespace", client, appsecconfig.SidecarInjectionPattern.MutatePod)

	require.NoError(t, err)
	assert.True(t, matched)
	assert.Equal(t, appsecconfig.MutationMutated, outcome)
	assert.Equal(t, "custom-namespace", capturedNamespace, "Callback should receive correct namespace")
}

func TestWebhook_WebhookFunc_CreateOperation_mutates_config_when_owner_mutates(t *testing.T) {
	mockPattern := &mockPattern{
		matchExpression: "true",
		podEligible:     true,
		mutatePodFunc: func(pod *corev1.Pod, _ string, _ dynamic.Interface) (appsecconfig.MutationOutcome, error) {
			// Add a label to verify mutation happened
			if pod.Labels == nil {
				pod.Labels = make(map[string]string)
			}
			pod.Labels["mutated"] = "true"
			return appsecconfig.MutationMutated, nil
		},
	}
	configMutator := &mockMutator{}

	webhook := &Webhook{
		name:          webhookName,
		patterns:      newTestPatternMap(mockPattern),
		configMutator: configMutator,
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
	assert.Equal(t, 1, configMutator.mutatePodCallCount, "Config mutator should run after sidecar mutation")
}

func TestWebhook_WebhookFunc_CreateOperation_maps_outcomes_for_admission(t *testing.T) {
	realErr := errors.New("boom")
	tests := []struct {
		name                   string
		outcome                appsecconfig.MutationOutcome
		err                    error
		expectResultMessage    string
		expectConfigMutatorRun bool
	}{
		{
			name:                   "mutation mutated runs config mutator",
			outcome:                appsecconfig.MutationMutated,
			expectConfigMutatorRun: true,
		},
		{
			name:    "mutation skipped reason is not propagated",
			outcome: appsecconfig.MutationSkipped,
			err:     &appsecconfig.MutationSkippedReason{Reason: appsecconfig.SkipReasonAlreadySidecar},
		},
		{
			name:                "mutation error propagates real error",
			outcome:             appsecconfig.MutationError,
			err:                 realErr,
			expectResultMessage: "failed to mutate pod: boom",
		},
		{
			name:                "skipped with real error is promoted to error",
			outcome:             appsecconfig.MutationSkipped,
			err:                 realErr,
			expectResultMessage: "failed to mutate pod: boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			mockPattern := &mockPattern{
				matchExpression: "true",
				podEligible:     true,
				mutatePodFunc: func(*corev1.Pod, string, dynamic.Interface) (appsecconfig.MutationOutcome, error) {
					return tt.outcome, tt.err
				},
			}
			configMutator := &mockMutator{}
			webhook := &Webhook{
				name:          webhookName,
				patterns:      newTestPatternMap(mockPattern),
				configMutator: configMutator,
			}
			request := newTestAdmissionRequest(t, admissionregistrationv1.Create)

			// When
			response := webhook.WebhookFunc()(request)

			// Then
			require.NotNil(t, response)
			assert.True(t, response.Allowed)
			assert.Equal(t, 1, mockPattern.mutatePodCallCount)
			if tt.expectResultMessage != "" {
				require.NotNil(t, response.Result)
				assert.Contains(t, response.Result.Message, tt.expectResultMessage)
				assert.Equal(t, 0, configMutator.mutatePodCallCount)
				return
			}

			require.Nil(t, response.Result)
			if tt.expectConfigMutatorRun {
				assert.Equal(t, 1, configMutator.mutatePodCallCount)
			} else {
				assert.Equal(t, 0, configMutator.mutatePodCallCount)
				assert.Contains(t, []string{"[]", "null"}, string(response.Patch))
			}
		})
	}
}

func TestWebhook_WebhookFunc_CreateOperation_returns_false_without_sidecar_counter_when_no_pattern_owns_pod(t *testing.T) {
	// Given
	pattern1 := &mockPattern{matchExpression: "pattern1", podEligible: false}
	pattern2 := &mockPattern{matchExpression: "pattern2", podEligible: false}
	configMutator := &mockMutator{}
	webhook := &Webhook{
		name:          webhookName,
		patterns:      newTestPatternMap(pattern1, pattern2),
		configMutator: configMutator,
	}
	request := newTestAdmissionRequest(t, admissionregistrationv1.Create)

	// When
	response := webhook.WebhookFunc()(request)

	// Then
	require.NotNil(t, response)
	assert.True(t, response.Allowed)
	require.Nil(t, response.Result)
	assert.Contains(t, []string{"[]", "null"}, string(response.Patch))
	assert.Equal(t, 0, pattern1.mutatePodCallCount, "non-owning pattern must not emit sidecar mutation metrics")
	assert.Equal(t, 0, pattern2.mutatePodCallCount, "non-owning pattern must not emit sidecar mutation metrics")
	assert.Equal(t, 0, configMutator.mutatePodCallCount)
}

func TestWebhook_WebhookFunc_DeleteOperation_maps_outcomes_for_admission(t *testing.T) {
	realErr := errors.New("boom")
	tests := []struct {
		name                string
		podEligible         bool
		outcome             appsecconfig.MutationOutcome
		err                 error
		expectDeletedCalled bool
		expectResultMessage string
	}{
		{
			name:                "owner mutated returns empty patch",
			podEligible:         true,
			outcome:             appsecconfig.MutationMutated,
			expectDeletedCalled: true,
		},
		{
			name:                "owner skipped returns empty patch",
			podEligible:         true,
			outcome:             appsecconfig.MutationSkipped,
			err:                 &appsecconfig.MutationSkippedReason{Reason: appsecconfig.SkipReasonAlreadySidecar},
			expectDeletedCalled: true,
		},
		{
			name:                "owner error returns delete error",
			podEligible:         true,
			outcome:             appsecconfig.MutationError,
			err:                 realErr,
			expectDeletedCalled: true,
			expectResultMessage: "failed to delete resources associated with sidecar: boom",
		},
		{
			name:        "no owner returns empty patch",
			podEligible: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			mockPattern := &mockPattern{
				matchExpression: "true",
				podEligible:     tt.podEligible,
				podDeletedFunc: func(*corev1.Pod, string, dynamic.Interface) (appsecconfig.MutationOutcome, error) {
					return tt.outcome, tt.err
				},
			}
			webhook := &Webhook{
				name:     webhookName,
				patterns: newTestPatternMap(mockPattern),
			}
			request := newTestAdmissionRequest(t, admissionregistrationv1.Delete)

			// When
			response := webhook.WebhookFunc()(request)

			// Then
			require.NotNil(t, response)
			assert.True(t, response.Allowed, "Request should be allowed")
			if tt.expectDeletedCalled {
				assert.Equal(t, 1, mockPattern.podDeletedCallCount, "Pattern PodDeleted should be called")
			} else {
				assert.Equal(t, 0, mockPattern.podDeletedCallCount, "Pattern PodDeleted should not be called")
			}
			if tt.expectResultMessage != "" {
				require.NotNil(t, response.Result)
				assert.Contains(t, response.Result.Message, tt.expectResultMessage)
				return
			}

			require.Nil(t, response.Result)
			assert.Equal(t, []byte("[]"), response.Patch, "Should have empty patch")
		})
	}
}

func TestWebhook_WebhookFunc_InvalidJSON(t *testing.T) {
	webhook := &Webhook{
		name:     webhookName,
		patterns: newTestPatternMap(),
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
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	pod.Labels["config-mutated"] = "true"
	return true, nil
}

func newTestAdmissionRequest(t *testing.T, operation admissionregistrationv1.OperationType) *admission.Request {
	t.Helper()

	pod := newTestPod("test-pod", "default")
	podBytes, err := json.Marshal(pod)
	require.NoError(t, err)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	request := &admission.Request{
		Operation:     operation,
		Namespace:     "default",
		DynamicClient: client,
	}
	switch operation {
	case admissionregistrationv1.Create:
		request.Object = podBytes
	case admissionregistrationv1.Delete:
		request.OldObject = podBytes
	}

	return request
}
