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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

const (
	sidecarContainerName = "datadog-appsec-processor" // From sidecar package
)

// newTestIstioSidecarPattern creates a test instance of the sidecar pattern
func newTestIstioSidecarPattern(t *testing.T) *istioGatewaySidecarPattern {
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)

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

	basePattern := newTestIstioPattern(client, logger, config)

	return &istioGatewaySidecarPattern{
		istioInjectionPattern: basePattern,
	}
}

// newTestPod creates a test pod with gateway class label
func newTestPodForSidecar(name, namespace, gatewayClassName string) *corev1.Pod {
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
					Name:  "app",
					Image: "nginx:latest",
				},
			},
		},
	}
}

func TestSidecarPattern_Added_IsNoOp(t *testing.T) {
	ctx := context.Background()
	pattern := newTestIstioSidecarPattern(t)

	gwClass := newTestGatewayClass("istio", istioGatewayControllerName)

	// Execute - should be a no-op
	err := pattern.Added(ctx, gwClass)

	// Verify
	require.NoError(t, err)
}

func TestSidecarPattern_InjectSidecar_Success(t *testing.T) {
	ctx := context.Background()
	pattern := newTestIstioSidecarPattern(t)

	pod := newTestPodForSidecar("test-pod", "default", "istio")
	gwClass := newTestGatewayClass("istio", istioGatewayControllerName)

	// Setup mock client to return the gateway class
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "gatewayclasses", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, gwClass, nil
	})

	// Track created resources
	var createdFilter *unstructured.Unstructured

	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "envoyfilters", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdFilter = createAction.GetObject().(*unstructured.Unstructured)
		return true, createdFilter, nil
	})

	// Execute
	modified, err := pattern.InjectSidecar(ctx, pod, "default")

	// Verify
	require.NoError(t, err)
	assert.True(t, modified, "Pod should be modified")

	// Check that sidecar container was added
	require.Len(t, pod.Spec.Containers, 2, "Should have original container + sidecar")
	sidecarContainer := pod.Spec.Containers[1]
	assert.Equal(t, sidecarContainerName, sidecarContainer.Name, "Sidecar should have correct name")

	// Check that EnvoyFilter was created
	assert.NotNil(t, createdFilter, "EnvoyFilter should be created")
}

func TestSidecarPattern_InjectSidecar_AlreadyInjected(t *testing.T) {
	ctx := context.Background()
	pattern := newTestIstioSidecarPattern(t)

	pod := newTestPodForSidecar("test-pod", "default", "istio")
	// Add sidecar container to simulate already injected pod
	pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
		Name:  sidecarContainerName,
		Image: "datadog/appsec-processor:latest",
	})

	// Execute
	modified, err := pattern.InjectSidecar(ctx, pod, "default")

	// Verify
	require.NoError(t, err)
	assert.False(t, modified, "Pod should not be modified if already has sidecar")
	assert.Len(t, pod.Spec.Containers, 2, "Should still have 2 containers")
}

func TestSidecarPattern_InjectSidecar_GatewayClassNotFound(t *testing.T) {
	ctx := context.Background()
	pattern := newTestIstioSidecarPattern(t)

	pod := newTestPodForSidecar("test-pod", "default", "missing-gatewayclass")

	// Setup mock client to return NotFound error
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "gatewayclasses", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.NewNotFound(schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "gatewayclasses"}, "missing-gatewayclass")
	})

	// Execute
	modified, err := pattern.InjectSidecar(ctx, pod, "default")

	// Verify
	require.Error(t, err)
	assert.False(t, modified)
	assert.Contains(t, err.Error(), "error getting gatewayclass")
}

func TestSidecarPattern_InjectSidecar_NonIstioGatewayClass(t *testing.T) {
	ctx := context.Background()
	pattern := newTestIstioSidecarPattern(t)

	pod := newTestPodForSidecar("test-pod", "default", "envoy")
	gwClass := newTestGatewayClass("envoy", "envoy.io/gateway-controller")

	// Setup mock client to return non-Istio gateway class
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "gatewayclasses", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, gwClass, nil
	})

	createCalled := false
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createCalled = true
		return true, nil, nil
	})

	// Execute
	modified, err := pattern.InjectSidecar(ctx, pod, "default")

	// Verify
	require.NoError(t, err)
	assert.False(t, modified, "Pod should not be modified for non-Istio gateway")
	assert.False(t, createCalled, "Should not create resources for non-Istio gateway")
}

func TestSidecarPattern_SidecarDeleted_IsNoOp(t *testing.T) {
	ctx := context.Background()
	pattern := newTestIstioSidecarPattern(t)

	pod := newTestPodForSidecar("test-pod", "default", "istio")

	deleteCalled := false
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("delete", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteCalled = true
		return true, nil, nil
	})

	// Execute - should be a no-op
	err := pattern.SidecarDeleted(ctx, pod, "default")

	// Verify
	require.NoError(t, err)
	assert.False(t, deleteCalled, "SidecarDeleted should be a no-op")
}

func TestSidecarPattern_PodSelector(t *testing.T) {
	pattern := newTestIstioSidecarPattern(t)

	selector := pattern.PodSelector()

	// Verify selector is not nil
	require.NotNil(t, selector)

	// Test that it matches pods with gateway class label
	podWithLabel := labels.Set{
		gatewayClassNamePodLabel: "istio",
	}
	assert.True(t, selector.Matches(podWithLabel), "Should match pods with gateway class label")

	// Test that it doesn't match pods without gateway class label
	podWithoutLabel := labels.Set{
		"app": "myapp",
	}
	assert.False(t, selector.Matches(podWithoutLabel), "Should not match pods without gateway class label")
}
