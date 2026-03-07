// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package envoygateway

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

// newTestEnvoyGatewayPattern creates a test instance with mocked dependencies
func newTestEnvoyGatewayPattern(_ *testing.T, client dynamic.Interface, logger log.Component, config appsecconfig.Config) *envoyGatewayInjectionPattern {
	recorder := record.NewFakeRecorder(100)
	eventRec := eventRecorder{recorder: recorder}

	return &envoyGatewayInjectionPattern{
		client:        client,
		logger:        logger,
		config:        config,
		eventRecorder: eventRec,
	}
}

// newTestGateway creates a test Gateway unstructured object with listeners
func newTestGateway(namespace, name string, listeners ...map[string]any) *unstructured.Unstructured {
	if len(listeners) == 0 {
		listeners = []map[string]any{
			{"name": "http", "protocol": "HTTP", "port": int64(80)},
		}
	}

	// Convert to []any for unstructured
	listenersAny := make([]any, len(listeners))
	for i, l := range listeners {
		listenersAny[i] = l
	}

	gateway := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "Gateway",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"gatewayClassName": "eg",
				"listeners":        listenersAny,
			},
		},
	}
	return gateway
}

// newTestGatewayClass creates a test GatewayClass unstructured object
func newTestGatewayClass(name, controllerName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "GatewayClass",
			"metadata": map[string]any{
				"name": name,
			},
			"spec": map[string]any{
				"controllerName": controllerName,
			},
		},
	}
}

// setupEnvoyGatewayClassReactor adds a reactor that returns the Envoy Gateway GatewayClass
func setupEnvoyGatewayClassReactor(client *dynamicfake.FakeDynamicClient) {
	egClass := newTestGatewayClass("eg", envoyGatewayControllerName)
	client.PrependReactor("get", "gatewayclasses", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		getAction := action.(k8stesting.GetAction)
		if getAction.GetName() == "eg" {
			return true, egClass, nil
		}
		return false, nil, nil
	})
}

// newTestPatchPolicy creates a test EnvoyPatchPolicy unstructured object
func newTestPatchPolicy(namespace, gatewayName string) *unstructured.Unstructured {
	policy := &unstructured.Unstructured{}
	policy.SetAPIVersion("gateway.envoyproxy.io/v1alpha1")
	policy.SetKind("EnvoyPatchPolicy")
	policy.SetNamespace(namespace)
	policy.SetName(patchPolicyName(gatewayName))
	return policy
}

// newTestDynamicClient creates a fake dynamic client with all required list kinds registered
func newTestDynamicClient(scheme *runtime.Scheme, objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			oldExtensionGVR: "EnvoyExtensionPolicyList",
		},
		objects...,
	)
}

func defaultTestConfig() appsecconfig.Config {
	return appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
		Injection: appsecconfig.Injection{
			CommonLabels:      map[string]string{"app": "datadog"},
			CommonAnnotations: map[string]string{"managed-by": "datadog"},
		},
	}
}

func TestAdded_SuccessfulCreation(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := newTestDynamicClient(scheme)
	config := defaultTestConfig()

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")
	setupEnvoyGatewayClassReactor(client)

	// Track created resources
	var createdPolicy *unstructured.Unstructured

	client.PrependReactor("create", "envoypatchpolicies", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdPolicy = createAction.GetObject().(*unstructured.Unstructured)
		return false, createdPolicy, nil
	})

	// Execute
	err := pattern.Added(ctx, gateway)

	// Verify
	require.NoError(t, err)
	assert.NotNil(t, createdPolicy, "EnvoyPatchPolicy should be created")

	assert.Equal(t, "test-ns", createdPolicy.GetNamespace())
	assert.Equal(t, patchPolicyName("test-gateway"), createdPolicy.GetName())
	assert.Equal(t, "gateway.envoyproxy.io/v1alpha1", createdPolicy.GetAPIVersion())
	assert.Equal(t, "EnvoyPatchPolicy", createdPolicy.GetKind())
}

func TestAdded_MultipleListeners(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := newTestDynamicClient(scheme)
	config := defaultTestConfig()

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway",
		map[string]any{"name": "http", "protocol": "HTTP", "port": int64(80)},
		map[string]any{"name": "https", "protocol": "HTTPS", "port": int64(443)},
	)
	setupEnvoyGatewayClassReactor(client)

	var createdPolicy *unstructured.Unstructured

	client.PrependReactor("create", "envoypatchpolicies", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdPolicy = createAction.GetObject().(*unstructured.Unstructured)
		return false, createdPolicy, nil
	})

	// Execute
	err := pattern.Added(ctx, gateway)

	// Verify
	require.NoError(t, err)
	assert.NotNil(t, createdPolicy, "EnvoyPatchPolicy should be created")

	// Verify jsonPatches contains 1 cluster + 2 listener patches = 3 total
	jsonPatches, found, err := unstructured.NestedSlice(createdPolicy.Object, "spec", "jsonPatches")
	require.NoError(t, err)
	require.True(t, found)
	assert.Len(t, jsonPatches, 3, "Should have 1 cluster + 2 listener patches")
}

func TestAdded_TwoGateways(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := newTestDynamicClient(scheme)
	config := defaultTestConfig()

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway1 := newTestGateway("test-ns", "gateway-1")
	gateway2 := newTestGateway("test-ns", "gateway-2")
	setupEnvoyGatewayClassReactor(client)

	var policyCount int

	client.PrependReactor("create", "envoypatchpolicies", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		policyCount++
		return false, createAction.GetObject(), nil
	})

	// Execute
	err1 := pattern.Added(ctx, gateway1)
	err2 := pattern.Added(ctx, gateway2)

	// Verify - each gateway should get its own policy
	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.Equal(t, 2, policyCount, "Should create 2 separate EnvoyPatchPolicies")
}

func TestAdded_NonEnvoyGatewaySkipped(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := newTestDynamicClient(scheme)
	config := defaultTestConfig()

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)

	// Gateway using an Istio GatewayClass
	gateway := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "Gateway",
			"metadata": map[string]any{
				"name":      "istio-gateway",
				"namespace": "default",
			},
			"spec": map[string]any{
				"gatewayClassName": "istio",
				"listeners": []any{
					map[string]any{"name": "http", "protocol": "HTTP", "port": int64(80)},
				},
			},
		},
	}

	// Setup mock: return an Istio GatewayClass
	istioClass := newTestGatewayClass("istio", "istio.io/gateway-controller")
	client.PrependReactor("get", "gatewayclasses", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, istioClass, nil
	})

	createCalled := false
	client.PrependReactor("create", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createCalled = true
		return false, nil, nil
	})

	err := pattern.Added(ctx, gateway)
	require.NoError(t, err)
	assert.False(t, createCalled, "Should not create EnvoyPatchPolicy for non-Envoy Gateway gateways")
}

func TestAdded_PolicyAlreadyExists(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	// Pre-create the EnvoyPatchPolicy
	existingPolicy := newTestPatchPolicy("test-ns", "test-gateway")
	client := newTestDynamicClient(scheme, existingPolicy)
	config := defaultTestConfig()

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")
	setupEnvoyGatewayClassReactor(client)

	// Execute - Create is called but returns AlreadyExists, which is handled gracefully
	err := pattern.Added(ctx, gateway)

	// Verify - should succeed (AlreadyExists is not an error)
	require.NoError(t, err)
}

func TestAdded_PolicyCreationError(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := newTestDynamicClient(scheme)
	config := defaultTestConfig()

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")
	setupEnvoyGatewayClassReactor(client)

	// Simulate error when creating EnvoyPatchPolicy
	client.PrependReactor("create", "envoypatchpolicies", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("failed to create policy")
	})

	// Execute
	err := pattern.Added(ctx, gateway)

	// Verify - should return error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not create EnvoyPatchPolicy")
}

func TestDeleted_SuccessfulDeletion(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	// Pre-create the EnvoyPatchPolicy
	existingPolicy := newTestPatchPolicy("test-ns", "test-gateway")
	client := newTestDynamicClient(scheme, existingPolicy)
	config := defaultTestConfig()

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")

	deletedResources := []string{}

	client.PrependReactor("delete", "envoypatchpolicies", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteAction := action.(k8stesting.DeleteAction)
		deletedResources = append(deletedResources, deleteAction.GetName())
		return false, nil, nil
	})

	// Execute
	err := pattern.Deleted(ctx, gateway)

	// Verify
	require.NoError(t, err)
	assert.Contains(t, deletedResources, patchPolicyName("test-gateway"), "EnvoyPatchPolicy should be deleted")
}

func TestDeleted_PolicyAlreadyDeleted(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := newTestDynamicClient(scheme) // No existing policy
	config := defaultTestConfig()

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")

	deleteCalled := false
	client.PrependReactor("delete", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteCalled = true
		return false, nil, nil
	})

	// Execute
	err := pattern.Deleted(ctx, gateway)

	// Verify - should succeed without attempting deletion
	require.NoError(t, err)
	assert.False(t, deleteCalled, "Should not attempt to delete when policy doesn't exist")
}

func TestDeleted_GetPolicyError(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := newTestDynamicClient(scheme)
	config := defaultTestConfig()

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")

	// Simulate a non-NotFound error when checking for existing policy
	client.PrependReactor("get", "envoypatchpolicies", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("API server error")
	})

	// Execute
	err := pattern.Deleted(ctx, gateway)

	// Verify - should return error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not check if EnvoyPatchPolicy was already deleted")
}

func TestDeleted_PolicyDeletionError(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	// Pre-create the EnvoyPatchPolicy
	existingPolicy := newTestPatchPolicy("test-ns", "test-gateway")
	client := newTestDynamicClient(scheme, existingPolicy)
	config := defaultTestConfig()

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")

	// Simulate error when deleting policy
	client.PrependReactor("delete", "envoypatchpolicies", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("failed to delete policy")
	})

	// Execute
	err := pattern.Deleted(ctx, gateway)

	// Verify - should return error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete policy")
}

func TestExtractListeners(t *testing.T) {
	tests := []struct {
		name      string
		gateway   *unstructured.Unstructured
		expected  []gatewayListener
		expectErr bool
	}{
		{
			name:    "single HTTP listener",
			gateway: newTestGateway("ns", "gw", map[string]any{"name": "http", "protocol": "HTTP", "port": int64(80)}),
			expected: []gatewayListener{
				{name: "http", protocol: "HTTP"},
			},
		},
		{
			name: "HTTP and HTTPS listeners",
			gateway: newTestGateway("ns", "gw",
				map[string]any{"name": "http", "protocol": "HTTP", "port": int64(80)},
				map[string]any{"name": "https", "protocol": "HTTPS", "port": int64(443)},
			),
			expected: []gatewayListener{
				{name: "http", protocol: "HTTP"},
				{name: "https", protocol: "HTTPS"},
			},
		},
		{
			name: "no listeners",
			gateway: &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{
						"listeners": []any{},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "missing protocol defaults to HTTP",
			gateway: &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{
						"listeners": []any{
							map[string]any{"name": "default"},
						},
					},
				},
			},
			expected: []gatewayListener{
				{name: "default", protocol: "HTTP"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listeners, err := extractListeners(tt.gateway)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, listeners)
			}
		})
	}
}

func TestFilterChainPath(t *testing.T) {
	tests := []struct {
		protocol string
		expected string
	}{
		{"HTTP", "/default_filter_chain"},
		{"HTTPS", "/filter_chains/0"},
		{"TLS", "/filter_chains/0"},
		{"TCP", "/default_filter_chain"},
		{"", "/default_filter_chain"},
	}

	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			assert.Equal(t, tt.expected, filterChainPath(tt.protocol))
		})
	}
}

func TestBuildClusterPatch(t *testing.T) {
	patch, err := buildClusterPatch("appsec.datadog.svc", 8080)
	require.NoError(t, err)

	assert.Equal(t, "type.googleapis.com/envoy.config.cluster.v3.Cluster", string(patch.Type))
	assert.Equal(t, clusterName, patch.Name)
	assert.Equal(t, "add", string(patch.Operation.Op))
	assert.Equal(t, "", *patch.Operation.Path)
	assert.NotNil(t, patch.Operation.Value)
}

func TestBuildHTTPFilterPatch(t *testing.T) {
	tests := []struct {
		name         string
		listenerName string
		protocol     string
		expectedPath string
	}{
		{
			name:         "HTTP listener",
			listenerName: "ns/gw/http",
			protocol:     "HTTP",
			expectedPath: "/default_filter_chain/filters/0/typed_config/http_filters/0",
		},
		{
			name:         "HTTPS listener",
			listenerName: "ns/gw/https",
			protocol:     "HTTPS",
			expectedPath: "/filter_chains/0/filters/0/typed_config/http_filters/0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patch, err := buildHTTPFilterPatch(tt.listenerName, tt.protocol)
			require.NoError(t, err)

			assert.Equal(t, "type.googleapis.com/envoy.config.listener.v3.Listener", string(patch.Type))
			assert.Equal(t, tt.listenerName, patch.Name)
			assert.Equal(t, "add", string(patch.Operation.Op))
			assert.Equal(t, tt.expectedPath, *patch.Operation.Path)
			assert.NotNil(t, patch.Operation.Value)
		})
	}
}

func TestPatchPolicyName(t *testing.T) {
	assert.Equal(t, "datadog-appsec-my-gateway", patchPolicyName("my-gateway"))
	assert.Equal(t, "datadog-appsec-gw", patchPolicyName("gw"))
}

func TestCleanupOldResources(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	// Create old resources that should be cleaned up
	oldPolicy := &unstructured.Unstructured{}
	oldPolicy.SetAPIVersion("gateway.envoyproxy.io/v1alpha1")
	oldPolicy.SetKind("EnvoyExtensionPolicy")
	oldPolicy.SetNamespace("test-ns")
	oldPolicy.SetName(oldExtensionPolicyName)

	oldGrant := &unstructured.Unstructured{}
	oldGrant.SetAPIVersion("gateway.networking.k8s.io/v1beta1")
	oldGrant.SetKind("ReferenceGrant")
	oldGrant.SetNamespace("datadog")
	oldGrant.SetName(oldExtensionPolicyName)

	client := newTestDynamicClient(scheme, oldPolicy, oldGrant)
	config := defaultTestConfig()

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)

	var deletedResources []string
	client.PrependReactor("delete", "*", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteAction := action.(k8stesting.DeleteAction)
		resource := action.GetResource().Resource
		deletedResources = append(deletedResources, resource+"/"+deleteAction.GetName())
		return false, nil, nil
	})

	// Execute
	pattern.cleanupOldResources(ctx, "test-ns")

	// Verify both old resources are cleaned up
	assert.Contains(t, deletedResources, "envoyextensionpolicies/"+oldExtensionPolicyName)
	assert.Contains(t, deletedResources, "referencegrants/"+oldExtensionPolicyName)
}

func TestMode_External(t *testing.T) {
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := newTestDynamicClient(scheme)
	config := defaultTestConfig()
	config.Mode = appsecconfig.InjectionModeExternal

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	assert.Equal(t, appsecconfig.InjectionModeExternal, pattern.Mode())
}

func TestMode_Sidecar(t *testing.T) {
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := newTestDynamicClient(scheme)
	config := defaultTestConfig()
	config.Mode = appsecconfig.InjectionModeSidecar

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	assert.Equal(t, appsecconfig.InjectionModeSidecar, pattern.Mode())
}

func TestNew_ExternalMode(t *testing.T) {
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := newTestDynamicClient(scheme)
	recorder := record.NewFakeRecorder(100)
	config := defaultTestConfig()
	config.Mode = appsecconfig.InjectionModeExternal

	pattern := New(client, logger, config, recorder)
	_, isSidecar := pattern.(*envoyGatewaySidecarPattern)
	assert.False(t, isSidecar, "External mode should not return sidecar pattern")
}

func TestNew_SidecarMode(t *testing.T) {
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := newTestDynamicClient(scheme)
	recorder := record.NewFakeRecorder(100)
	config := defaultTestConfig()
	config.Mode = appsecconfig.InjectionModeSidecar

	pattern := New(client, logger, config, recorder)
	_, isSidecar := pattern.(*envoyGatewaySidecarPattern)
	assert.True(t, isSidecar, "Sidecar mode should return sidecar pattern")
}

func TestAdded_SidecarMode_ProcessorAddress(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := newTestDynamicClient(scheme)
	config := defaultTestConfig()
	config.Mode = appsecconfig.InjectionModeSidecar
	config.Sidecar.Port = 9090

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")
	setupEnvoyGatewayClassReactor(client)

	var createdPolicy *unstructured.Unstructured

	client.PrependReactor("create", "envoypatchpolicies", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdPolicy = createAction.GetObject().(*unstructured.Unstructured)
		return false, createdPolicy, nil
	})

	err := pattern.Added(ctx, gateway)
	require.NoError(t, err)
	assert.NotNil(t, createdPolicy, "EnvoyPatchPolicy should be created for sidecar mode")
}

func TestNewPatchPolicy_TargetRef(t *testing.T) {
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := newTestDynamicClient(scheme)
	config := defaultTestConfig()

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "my-gateway")

	policy, err := pattern.newPatchPolicy("test-ns", "my-gateway", gateway)
	require.NoError(t, err)

	assert.Equal(t, "gateway.networking.k8s.io", string(policy.Spec.TargetRef.Group))
	assert.Equal(t, "Gateway", string(policy.Spec.TargetRef.Kind))
	assert.Equal(t, "my-gateway", string(policy.Spec.TargetRef.Name))
	assert.Equal(t, "JSONPatch", string(policy.Spec.Type))
}

func TestAdded_GatewayWithNoListeners(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := newTestDynamicClient(scheme)
	config := defaultTestConfig()

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	setupEnvoyGatewayClassReactor(client)
	// Gateway with no listeners
	gateway := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "Gateway",
			"metadata": map[string]any{
				"name":      "test-gateway",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"gatewayClassName": "eg",
				"listeners":        []any{},
			},
		},
	}

	err := pattern.Added(ctx, gateway)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gateway has no listeners")
}

// Regression test: ensure two gateways in same namespace each get their own policy
func TestAdded_TwoGatewaysSameNamespace_IndependentPolicies(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			patchPolicyGVR:  "EnvoyPatchPolicyList",
			oldExtensionGVR: "EnvoyExtensionPolicyList",
		},
	)
	config := defaultTestConfig()

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gw1 := newTestGateway("test-ns", "gateway-1")
	gw2 := newTestGateway("test-ns", "gateway-2")
	setupEnvoyGatewayClassReactor(client)

	createdNames := []string{}
	client.PrependReactor("create", "envoypatchpolicies", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		obj := createAction.GetObject().(*unstructured.Unstructured)
		createdNames = append(createdNames, obj.GetName())
		return false, obj, nil
	})

	require.NoError(t, pattern.Added(ctx, gw1))
	require.NoError(t, pattern.Added(ctx, gw2))

	assert.Equal(t, []string{patchPolicyName("gateway-1"), patchPolicyName("gateway-2")}, createdNames)
}
