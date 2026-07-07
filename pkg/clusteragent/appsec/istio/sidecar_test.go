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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

func TestSidecarPattern_MutatePod_Success(t *testing.T) {
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

	outcome, err := pattern.MutatePod(pod, "default", pattern.client)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, appsecconfig.MutationMutated, outcome, "Pod should be modified")

	// Check that sidecar container was added
	require.Len(t, pod.Spec.Containers, 2, "Should have original container + sidecar")
	sidecarContainer := pod.Spec.Containers[1]
	assert.Equal(t, sidecarContainerName, sidecarContainer.Name, "Sidecar should have correct name")

	// Check that EnvoyFilter was created
	assert.NotNil(t, createdFilter, "EnvoyFilter should be created")
}

func TestSidecarPattern_MutatePod_AlreadyInjected(t *testing.T) {
	pattern := newTestIstioSidecarPattern(t)

	pod := newTestPodForSidecar("test-pod", "default", "istio")
	// Add sidecar container to simulate already injected pod
	pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
		Name:  sidecarContainerName,
		Image: "datadog/appsec-processor:latest",
	})

	// Execute
	outcome, err := pattern.MutatePod(pod, "default", pattern.client)

	// Verify
	require.Error(t, err)
	assert.Equal(t, appsecconfig.MutationSkipped, outcome, "Should skip pod that already has sidecar")
	var skipReason *appsecconfig.MutationSkippedReason
	require.ErrorAs(t, err, &skipReason)
	assert.Equal(t, appsecconfig.SkipReasonAlreadySidecar, skipReason.Reason)
	assert.Len(t, pod.Spec.Containers, 2, "Should still have 2 containers")
}

func TestSidecarPattern_MutatePod_GatewayClassNotFound(t *testing.T) {
	pattern := newTestIstioSidecarPattern(t)

	pod := newTestPodForSidecar("test-pod", "default", "missing-gatewayclass")

	// Setup mock client to return NotFound error
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "gatewayclasses", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, k8serrors.NewNotFound(schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "gatewayclasses"}, "missing-gatewayclass")
	})

	// Execute
	outcome, err := pattern.MutatePod(pod, "default", pattern.client)

	// Verify
	require.Error(t, err)
	assert.Equal(t, appsecconfig.MutationError, outcome)
	assert.Contains(t, err.Error(), "error getting gatewayclass")
}

func TestSidecarPattern_MutatePod_NonIstioGatewayClass(t *testing.T) {
	pattern := newTestIstioSidecarPattern(t)

	pod := newTestPodForSidecar("test-pod", "default", "envoy")
	gwClass := newTestGatewayClass("envoy", "envoy.io/gateway-controller")

	// Setup mock client to return non-Istio gateway class
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "gatewayclasses", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, gwClass, nil
	})

	isEligible := pattern.IsPodEligible(pod, "default")

	// Verify
	assert.False(t, isEligible, "Should not own pod for non-Istio gateway class")
}

func TestSidecarPattern_PodDeleted_IsNoOp(t *testing.T) {
	pattern := newTestIstioSidecarPattern(t)

	pod := newTestPodForSidecar("test-pod", "default", "istio")

	deleteCalled := false
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("delete", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteCalled = true
		return true, nil, nil
	})

	// Execute - should be a no-op
	outcome, err := pattern.PodDeleted(pod, "default", pattern.client)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, appsecconfig.MutationMutated, outcome)
	assert.False(t, deleteCalled, "PodDeleted should be a no-op")
}

func TestSidecarPattern_IsPodEligible(t *testing.T) {
	tests := []struct {
		name           string
		pod            *corev1.Pod
		setupMock      func(*dynamicfake.FakeDynamicClient)
		expectEligible bool
	}{
		{
			name: "eligible when pod has gateway class label and is istio",
			pod:  newTestPodForSidecar("test-pod", "default", "istio"),
			setupMock: func(client *dynamicfake.FakeDynamicClient) {
				gwClass := newTestGatewayClass("istio", istioGatewayControllerName)
				client.PrependReactor("get", "gatewayclasses", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, gwClass, nil
				})
			},
			expectEligible: true,
		},
		{
			name: "eligible when sidecar already exists on owned pod",
			pod: func() *corev1.Pod {
				pod := newTestPodForSidecar("test-pod", "default", "istio")
				pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
					Name:  sidecarContainerName,
					Image: "appsec:latest",
				})
				return pod
			}(),
			setupMock: func(client *dynamicfake.FakeDynamicClient) {
				gwClass := newTestGatewayClass("istio", istioGatewayControllerName)
				client.PrependReactor("get", "gatewayclasses", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, gwClass, nil
				})
			},
			expectEligible: true,
		},
		{
			name: "not eligible when gateway class not found",
			pod:  newTestPodForSidecar("test-pod", "default", "missing-gc"),
			setupMock: func(client *dynamicfake.FakeDynamicClient) {
				client.PrependReactor("get", "gatewayclasses", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, k8serrors.NewNotFound(schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "gatewayclasses"}, "missing-gc")
				})
			},
			expectEligible: false,
		},
		{
			name: "not eligible when gateway class is not istio",
			pod:  newTestPodForSidecar("test-pod", "default", "envoy"),
			setupMock: func(client *dynamicfake.FakeDynamicClient) {
				gwClass := newTestGatewayClass("envoy", "envoy.io/gateway-controller")
				client.PrependReactor("get", "gatewayclasses", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, gwClass, nil
				})
			},
			expectEligible: false,
		},
		{
			name: "not eligible when gateway class controller is malformed",
			pod:  newTestPodForSidecar("test-pod", "default", "malformed"),
			setupMock: func(client *dynamicfake.FakeDynamicClient) {
				gwClass := &unstructured.Unstructured{Object: map[string]any{
					"apiVersion": "gateway.networking.k8s.io/v1",
					"kind":       "GatewayClass",
					"metadata": map[string]any{
						"name": "malformed",
					},
					"spec": map[string]any{
						"controllerName": []any{"not-a-string"},
					},
				}}
				client.PrependReactor("get", "gatewayclasses", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, gwClass, nil
				})
			},
			expectEligible: false,
		},
		{
			name: "not eligible when pod has no gateway class label",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels:    map[string]string{},
				},
			},
			setupMock: func(_ *dynamicfake.FakeDynamicClient) {
				// Should not call get since label is missing
			},
			expectEligible: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := newTestIstioSidecarPattern(t)
			if tt.setupMock != nil {
				tt.setupMock(pattern.client.(*dynamicfake.FakeDynamicClient))
			}

			result := pattern.IsPodEligible(tt.pod, tt.pod.Namespace)

			assert.Equal(t, tt.expectEligible, result)
		})
	}
}

func TestSidecarPattern_MatchCondition(t *testing.T) {
	pattern := newTestIstioSidecarPattern(t)

	condition := pattern.MatchCondition()

	// Verify the condition checks for gateway class name label
	assert.NotEmpty(t, condition.Expression)
	assert.Contains(t, condition.Expression, gatewayClassNamePodLabel, "Expression should check for gateway class label")
	assert.Contains(t, condition.Expression, "object.metadata.labels", "Expression should reference object metadata labels")

	t.Logf("Generated CEL expression: %s", condition.Expression)
}

func TestSidecarPattern_MutatePod_EnvoyFilterCreationFailure(t *testing.T) {
	pattern := newTestIstioSidecarPattern(t)

	pod := newTestPodForSidecar("test-pod", "default", "istio")
	gwClass := newTestGatewayClass("istio", istioGatewayControllerName)

	// Setup mock client to return the gateway class
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "gatewayclasses", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, gwClass, nil
	})

	// Make EnvoyFilter creation fail
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "envoyfilters", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, k8serrors.NewInternalError(assert.AnError)
	})

	// Execute
	outcome, err := pattern.MutatePod(pod, "default", pattern.client)

	// Verify
	require.Error(t, err)
	assert.Equal(t, appsecconfig.MutationError, outcome)
	assert.Contains(t, err.Error(), "could not create Envoy Filter")
}

func TestSidecarPattern_MutatePod_IdempotentEnvoyFilterCreation(t *testing.T) {
	pattern := newTestIstioSidecarPattern(t)

	pod1 := newTestPodForSidecar("test-pod-1", "default", "istio")
	pod2 := newTestPodForSidecar("test-pod-2", "default", "istio")
	gwClass := newTestGatewayClass("istio", istioGatewayControllerName)

	createCallCount := 0

	// Setup mock client
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "gatewayclasses", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, gwClass, nil
	})

	var createdFilter *unstructured.Unstructured
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "envoyfilters", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createCallCount++
		if createCallCount == 1 {
			createAction := action.(k8stesting.CreateAction)
			createdFilter = createAction.GetObject().(*unstructured.Unstructured)
			return true, createdFilter, nil
		}
		// Second call should get AlreadyExists error
		return true, nil, k8serrors.NewAlreadyExists(schema.GroupResource{Group: "networking.istio.io", Resource: "envoyfilters"}, "appsec-extproc")
	})

	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "envoyfilters", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		if createdFilter != nil {
			return true, createdFilter, nil
		}
		return true, nil, k8serrors.NewNotFound(schema.GroupResource{Group: "networking.istio.io", Resource: "envoyfilters"}, "appsec-extproc")
	})

	// First pod injection - creates EnvoyFilter
	outcome1, err1 := pattern.MutatePod(pod1, "default", pattern.client)
	require.NoError(t, err1)
	assert.Equal(t, appsecconfig.MutationMutated, outcome1)
	assert.Equal(t, 1, createCallCount, "EnvoyFilter should be created once")

	// Second pod injection - EnvoyFilter already exists
	outcome2, err2 := pattern.MutatePod(pod2, "default", pattern.client)
	require.NoError(t, err2)
	assert.Equal(t, appsecconfig.MutationMutated, outcome2)
	// With current implementation, Added() will try to create again and handle AlreadyExists
}

func TestSidecarPattern_MutatePod_ContainerInjection(t *testing.T) {
	pattern := newTestIstioSidecarPattern(t)

	// Set specific sidecar config
	pattern.config.Sidecar = appsecconfig.Sidecar{
		Image:                "ghcr.io/datadog/appsec:v1.0",
		ImageTag:             "latest",
		Port:                 8080,
		HealthPort:           8081,
		BodyParsingSizeLimit: "5000000",
		CPURequest:           "200m",
		MemoryRequest:        "256Mi",
		CPULimit:             "500m",
		MemoryLimit:          "512Mi",
	}

	pod := newTestPodForSidecar("test-pod", "default", "istio")
	gwClass := newTestGatewayClass("istio", istioGatewayControllerName)

	// Setup mock client
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "gatewayclasses", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, gwClass, nil
	})

	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "envoyfilters", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		return true, createAction.GetObject(), nil
	})

	// Execute
	outcome, err := pattern.MutatePod(pod, "default", pattern.client)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, appsecconfig.MutationMutated, outcome)

	// Verify sidecar container was injected with correct config
	require.Len(t, pod.Spec.Containers, 2, "Should have original + sidecar container")

	sidecar := pod.Spec.Containers[1]
	assert.Equal(t, sidecarContainerName, sidecar.Name)
	assert.Equal(t, "ghcr.io/datadog/appsec:v1.0:latest", sidecar.Image)

	// Verify ports
	require.Len(t, sidecar.Ports, 2)
	assert.Equal(t, int32(8080), sidecar.Ports[0].ContainerPort)
	assert.Equal(t, int32(8081), sidecar.Ports[1].ContainerPort)

	// Verify health probe
	require.NotNil(t, sidecar.LivenessProbe)
	require.NotNil(t, sidecar.LivenessProbe.HTTPGet)
	assert.Equal(t, int32(8081), sidecar.LivenessProbe.HTTPGet.Port.IntVal)

	// Verify resources
	assert.Equal(t, "200m", sidecar.Resources.Requests.Cpu().String())
	assert.Equal(t, "256Mi", sidecar.Resources.Requests.Memory().String())
	assert.Equal(t, "500m", sidecar.Resources.Limits.Cpu().String())
	assert.Equal(t, "512Mi", sidecar.Resources.Limits.Memory().String())

	// Verify env vars
	found := false
	for _, env := range sidecar.Env {
		if env.Name == "DD_APPSEC_BODY_PARSING_SIZE_LIMIT" {
			assert.Equal(t, "5000000", env.Value)
			found = true
			break
		}
	}
	assert.True(t, found, "Should have DD_APPSEC_BODY_PARSING_SIZE_LIMIT env var")
}
