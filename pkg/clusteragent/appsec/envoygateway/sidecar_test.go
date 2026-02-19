// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package envoygateway

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
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

const sidecarContainerName = "datadog-appsec-processor" // From sidecar package

func newTestSidecarPattern(t *testing.T) *envoyGatewaySidecarPattern {
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			podGVR:          "PodList",
			oldExtensionGVR: "EnvoyExtensionPolicyList",
		},
	)
	config := defaultTestConfig()
	config.Mode = appsecconfig.InjectionModeSidecar
	config.Sidecar = appsecconfig.Sidecar{
		Image:      "ghcr.io/datadog/appsec:v1.0",
		ImageTag:   "latest",
		Port:       8080,
		HealthPort: 8081,
	}

	basePattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	return &envoyGatewaySidecarPattern{
		envoyGatewayInjectionPattern: basePattern,
	}
}

func newTestPodForSidecar(name, namespace, gatewayName, gatewayNamespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				owningGatewayNameLabel:      gatewayName,
				owningGatewayNamespaceLabel: gatewayNamespace,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "envoy",
					Image: "envoyproxy/envoy:latest",
				},
			},
		},
	}
}

func TestSidecarPattern_Added_IsNoOp(t *testing.T) {
	ctx := context.Background()
	pattern := newTestSidecarPattern(t)

	gateway := newTestGateway("test-ns", "test-gateway")

	// Execute - should be a no-op
	err := pattern.Added(ctx, gateway)

	// Verify
	require.NoError(t, err)
}

func TestSidecarPattern_MutatePod_Success(t *testing.T) {
	pattern := newTestSidecarPattern(t)

	pod := newTestPodForSidecar("test-pod", "default", "test-gateway", "test-ns")
	gateway := newTestGateway("test-ns", "test-gateway")

	// Setup mock client to return the gateway and gateway class
	setupEnvoyGatewayClassReactor(pattern.client.(*dynamicfake.FakeDynamicClient))
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, gateway, nil
	})

	// Track created resources
	var createdPolicy *unstructured.Unstructured
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "envoypatchpolicies", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdPolicy = createAction.GetObject().(*unstructured.Unstructured)
		return true, createdPolicy, nil
	})

	// Execute
	modified, err := pattern.MutatePod(pod, "default", pattern.client)

	// Verify
	require.NoError(t, err)
	assert.True(t, modified, "Pod should be modified")

	// Check that sidecar container was added
	require.Len(t, pod.Spec.Containers, 2, "Should have original container + sidecar")
	sidecarContainer := pod.Spec.Containers[1]
	assert.Equal(t, sidecarContainerName, sidecarContainer.Name, "Sidecar should have correct name")

	// Check that EnvoyPatchPolicy was created
	assert.NotNil(t, createdPolicy, "EnvoyPatchPolicy should be created")
}

func TestSidecarPattern_MutatePod_AlreadyInjected(t *testing.T) {
	pattern := newTestSidecarPattern(t)

	pod := newTestPodForSidecar("test-pod", "default", "test-gateway", "test-ns")
	// Add sidecar container to simulate already injected pod
	pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
		Name:  sidecarContainerName,
		Image: "datadog/appsec-processor:latest",
	})

	// Execute ShouldMutatePod - webhook checks this first
	shouldMutate := pattern.ShouldMutatePod(pod)

	// Verify
	assert.False(t, shouldMutate, "Should not mutate pod that already has sidecar")
	assert.Len(t, pod.Spec.Containers, 2, "Should still have 2 containers")
}

func TestSidecarPattern_MutatePod_GatewayNotFound(t *testing.T) {
	pattern := newTestSidecarPattern(t)

	pod := newTestPodForSidecar("test-pod", "default", "missing-gw", "test-ns")

	// Setup mock client to return NotFound error
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.NewNotFound(schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "gateways"}, "missing-gw")
	})

	// Execute
	modified, err := pattern.MutatePod(pod, "default", pattern.client)

	// Verify
	require.Error(t, err)
	assert.False(t, modified)
	assert.Contains(t, err.Error(), "error getting gateway")
}

func TestSidecarPattern_PodDeleted_LastPod(t *testing.T) {
	pattern := newTestSidecarPattern(t)

	pod := newTestPodForSidecar("test-pod", "default", "test-gateway", "test-ns")
	gateway := newTestGateway("test-ns", "test-gateway")

	// Mock: list pods returns only this pod (last one)
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "pods", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{{Object: map[string]any{
				"metadata": map[string]any{"name": "test-pod", "namespace": "default"},
			}}},
		}, nil
	})

	// Mock: return the gateway
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, gateway, nil
	})

	// Execute - should trigger cleanup since it's the last pod
	_, err := pattern.PodDeleted(pod, "default", pattern.client)

	// Verify - no error (policy may not exist, that's fine)
	require.NoError(t, err)
}

func TestSidecarPattern_PodDeleted_NotLastPod(t *testing.T) {
	pattern := newTestSidecarPattern(t)

	pod := newTestPodForSidecar("test-pod", "default", "test-gateway", "test-ns")

	// Mock: list pods returns multiple pods (not last)
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("*", "*", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		t.Logf("Reactor called: verb=%s resource=%s", action.GetVerb(), action.GetResource().Resource)
		if action.GetVerb() == "list" && action.GetResource().Resource == "pods" {
			podLabels := map[string]string{
				owningGatewayNameLabel:      "test-gateway",
				owningGatewayNamespaceLabel: "test-ns",
			}
			pod1 := unstructured.Unstructured{}
			pod1.SetName("test-pod")
			pod1.SetNamespace("default")
			pod1.SetLabels(podLabels)
			pod2 := unstructured.Unstructured{}
			pod2.SetName("other-pod")
			pod2.SetNamespace("default")
			pod2.SetLabels(podLabels)
			return true, &unstructured.UnstructuredList{Items: []unstructured.Unstructured{pod1, pod2}}, nil
		}
		if action.GetVerb() == "list" {
			return true, &unstructured.UnstructuredList{}, nil
		}
		return false, nil, nil
	})

	// Execute - should not proceed to delete since other pods exist
	_, err := pattern.PodDeleted(pod, "default", pattern.client)

	require.NoError(t, err)
}

func TestSidecarPattern_ShouldMutatePod(t *testing.T) {
	tests := []struct {
		name               string
		pod                *corev1.Pod
		expectShouldMutate bool
	}{
		{
			name:               "should mutate when pod has gateway labels",
			pod:                newTestPodForSidecar("test-pod", "default", "gw", "ns"),
			expectShouldMutate: true,
		},
		{
			name: "should not mutate when sidecar already exists",
			pod: func() *corev1.Pod {
				pod := newTestPodForSidecar("test-pod", "default", "gw", "ns")
				pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
					Name:  sidecarContainerName,
					Image: "appsec:latest",
				})
				return pod
			}(),
			expectShouldMutate: false,
		},
		{
			name: "should not mutate when pod has no gateway label",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels:    map[string]string{},
				},
			},
			expectShouldMutate: false,
		},
		{
			name: "should not mutate when gateway name label is empty",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels: map[string]string{
						owningGatewayNameLabel: "",
					},
				},
			},
			expectShouldMutate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := newTestSidecarPattern(t)
			result := pattern.ShouldMutatePod(tt.pod)
			assert.Equal(t, tt.expectShouldMutate, result)
		})
	}
}

func TestSidecarPattern_IsNamespaceEligible(t *testing.T) {
	pattern := newTestSidecarPattern(t)

	for _, ns := range []string{"default", "kube-system", "envoy-gateway-system", "datadog", ""} {
		t.Run("namespace_"+ns, func(t *testing.T) {
			result := pattern.IsNamespaceEligible(ns)
			assert.True(t, result, "All namespaces should be eligible")
		})
	}
}

func TestSidecarPattern_MatchCondition(t *testing.T) {
	pattern := newTestSidecarPattern(t)

	condition := pattern.MatchCondition()

	// Verify the condition checks for owning gateway name label
	assert.NotEmpty(t, condition.Expression)
	assert.Contains(t, condition.Expression, owningGatewayNameLabel, "Expression should check for owning gateway label")
	assert.Contains(t, condition.Expression, "object.metadata.labels", "Expression should reference object metadata labels")
}

func TestSidecarPattern_MutatePod_ContainerInjection(t *testing.T) {
	pattern := newTestSidecarPattern(t)

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

	pod := newTestPodForSidecar("test-pod", "default", "test-gateway", "test-ns")
	gateway := newTestGateway("test-ns", "test-gateway")

	// Setup mock client
	setupEnvoyGatewayClassReactor(pattern.client.(*dynamicfake.FakeDynamicClient))
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, gateway, nil
	})

	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "envoypatchpolicies", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		return true, createAction.GetObject(), nil
	})

	// Execute
	modified, err := pattern.MutatePod(pod, "default", pattern.client)

	// Verify
	require.NoError(t, err)
	assert.True(t, modified)

	// Verify sidecar container was injected with correct config
	require.Len(t, pod.Spec.Containers, 2, "Should have original + sidecar container")

	sc := pod.Spec.Containers[1]
	assert.Equal(t, sidecarContainerName, sc.Name)
	assert.Equal(t, "ghcr.io/datadog/appsec:v1.0:latest", sc.Image)

	// Verify ports
	require.Len(t, sc.Ports, 2)
	assert.Equal(t, int32(8080), sc.Ports[0].ContainerPort)
	assert.Equal(t, int32(8081), sc.Ports[1].ContainerPort)

	// Verify health probe
	require.NotNil(t, sc.LivenessProbe)
	require.NotNil(t, sc.LivenessProbe.HTTPGet)
	assert.Equal(t, int32(8081), sc.LivenessProbe.HTTPGet.Port.IntVal)

	// Verify resources
	assert.Equal(t, "200m", sc.Resources.Requests.Cpu().String())
	assert.Equal(t, "256Mi", sc.Resources.Requests.Memory().String())
	assert.Equal(t, "500m", sc.Resources.Limits.Cpu().String())
	assert.Equal(t, "512Mi", sc.Resources.Limits.Memory().String())

	// Verify env vars
	found := false
	for _, env := range sc.Env {
		if env.Name == "DD_APPSEC_BODY_PARSING_SIZE_LIMIT" {
			assert.Equal(t, "5000000", env.Value)
			found = true
			break
		}
	}
	assert.True(t, found, "Should have DD_APPSEC_BODY_PARSING_SIZE_LIMIT env var")
}

func TestSidecarPattern_MutatePod_IdempotentPolicyCreation(t *testing.T) {
	pattern := newTestSidecarPattern(t)

	pod1 := newTestPodForSidecar("test-pod-1", "default", "test-gateway", "test-ns")
	pod2 := newTestPodForSidecar("test-pod-2", "default", "test-gateway", "test-ns")
	gateway := newTestGateway("test-ns", "test-gateway")

	createCallCount := 0

	setupEnvoyGatewayClassReactor(pattern.client.(*dynamicfake.FakeDynamicClient))
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, gateway, nil
	})

	var createdPolicy *unstructured.Unstructured
	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "envoypatchpolicies", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createCallCount++
		if createCallCount == 1 {
			createAction := action.(k8stesting.CreateAction)
			createdPolicy = createAction.GetObject().(*unstructured.Unstructured)
			return true, createdPolicy, nil
		}
		return true, nil, errors.NewAlreadyExists(schema.GroupResource{Group: "gateway.envoyproxy.io", Resource: "envoypatchpolicies"}, patchPolicyName("test-gateway"))
	})

	pattern.client.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "envoypatchpolicies", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		if createdPolicy != nil {
			return true, createdPolicy, nil
		}
		return true, nil, errors.NewNotFound(schema.GroupResource{Group: "gateway.envoyproxy.io", Resource: "envoypatchpolicies"}, patchPolicyName("test-gateway"))
	})

	// First pod injection - creates EnvoyPatchPolicy
	modified1, err1 := pattern.MutatePod(pod1, "default", pattern.client)
	require.NoError(t, err1)
	assert.True(t, modified1)

	// Second pod injection - EnvoyPatchPolicy already exists
	modified2, err2 := pattern.MutatePod(pod2, "default", pattern.client)
	require.NoError(t, err2)
	assert.True(t, modified2)
}
