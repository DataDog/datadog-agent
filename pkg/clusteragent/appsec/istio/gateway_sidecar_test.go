// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package istio

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"

	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

// newTestNativeGatewaySidecarPattern creates a test instance of the native gateway sidecar pattern
func newTestNativeGatewaySidecarPattern(t *testing.T) *istioNativeGatewaySidecarPattern {
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := newFakeDynamicClient(scheme)

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
		Injection: appsecconfig.Injection{
			IstioNamespace:    "istio-system",
			CommonLabels:      map[string]string{"app": "datadog"},
			CommonAnnotations: map[string]string{"managed-by": "datadog"},
		},
	}

	recorder := record.NewFakeRecorder(100)

	basePattern := &istioInjectionPattern{
		client: client,
		logger: logger,
		config: config,
		eventRecorder: eventRecorder{
			recorder: recorder,
		},
	}

	return &istioNativeGatewaySidecarPattern{
		istioNativeGatewayPattern: &istioNativeGatewayPattern{
			istioInjectionPattern: basePattern,
		},
	}
}

// newTestPodForGatewaySidecar creates a test pod with Istio-related labels
func newTestPodForGatewaySidecar(name, namespace string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
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

func TestGatewaySidecar_ShouldMutatePod_MatchingSelector(t *testing.T) {
	pattern := newTestNativeGatewaySidecarPattern(t)

	gateway := newTestIstioGateway("test-gw", "default", map[string]any{
		"app":   "istio-gateway",
		"istio": "ingressgateway",
	})

	// Pre-populate client with the gateway
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{*gateway},
		}, nil
	})

	pod := newTestPodForGatewaySidecar("test-pod", "default", map[string]string{
		"app":   "istio-gateway",
		"istio": "ingressgateway",
	})

	result := pattern.ShouldMutatePod(pod)
	assert.True(t, result, "Should mutate pod when selector matches")
}

func TestGatewaySidecar_ShouldMutatePod_NoMatchingGateway(t *testing.T) {
	pattern := newTestNativeGatewaySidecarPattern(t)

	gateway := newTestIstioGateway("test-gw", "default", map[string]any{
		"app": "other-gateway",
	})

	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{*gateway},
		}, nil
	})

	pod := newTestPodForGatewaySidecar("test-pod", "default", map[string]string{
		"app":   "istio-gateway",
		"istio": "ingressgateway",
	})

	result := pattern.ShouldMutatePod(pod)
	assert.False(t, result, "Should not mutate pod when no gateway selector matches")
}

func TestGatewaySidecar_ShouldMutatePod_AlreadyInjected(t *testing.T) {
	pattern := newTestNativeGatewaySidecarPattern(t)

	pod := newTestPodForGatewaySidecar("test-pod", "default", map[string]string{
		"app":   "istio-gateway",
		"istio": "ingressgateway",
	})
	// Add sidecar container
	pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
		Name:  sidecarContainerName,
		Image: "datadog/appsec-processor:latest",
	})

	result := pattern.ShouldMutatePod(pod)
	assert.False(t, result, "Should not mutate pod that already has sidecar")
}

func TestGatewaySidecar_ShouldMutatePod_ListError(t *testing.T) {
	pattern := newTestNativeGatewaySidecarPattern(t)

	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.NewInternalError(assert.AnError)
	})

	pod := newTestPodForGatewaySidecar("test-pod", "default", map[string]string{
		"app":   "istio-gateway",
		"istio": "ingressgateway",
	})

	result := pattern.ShouldMutatePod(pod)
	assert.False(t, result, "Should not mutate pod when listing gateways fails")
}

func TestGatewaySidecar_MutatePod_Success(t *testing.T) {
	pattern := newTestNativeGatewaySidecarPattern(t)

	gateway := newTestIstioGateway("test-gw", "default", map[string]any{
		"app":   "istio-gateway",
		"istio": "ingressgateway",
	})

	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{*gateway},
		}, nil
	})

	var createdFilter *unstructured.Unstructured
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "envoyfilters", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdFilter = createAction.GetObject().(*unstructured.Unstructured)
		return true, createdFilter, nil
	})

	pod := newTestPodForGatewaySidecar("test-pod", "default", map[string]string{
		"app":   "istio-gateway",
		"istio": "ingressgateway",
	})

	modified, err := pattern.MutatePod(pod, "default", pattern.client)

	require.NoError(t, err)
	assert.True(t, modified)
	require.Len(t, pod.Spec.Containers, 2, "Should have original + sidecar container")
	assert.Equal(t, sidecarContainerName, pod.Spec.Containers[1].Name)
	assert.NotNil(t, createdFilter, "EnvoyFilter should be created")
}

func TestGatewaySidecar_MutatePod_NoMatchingGateway(t *testing.T) {
	pattern := newTestNativeGatewaySidecarPattern(t)

	// Return empty gateway list
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{},
		}, nil
	})

	pod := newTestPodForGatewaySidecar("test-pod", "default", map[string]string{
		"app":   "istio-gateway",
		"istio": "ingressgateway",
	})

	modified, err := pattern.MutatePod(pod, "default", pattern.client)

	require.NoError(t, err)
	assert.False(t, modified)
	assert.Len(t, pod.Spec.Containers, 1, "Should not add sidecar when no gateway matches")
}

func TestGatewaySidecar_MutatePod_EnvoyFilterCreationError(t *testing.T) {
	pattern := newTestNativeGatewaySidecarPattern(t)

	gateway := newTestIstioGateway("test-gw", "default", map[string]any{
		"app": "istio-gateway",
	})

	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{*gateway},
		}, nil
	})

	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "envoyfilters", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.NewInternalError(assert.AnError)
	})

	pod := newTestPodForGatewaySidecar("test-pod", "default", map[string]string{
		"app": "istio-gateway",
	})

	modified, err := pattern.MutatePod(pod, "default", pattern.client)

	require.Error(t, err)
	assert.False(t, modified)
	assert.Contains(t, err.Error(), "could not create Envoy Filter")
}

func TestGatewaySidecar_MatchCondition(t *testing.T) {
	pattern := newTestNativeGatewaySidecarPattern(t)

	condition := pattern.MatchCondition()

	assert.NotEmpty(t, condition.Expression)
	// Standard Istio gateway pods carry the "istio" label key (istio=ingressgateway etc.)
	// This pre-filter passes those pods through to ShouldMutatePod for precise selector matching.
	assert.Contains(t, condition.Expression, "istio")
	assert.Contains(t, condition.Expression, "object.metadata.labels")

	t.Logf("Generated CEL expression: %s", condition.Expression)
}

func TestGatewaySidecar_SelectorMatchesPod(t *testing.T) {
	tests := []struct {
		name        string
		selector    map[string]any
		podLabels   map[string]string
		expectMatch bool
	}{
		{
			name:        "exact match",
			selector:    map[string]any{"app": "istio-gateway", "istio": "ingressgateway"},
			podLabels:   map[string]string{"app": "istio-gateway", "istio": "ingressgateway"},
			expectMatch: true,
		},
		{
			name:        "pod has extra labels",
			selector:    map[string]any{"app": "istio-gateway"},
			podLabels:   map[string]string{"app": "istio-gateway", "istio": "ingressgateway", "extra": "label"},
			expectMatch: true,
		},
		{
			name:        "partial match fails",
			selector:    map[string]any{"app": "istio-gateway", "istio": "ingressgateway"},
			podLabels:   map[string]string{"app": "istio-gateway"},
			expectMatch: false,
		},
		{
			name:        "no match",
			selector:    map[string]any{"app": "other"},
			podLabels:   map[string]string{"app": "istio-gateway"},
			expectMatch: false,
		},
		{
			name:        "empty selector",
			selector:    map[string]any{},
			podLabels:   map[string]string{"app": "istio-gateway"},
			expectMatch: false,
		},
		{
			name:        "nil selector",
			selector:    nil,
			podLabels:   map[string]string{"app": "istio-gateway"},
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := newTestIstioGateway("test-gw", "default", tt.selector)
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: tt.podLabels,
				},
			}

			result := selectorMatchesPod(gateway, pod)
			assert.Equal(t, tt.expectMatch, result)
		})
	}
}

func TestGatewaySidecar_IsNamespaceEligible(t *testing.T) {
	pattern := newTestNativeGatewaySidecarPattern(t)

	for _, ns := range []string{"default", "kube-system", "istio-system", "datadog", ""} {
		t.Run("namespace_"+ns, func(t *testing.T) {
			result := pattern.IsNamespaceEligible(ns)
			assert.True(t, result, "All namespaces should be eligible")
		})
	}
}

func TestGatewaySidecar_PodDeleted_IsNoOp(t *testing.T) {
	pattern := newTestNativeGatewaySidecarPattern(t)

	pod := newTestPodForGatewaySidecar("test-pod", "default", map[string]string{"app": "istio-gateway"})

	deleteCalled := false
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("delete", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteCalled = true
		return true, nil, nil
	})

	_, err := pattern.PodDeleted(pod, "default", pattern.client)

	require.NoError(t, err)
	assert.False(t, deleteCalled, "PodDeleted should be a no-op")
}

func TestGatewaySidecar_Added_IsNoOp(t *testing.T) {
	ctx := context.Background()
	pattern := newTestNativeGatewaySidecarPattern(t)

	gw := newTestIstioGateway("test-gw", "default", map[string]any{"app": "istio-gateway"})

	err := pattern.Added(ctx, gw)

	require.NoError(t, err)
}

func TestGatewaySidecar_MutatePod_ListGatewaysError(t *testing.T) {
	pattern := newTestNativeGatewaySidecarPattern(t)

	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.NewInternalError(assert.AnError)
	})

	pod := newTestPodForGatewaySidecar("test-pod", "default", map[string]string{
		"app": "istio-gateway",
	})

	modified, err := pattern.MutatePod(pod, "default", pattern.client)

	require.Error(t, err)
	assert.False(t, modified)
	assert.Contains(t, err.Error(), "error listing Istio gateways")
}

func TestGatewaySidecar_SelectorMatchesPod_MissingSelectorField(t *testing.T) {
	// Gateway without a spec.selector field at all
	gateway := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "networking.istio.io/v1",
			"kind":       "Gateway",
			"metadata": map[string]any{
				"name":      "no-selector",
				"namespace": "default",
			},
			"spec": map[string]any{
				"servers": []any{},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"app": "test"},
		},
	}

	result := selectorMatchesPod(gateway, pod)
	assert.False(t, result)
}

func TestGatewaySidecar_Resource(t *testing.T) {
	pattern := newTestNativeGatewaySidecarPattern(t)

	gvr := pattern.Resource()
	assert.Equal(t, schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1",
		Resource: "gateways",
	}, gvr)
}
