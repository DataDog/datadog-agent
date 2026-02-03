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

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

const (
	testNamespace            = "gateway-ns"
	testGatewayClassName     = "istio"
	gatewayClassNamePodLabel = "gateway.networking.k8s.io/gateway-class-name"
	sidecarContainerName     = "datadog-appsec-processor"
)

// mockSidecarPattern implements appsecconfig.SidecarInjectionPattern for testing
type mockSidecarPattern struct {
	matchExpression     string
	shouldMutate        bool
	namespaceEligible   bool
	injectSidecar       bool
	injectSidecarErr    error
	mutatePodCallCount  int
	podDeletedCallCount int
	sidecarImage        string
	sidecarPort         int32
}

func (m *mockSidecarPattern) Mode() appsecconfig.InjectionMode {
	return appsecconfig.InjectionModeSidecar
}

func (m *mockSidecarPattern) IsInjectionPossible(context.Context) error {
	return nil
}

func (m *mockSidecarPattern) Resource() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gatewayclasses"}
}

func (m *mockSidecarPattern) Namespace() string {
	return metav1.NamespaceAll
}

func (m *mockSidecarPattern) Added(context.Context, *unstructured.Unstructured) error {
	return nil
}

func (m *mockSidecarPattern) Deleted(context.Context, *unstructured.Unstructured) error {
	return nil
}

func (m *mockSidecarPattern) MatchCondition() admissionregistrationv1.MatchCondition {
	return admissionregistrationv1.MatchCondition{
		Expression: m.matchExpression,
	}
}

func (m *mockSidecarPattern) ShouldMutatePod(_ *corev1.Pod) bool {
	return m.shouldMutate
}

func (m *mockSidecarPattern) IsNamespaceEligible(_ string) bool {
	return m.namespaceEligible
}

func (m *mockSidecarPattern) MutatePod(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
	m.mutatePodCallCount++
	if m.injectSidecarErr != nil {
		return false, m.injectSidecarErr
	}
	if !m.injectSidecar {
		return false, nil
	}

	// Inject sidecar container
	sidecarContainer := corev1.Container{
		Name:  sidecarContainerName,
		Image: m.sidecarImage,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: m.sidecarPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
	}
	pod.Spec.Containers = append(pod.Spec.Containers, sidecarContainer)
	return true, nil
}

func (m *mockSidecarPattern) PodDeleted(_ *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
	m.podDeletedCallCount++
	return true, nil
}

// noopMutator is a mutator that does nothing, used for testing
type noopMutator struct{}

func (n *noopMutator) MutatePod(_ *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
	return false, nil
}

// newTestWebhook creates a Webhook with mock patterns for testing
func newTestWebhook(patterns []appsecconfig.SidecarInjectionPattern) *Webhook {
	return &Webhook{
		name:          webhookName,
		isEnabled:     len(patterns) > 0,
		endpoint:      "/appsec-proxies",
		resources:     map[string][]string{"": {"pods"}},
		operations:    []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Delete},
		patterns:      patterns,
		configMutator: &noopMutator{},
	}
}

// newGatewayPod creates a pod that looks like it belongs to a gateway
func newGatewayPod(name, namespace, gatewayClassName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				gatewayClassNamePodLabel: gatewayClassName,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "istio-proxy",
					Image: "istio/proxyv2:latest",
				},
			},
		},
	}
}

// TestAppsecWebhookIntegration is an integration style test that ensures configuration
// maps to the expected pod mutation for appsec sidecar injection.
func TestAppsecWebhookIntegration(t *testing.T) {
	type expected struct {
		// sidecarInjected indicates whether a sidecar container should be present
		sidecarInjected bool
		// sidecarImage is the expected image of the sidecar container
		sidecarImage string
		// sidecarPort is the expected port of the sidecar container
		sidecarPort int32
		// containerCount is the expected number of containers after mutation
		containerCount int
		// mutatePodCalled indicates MutatePod should have been called
		mutatePodCalled bool
	}

	tests := map[string]struct {
		pod          *corev1.Pod
		pattern      *mockSidecarPattern
		shouldMutate bool
		expected     *expected
	}{
		"pod with gateway class label should be mutated": {
			pod: newGatewayPod("gateway-pod", testNamespace, testGatewayClassName),
			pattern: &mockSidecarPattern{
				matchExpression:   "'" + gatewayClassNamePodLabel + "' in object.metadata.labels",
				shouldMutate:      true,
				namespaceEligible: true,
				injectSidecar:     true,
				sidecarImage:      "ghcr.io/datadog/appsec:latest",
				sidecarPort:       8080,
			},
			shouldMutate: true,
			expected: &expected{
				sidecarInjected: true,
				sidecarImage:    "ghcr.io/datadog/appsec:latest",
				sidecarPort:     8080,
				containerCount:  2,
				mutatePodCalled: true,
			},
		},
		"pod without gateway class label should not be mutated": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "regular-pod",
					Namespace: testNamespace,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "nginx:latest"},
					},
				},
			},
			pattern: &mockSidecarPattern{
				matchExpression:   "'" + gatewayClassNamePodLabel + "' in object.metadata.labels",
				shouldMutate:      false, // ShouldMutatePod returns false
				namespaceEligible: true,
				injectSidecar:     true,
				sidecarImage:      "ghcr.io/datadog/appsec:latest",
				sidecarPort:       8080,
			},
			shouldMutate: false,
			expected:     nil,
		},
		"pod in ineligible namespace should not be mutated": {
			pod: newGatewayPod("gateway-pod", "datadog", testGatewayClassName),
			pattern: &mockSidecarPattern{
				matchExpression:   "'" + gatewayClassNamePodLabel + "' in object.metadata.labels",
				shouldMutate:      true,
				namespaceEligible: false, // Namespace not eligible
				injectSidecar:     true,
				sidecarImage:      "ghcr.io/datadog/appsec:latest",
				sidecarPort:       8080,
			},
			shouldMutate: false,
			expected:     nil,
		},
		"pod that already has sidecar should not be mutated again": {
			pod: func() *corev1.Pod {
				pod := newGatewayPod("gateway-pod", testNamespace, testGatewayClassName)
				pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
					Name:  sidecarContainerName,
					Image: "ghcr.io/datadog/appsec:v1.0",
				})
				return pod
			}(),
			pattern: &mockSidecarPattern{
				matchExpression:   "'" + gatewayClassNamePodLabel + "' in object.metadata.labels",
				shouldMutate:      false, // ShouldMutatePod returns false (sidecar exists)
				namespaceEligible: true,
				injectSidecar:     true,
				sidecarImage:      "ghcr.io/datadog/appsec:latest",
				sidecarPort:       8080,
			},
			shouldMutate: false,
			expected:     nil,
		},
		"pattern that returns false for inject should not add sidecar": {
			pod: newGatewayPod("gateway-pod", testNamespace, testGatewayClassName),
			pattern: &mockSidecarPattern{
				matchExpression:   "'" + gatewayClassNamePodLabel + "' in object.metadata.labels",
				shouldMutate:      true,
				namespaceEligible: true,
				injectSidecar:     false, // Pattern decides not to inject
				sidecarImage:      "ghcr.io/datadog/appsec:latest",
				sidecarPort:       8080,
			},
			shouldMutate: false,
			expected: &expected{
				sidecarInjected: false,
				containerCount:  1,
				mutatePodCalled: true,
			},
		},
		"sidecar injection with custom port": {
			pod: newGatewayPod("gateway-pod", testNamespace, testGatewayClassName),
			pattern: &mockSidecarPattern{
				matchExpression:   "'" + gatewayClassNamePodLabel + "' in object.metadata.labels",
				shouldMutate:      true,
				namespaceEligible: true,
				injectSidecar:     true,
				sidecarImage:      "custom-registry/appsec-processor:v2.0",
				sidecarPort:       9090,
			},
			shouldMutate: true,
			expected: &expected{
				sidecarInjected: true,
				sidecarImage:    "custom-registry/appsec-processor:v2.0",
				sidecarPort:     9090,
				containerCount:  2,
				mutatePodCalled: true,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup webhook with mock pattern
			webhook := newTestWebhook([]appsecconfig.SidecarInjectionPattern{test.pattern})
			require.NotNil(t, webhook)

			// Create dynamic client
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			mockDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

			// Mutate pod
			in := test.pod.DeepCopy()
			mutated, err := webhook.callPattern(in, in.Namespace, mockDynamic, appsecconfig.SidecarInjectionPattern.MutatePod)
			require.NoError(t, err)

			// Verify mutation occurred or not
			if !test.shouldMutate {
				if test.expected != nil && test.expected.mutatePodCalled {
					assert.Equal(t, 1, test.pattern.mutatePodCallCount, "MutatePod should be called")
				}
				assert.False(t, mutated, "Pod should not be mutated")
				return
			}

			// Mutation expected
			require.NotNil(t, test.expected, "Test setup error: expected should be defined when shouldMutate is true")
			assert.True(t, mutated, "Pod should be mutated")
			assert.Equal(t, 1, test.pattern.mutatePodCallCount, "MutatePod should be called once")

			// Verify sidecar injection
			if test.expected.sidecarInjected {
				require.Len(t, in.Spec.Containers, test.expected.containerCount, "Container count should match")

				// Find sidecar container
				var sidecar *corev1.Container
				for i := range in.Spec.Containers {
					if in.Spec.Containers[i].Name == sidecarContainerName {
						sidecar = &in.Spec.Containers[i]
						break
					}
				}
				require.NotNil(t, sidecar, "Sidecar container should exist")
				assert.Equal(t, test.expected.sidecarImage, sidecar.Image, "Sidecar image should match")
				require.Len(t, sidecar.Ports, 1, "Sidecar should have one port")
				assert.Equal(t, test.expected.sidecarPort, sidecar.Ports[0].ContainerPort, "Sidecar port should match")
			}
		})
	}
}

// TestAppsecWebhookDeleteOperation tests the webhook's DELETE operation handling
func TestAppsecWebhookDeleteOperation(t *testing.T) {
	tests := map[string]struct {
		pod                 *corev1.Pod
		pattern             *mockSidecarPattern
		expectPodDeleteCall bool
	}{
		"delete operation should call PodDeleted on matching pattern": {
			pod: newGatewayPod("gateway-pod", testNamespace, testGatewayClassName),
			pattern: &mockSidecarPattern{
				matchExpression:   "'" + gatewayClassNamePodLabel + "' in object.metadata.labels",
				shouldMutate:      true,
				namespaceEligible: true,
			},
			expectPodDeleteCall: true,
		},
		"delete operation should not call PodDeleted on non-matching pattern": {
			pod: newGatewayPod("gateway-pod", testNamespace, testGatewayClassName),
			pattern: &mockSidecarPattern{
				matchExpression:   "'" + gatewayClassNamePodLabel + "' in object.metadata.labels",
				shouldMutate:      false, // Pattern doesn't match
				namespaceEligible: true,
			},
			expectPodDeleteCall: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup webhook with mock pattern
			webhook := newTestWebhook([]appsecconfig.SidecarInjectionPattern{test.pattern})
			require.NotNil(t, webhook)

			// Create dynamic client
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			mockDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

			// Serialize pod for OldObject
			podBytes, err := json.Marshal(test.pod)
			require.NoError(t, err)

			// Create DELETE request
			request := &admission.Request{
				Operation:     admissionregistrationv1.Delete,
				Namespace:     test.pod.Namespace,
				OldObject:     podBytes,
				DynamicClient: mockDynamic,
			}

			// Call webhook function
			webhookFunc := webhook.WebhookFunc()
			response := webhookFunc(request)

			// Verify response
			require.NotNil(t, response)
			assert.True(t, response.Allowed, "DELETE should always be allowed")

			// Verify PodDeleted was called appropriately
			if test.expectPodDeleteCall {
				assert.Equal(t, 1, test.pattern.podDeletedCallCount, "PodDeleted should be called")
			} else {
				assert.Equal(t, 0, test.pattern.podDeletedCallCount, "PodDeleted should not be called")
			}
		})
	}
}

// TestAppsecWebhookMultiplePatterns tests webhook behavior with multiple patterns
func TestAppsecWebhookMultiplePatterns(t *testing.T) {
	// First pattern doesn't match, second one does
	pattern1 := &mockSidecarPattern{
		matchExpression:   "'some-other-label' in object.metadata.labels",
		shouldMutate:      false,
		namespaceEligible: true,
		injectSidecar:     true,
		sidecarImage:      "pattern1:latest",
		sidecarPort:       8080,
	}

	pattern2 := &mockSidecarPattern{
		matchExpression:   "'" + gatewayClassNamePodLabel + "' in object.metadata.labels",
		shouldMutate:      true,
		namespaceEligible: true,
		injectSidecar:     true,
		sidecarImage:      "pattern2:latest",
		sidecarPort:       9090,
	}

	webhook := newTestWebhook([]appsecconfig.SidecarInjectionPattern{pattern1, pattern2})
	require.NotNil(t, webhook)

	// Create dynamic client
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	mockDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	// Create pod that matches pattern2
	pod := newGatewayPod("gateway-pod", testNamespace, testGatewayClassName)
	in := pod.DeepCopy()

	// Mutate pod
	mutated, err := webhook.callPattern(in, in.Namespace, mockDynamic, appsecconfig.SidecarInjectionPattern.MutatePod)
	require.NoError(t, err)
	assert.True(t, mutated, "Pod should be mutated")

	// Verify pattern1 was not called, pattern2 was called
	assert.Equal(t, 0, pattern1.mutatePodCallCount, "Pattern1 should not be called")
	assert.Equal(t, 1, pattern2.mutatePodCallCount, "Pattern2 should be called")

	// Verify sidecar from pattern2 was injected
	require.Len(t, in.Spec.Containers, 2)
	var sidecar *corev1.Container
	for i := range in.Spec.Containers {
		if in.Spec.Containers[i].Name == sidecarContainerName {
			sidecar = &in.Spec.Containers[i]
			break
		}
	}
	require.NotNil(t, sidecar)
	assert.Equal(t, "pattern2:latest", sidecar.Image)
	assert.Equal(t, int32(9090), sidecar.Ports[0].ContainerPort)
}

// TestAppsecWebhookMatchConditions tests that match conditions are properly generated
func TestAppsecWebhookMatchConditions(t *testing.T) {
	tests := map[string]struct {
		patterns        []*mockSidecarPattern
		expectedOrCount int // Number of || operators expected
	}{
		"single pattern": {
			patterns: []*mockSidecarPattern{
				{matchExpression: "'label1' in object.metadata.labels"},
			},
			expectedOrCount: 0,
		},
		"two patterns": {
			patterns: []*mockSidecarPattern{
				{matchExpression: "'label1' in object.metadata.labels"},
				{matchExpression: "'label2' in object.metadata.labels"},
			},
			expectedOrCount: 1,
		},
		"three patterns": {
			patterns: []*mockSidecarPattern{
				{matchExpression: "'label1' in object.metadata.labels"},
				{matchExpression: "'label2' in object.metadata.labels"},
				{matchExpression: "'label3' in object.metadata.labels"},
			},
			expectedOrCount: 2,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			patterns := make([]appsecconfig.SidecarInjectionPattern, len(test.patterns))
			for i, p := range test.patterns {
				patterns[i] = p
			}

			webhook := newTestWebhook(patterns)
			require.NotNil(t, webhook)

			conditions := webhook.MatchConditions()
			require.Len(t, conditions, 1, "Should have exactly one match condition")

			expression := conditions[0].Expression

			// Count || operators
			orCount := 0
			for i := 0; i < len(expression)-1; i++ {
				if expression[i] == '|' && expression[i+1] == '|' {
					orCount++
				}
			}
			assert.Equal(t, test.expectedOrCount, orCount, "Should have correct number of || operators")

			// Verify each pattern's expression is included
			for _, p := range test.patterns {
				assert.Contains(t, expression, p.matchExpression, "Expression should contain pattern's match expression")
			}

			t.Logf("Generated expression: %s", expression)
		})
	}
}

// TestAppsecWebhookNoPatterns tests behavior when no patterns are configured
func TestAppsecWebhookNoPatterns(t *testing.T) {
	webhook := newTestWebhook([]appsecconfig.SidecarInjectionPattern{})
	require.NotNil(t, webhook)

	// Match conditions should be empty
	conditions := webhook.MatchConditions()
	assert.Empty(t, conditions, "Should have no match conditions when no patterns")

	// Webhook should not be enabled
	assert.False(t, webhook.IsEnabled(), "Webhook should not be enabled without patterns")
}

// TestAppsecWebhookCreateOperation tests the full CREATE operation flow
func TestAppsecWebhookCreateOperation(t *testing.T) {
	pattern := &mockSidecarPattern{
		matchExpression:   "'" + gatewayClassNamePodLabel + "' in object.metadata.labels",
		shouldMutate:      true,
		namespaceEligible: true,
		injectSidecar:     true,
		sidecarImage:      "ghcr.io/datadog/appsec:latest",
		sidecarPort:       8080,
	}

	webhook := newTestWebhook([]appsecconfig.SidecarInjectionPattern{pattern})
	require.NotNil(t, webhook)

	// Create dynamic client
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	mockDynamic := dynamicfake.NewSimpleDynamicClient(scheme)

	// Create pod and serialize
	pod := newGatewayPod("gateway-pod", testNamespace, testGatewayClassName)
	podBytes, err := json.Marshal(pod)
	require.NoError(t, err)

	// Create CREATE request
	request := &admission.Request{
		Operation:     admissionregistrationv1.Create,
		Namespace:     pod.Namespace,
		Object:        podBytes,
		DynamicClient: mockDynamic,
	}

	// Call webhook function
	webhookFunc := webhook.WebhookFunc()
	response := webhookFunc(request)

	// Verify response
	require.NotNil(t, response)
	assert.True(t, response.Allowed, "CREATE should be allowed")
	assert.NotNil(t, response.Patch, "Should have a patch")

	// Verify pattern was called
	assert.Equal(t, 1, pattern.mutatePodCallCount, "MutatePod should be called")
}
