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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		grantManager: grantManager{
			client:            client,
			logger:            logger,
			eventRecorder:     eventRec,
			serviceName:       config.Processor.ServiceName,
			namespace:         config.Processor.Namespace,
			commonLabels:      config.CommonLabels,
			commonAnnotations: config.CommonAnnotations,
		},
	}
}

// newTestGateway creates a test Gateway unstructured object
func newTestGateway(namespace, name string) *unstructured.Unstructured {
	gateway := &unstructured.Unstructured{}
	gateway.SetAPIVersion("gateway.networking.k8s.io/v1")
	gateway.SetKind("Gateway")
	gateway.SetNamespace(namespace)
	gateway.SetName(name)
	return gateway
}

// newTestEnvoyExtensionPolicy creates a test EnvoyExtensionPolicy unstructured object
func newTestEnvoyExtensionPolicy(namespace string) *unstructured.Unstructured {
	policy := &unstructured.Unstructured{}
	policy.SetAPIVersion("gateway.envoyproxy.io/v1alpha1")
	policy.SetKind("EnvoyExtensionPolicy")
	policy.SetNamespace(namespace)
	policy.SetName(extProcName)
	return policy
}

// newTestReferenceGrant creates a test ReferenceGrant unstructured object
func newTestReferenceGrant(namespace, fromNamespace string) *unstructured.Unstructured {
	grant := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1beta1",
			"kind":       "ReferenceGrant",
			"metadata": map[string]any{
				"name":      referenceGrantName,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"from": []any{
					map[string]any{
						"group":     "gateway.envoyproxy.io",
						"kind":      "EnvoyExtensionPolicy",
						"namespace": fromNamespace,
					},
				},
				"to": []any{
					map[string]any{
						"kind": "Service",
						"name": "appsec-processor",
					},
				},
			},
		},
	}
	return grant
}

func newTestBackend(namespace, socketPath string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.envoyproxy.io/v1alpha1",
		"kind":       "Backend",
		"metadata": map[string]any{
			"name":      extProcName,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"endpoints": []any{
				map[string]any{
					"unix": map[string]any{"path": socketPath},
				},
			},
		},
	}}
}

func envoyGatewayListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		gatewayGVR:        "GatewayList",
		extensionGVR:      "EnvoyExtensionPolicyList",
		referenceGrantGVR: "ReferenceGrantList",
		backendGVR:        "BackendList",
	}
}

func TestAdded_SuccessfulCreation(t *testing.T) {
	ctx := context.Background()
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
			CommonLabels:      map[string]string{"app": "datadog"},
			CommonAnnotations: map[string]string{"managed-by": "datadog"},
		},
	}

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")

	// Track created resources
	var createdPolicy *unstructured.Unstructured
	var createdGrant *unstructured.Unstructured

	client.PrependReactor("create", "envoyextensionpolicies", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdPolicy = createAction.GetObject().(*unstructured.Unstructured)
		return false, createdPolicy, nil
	})

	client.PrependReactor("create", "referencegrants", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdGrant = createAction.GetObject().(*unstructured.Unstructured)
		return false, createdGrant, nil
	})

	// Execute
	err := pattern.Added(ctx, gateway)

	// Verify
	require.NoError(t, err)
	assert.NotNil(t, createdPolicy, "EnvoyExtensionPolicy should be created")
	assert.NotNil(t, createdGrant, "ReferenceGrant should be created")

	assert.Equal(t, "test-ns", createdPolicy.GetNamespace())
	assert.Equal(t, extProcName, createdPolicy.GetName())
	assert.Equal(t, "gateway.envoyproxy.io/v1alpha1", createdPolicy.GetAPIVersion())

	assert.Equal(t, config.Processor.Namespace, createdGrant.GetNamespace())
	assert.Equal(t, referenceGrantName, createdGrant.GetName())
}

func TestAdded_SidecarCreatesBackendAndPolicyWithoutReferenceGrant(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, envoyGatewayListKinds())

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Mode: appsecconfig.InjectionModeSidecar,
			Sidecar: appsecconfig.Sidecar{
				UDSPath: "/var/run/datadog/extproc.sock",
			},
		},
	}

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")

	var referenceGrantActions int
	client.PrependReactor("*", "referencegrants", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		if action.Matches("create", "referencegrants") || action.Matches("patch", "referencegrants") || action.Matches("delete", "referencegrants") {
			referenceGrantActions++
		}
		return false, nil, nil
	})

	require.NoError(t, pattern.Added(ctx, gateway))

	backend, err := client.Resource(backendGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.NoError(t, err)
	endpoints, found, err := unstructured.NestedSlice(backend.Object, "spec", "endpoints")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, endpoints, 1)
	endpoint, ok := endpoints[0].(map[string]any)
	require.True(t, ok)
	uds, ok := endpoint["unix"].(map[string]any)
	require.True(t, ok)
	udsPath, ok := uds["path"].(string)
	require.True(t, ok)
	assert.Equal(t, "/var/run/datadog/extproc.sock", udsPath)

	policy, err := client.Resource(extensionGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.NoError(t, err)
	extProcs, found, err := unstructured.NestedSlice(policy.Object, "spec", "extProc")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, extProcs, 1)
	extProc, ok := extProcs[0].(map[string]any)
	require.True(t, ok)
	backendRefs, ok := extProc["backendRefs"].([]any)
	require.True(t, ok)
	require.Len(t, backendRefs, 1)
	backendRef, ok := backendRefs[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "gateway.envoyproxy.io", backendRef["group"])
	assert.Equal(t, "Backend", backendRef["kind"])
	assert.Equal(t, extProcName, backendRef["name"])
	assert.NotContains(t, backendRef, "namespace")
	assert.NotContains(t, backendRef, "port")

	grants, err := client.Resource(referenceGrantGVR).Namespace("datadog").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, grants.Items)
	assert.Zero(t, referenceGrantActions)
}

func TestAdded_ExternalRegressionCreatesServicePolicyAndReferenceGrant(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, envoyGatewayListKinds())

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Mode: appsecconfig.InjectionModeExternal,
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
	}

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	assert.Equal(t, appsecconfig.InjectionModeExternal, pattern.Mode())

	gateway := newTestGateway("test-ns", "test-gateway")
	require.NoError(t, pattern.Added(ctx, gateway))

	policy, err := client.Resource(extensionGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.NoError(t, err)
	extProcs, found, err := unstructured.NestedSlice(policy.Object, "spec", "extProc")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, extProcs, 1)
	extProc, ok := extProcs[0].(map[string]any)
	require.True(t, ok)
	backendRefs, ok := extProc["backendRefs"].([]any)
	require.True(t, ok)
	require.Len(t, backendRefs, 1)
	backendRef, ok := backendRefs[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "appsec-processor", backendRef["name"])
	assert.Equal(t, "datadog", backendRef["namespace"])
	assert.EqualValues(t, 8080, backendRef["port"])
	assert.NotContains(t, backendRef, "group")
	assert.NotContains(t, backendRef, "kind")

	grant, err := client.Resource(referenceGrantGVR).Namespace("datadog").Get(ctx, referenceGrantName, metav1.GetOptions{})
	require.NoError(t, err)
	from, found, err := unstructured.NestedSlice(grant.Object, "spec", "from")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, from, 1)
	fromRef, ok := from[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test-ns", fromRef["namespace"])
}

func TestAdded_SuccessfulCreationSecondGateway(t *testing.T) {
	ctx := context.Background()
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
			CommonLabels:      map[string]string{"app": "datadog"},
			CommonAnnotations: map[string]string{"managed-by": "datadog"},
		},
	}

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway1 := newTestGateway("test-ns1", "test-gateway")
	gateway2 := newTestGateway("test-ns2", "test-gateway")

	// Track created resources
	var createdPolicy *unstructured.Unstructured
	var createdGrant *unstructured.Unstructured

	var countPolicyCreate int
	var countRefGrantCreate int
	var countRefGrantPatch int

	client.PrependReactor("create", "envoyextensionpolicies", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdPolicy = createAction.GetObject().(*unstructured.Unstructured)
		countPolicyCreate++
		return false, createdPolicy, nil
	})

	client.PrependReactor("create", "referencegrants", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdGrant = createAction.GetObject().(*unstructured.Unstructured)
		countRefGrantCreate++
		return false, createdGrant, nil
	})

	client.PrependReactor("patch", "referencegrants", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		countRefGrantPatch++
		return false, createdGrant, nil
	})

	// Execute
	err1 := pattern.Added(ctx, gateway1)
	err2 := pattern.Added(ctx, gateway2)

	// Verify
	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.NotNil(t, createdPolicy, "EnvoyExtensionPolicy should be created")
	assert.NotNil(t, createdGrant, "ReferenceGrant should be created")

	assert.Equal(t, "test-ns2", createdPolicy.GetNamespace())
	assert.Equal(t, extProcName, createdPolicy.GetName())
	assert.Equal(t, "gateway.envoyproxy.io/v1alpha1", createdPolicy.GetAPIVersion())

	assert.Equal(t, config.Processor.Namespace, createdGrant.GetNamespace())
	assert.Equal(t, referenceGrantName, createdGrant.GetName())

	assert.Equal(t, 2, countPolicyCreate)
	assert.Equal(t, 1, countRefGrantCreate)
	assert.Equal(t, 2, countRefGrantPatch)
}

func TestAdded_PolicyAlreadyExists(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	// Pre-create the EnvoyExtensionPolicy
	existingPolicy := newTestEnvoyExtensionPolicy("test-ns")
	client := dynamicfake.NewSimpleDynamicClient(scheme, existingPolicy)

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
	}

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")

	createCalled := false
	client.PrependReactor("create", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createCalled = true
		return false, nil, nil
	})

	// Execute
	err := pattern.Added(ctx, gateway)

	// Verify - should succeed without creating anything new
	require.NoError(t, err)
	assert.False(t, createCalled, "Should not attempt to create new resources when policy already exists")
}

func TestAdded_GetPolicyError(t *testing.T) {
	ctx := context.Background()
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
	}

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")

	// Simulate a non-NotFound error when checking for existing policy
	client.PrependReactor("get", "envoyextensionpolicies", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("API server error")
	})

	// Execute
	err := pattern.Added(ctx, gateway)

	// Verify - should return error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not check if Envoy extension policy already exists")
}

func TestAdded_ReferenceGrantCreationError(t *testing.T) {
	ctx := context.Background()
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
	}

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")

	// Simulate error when creating ReferenceGrant
	client.PrependReactor("create", "referencegrants", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("failed to create reference grant")
	})

	// Execute
	err := pattern.Added(ctx, gateway)

	// Verify - should return error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not ensure ReferenceGrant for namespace")
}

func TestAdded_PolicyCreationError(t *testing.T) {
	ctx := context.Background()
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
	}

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")

	// Simulate error when creating EnvoyExtensionPolicy
	client.PrependReactor("create", "envoyextensionpolicies", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("failed to create policy")
	})

	// Execute
	err := pattern.Added(ctx, gateway)

	// Verify - should return error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not create Envoy extension policy")
}

func TestDeleted_SuccessfulDeletion_AloneInNamespace(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	// Pre-create the EnvoyExtensionPolicy and ReferenceGrant
	existingPolicy := newTestEnvoyExtensionPolicy("test-ns")
	existingGrant := newTestReferenceGrant("datadog", "test-ns")

	// Create client with custom list kinds for Gateway resources
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}: "GatewayList",
		},
		existingPolicy,
		existingGrant,
	)

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
	}

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")

	deletedResources := []string{}

	client.PrependReactor("delete", "*", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteAction := action.(k8stesting.DeleteAction)
		deletedResources = append(deletedResources, deleteAction.GetName())
		return false, nil, nil
	})

	// Mock the List call to return only this gateway (alone in namespace)
	client.PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		list := &unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{*gateway},
		}
		return true, list, nil
	})

	// Execute
	err := pattern.Deleted(ctx, gateway)

	// Verify
	require.NoError(t, err)
	assert.Contains(t, deletedResources, extProcName, "EnvoyExtensionPolicy should be deleted")
}

func TestDeleted_SidecarDeletesBackendAndPolicyOnlyForLastGateway(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	gateway1 := newTestGateway("test-ns", "gateway-1")
	gateway2 := newTestGateway("test-ns", "gateway-2")
	existingPolicy := newTestEnvoyExtensionPolicy("test-ns")
	existingBackend := newTestBackend("test-ns", "/var/run/datadog/extproc.sock")
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		envoyGatewayListKinds(),
		existingPolicy,
		existingBackend,
	)

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Mode: appsecconfig.InjectionModeSidecar,
			Sidecar: appsecconfig.Sidecar{
				UDSPath: "/var/run/datadog/extproc.sock",
			},
		},
	}

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)

	var referenceGrantActions int
	client.PrependReactor("*", "referencegrants", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		if action.Matches("create", "referencegrants") || action.Matches("patch", "referencegrants") || action.Matches("delete", "referencegrants") {
			referenceGrantActions++
		}
		return false, nil, nil
	})

	currentGateways := []unstructured.Unstructured{*gateway1, *gateway2}
	client.PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		list := &unstructured.UnstructuredList{Items: currentGateways}
		return true, list, nil
	})

	require.NoError(t, pattern.Deleted(ctx, gateway1))
	_, err := client.Resource(extensionGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.NoError(t, err)
	_, err = client.Resource(backendGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.NoError(t, err)

	currentGateways = []unstructured.Unstructured{*gateway2}
	require.NoError(t, pattern.Deleted(ctx, gateway2))
	_, err = client.Resource(extensionGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err), "EnvoyExtensionPolicy should be deleted after the last gateway")
	_, err = client.Resource(backendGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err), "Backend should be deleted after the last gateway")

	grants, err := client.Resource(referenceGrantGVR).Namespace("datadog").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, grants.Items)
	assert.Zero(t, referenceGrantActions)
}

func TestDeleted_PolicyAlreadyDeleted(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme) // No existing policy

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
	}

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

func TestDeleted_NotAloneInNamespace(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	// Pre-create the EnvoyExtensionPolicy
	existingPolicy := newTestEnvoyExtensionPolicy("test-ns")

	// Create client with custom list kinds for Gateway resources
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}: "GatewayList",
		},
		existingPolicy,
	)

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
	}

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")
	otherGateway := newTestGateway("test-ns", "other-gateway")

	// Mock the List call to return multiple gateways (not alone)
	client.PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		list := &unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{*gateway, *otherGateway},
		}
		return true, list, nil
	})

	deleteCalled := false
	client.PrependReactor("delete", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteCalled = true
		return false, nil, nil
	})

	// Execute
	err := pattern.Deleted(ctx, gateway)

	// Verify - should succeed without deleting anything
	require.NoError(t, err)
	assert.False(t, deleteCalled, "Should not delete resources when not alone in namespace")
}

func TestDeleted_GetPolicyError(t *testing.T) {
	ctx := context.Background()
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
	}

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")

	// Simulate a non-NotFound error when checking for existing policy
	client.PrependReactor("get", "envoyextensionpolicies", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("API server error")
	})

	// Execute
	err := pattern.Deleted(ctx, gateway)

	// Verify - should return error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not check if Envoy extension policy was already deleted")
}

func TestDeleted_ListGatewaysError(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	// Pre-create the EnvoyExtensionPolicy
	existingPolicy := newTestEnvoyExtensionPolicy("test-ns")

	// Create client with custom list kinds for Gateway resources
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}: "GatewayList",
		},
		existingPolicy,
	)

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
	}

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")

	// Simulate error when listing gateways
	client.PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("failed to list gateways")
	})

	// Execute
	err := pattern.Deleted(ctx, gateway)

	// Verify - should return error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine if gateway is alone in namespace")
}

func TestDeleted_PolicyDeletionError(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	// Pre-create the EnvoyExtensionPolicy
	existingPolicy := newTestEnvoyExtensionPolicy("test-ns")

	// Create client with custom list kinds for Gateway resources
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}: "GatewayList",
		},
		existingPolicy,
	)

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
	}

	pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
	gateway := newTestGateway("test-ns", "test-gateway")

	// Mock the List call to return only this gateway (alone in namespace)
	client.PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		list := &unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{*gateway},
		}
		return true, list, nil
	})

	// Simulate error when deleting policy
	client.PrependReactor("delete", "envoyextensionpolicies", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("failed to delete policy")
	})

	// Execute
	err := pattern.Deleted(ctx, gateway)

	// Verify - should return error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete policy")
}

func TestIsInjectionPossible_SidecarChecksBackendCRD(t *testing.T) {
	allCRDsPresent := func(client *dynamicfake.FakeDynamicClient) {
		client.PrependReactor("get", "customresourcedefinitions", func(action k8stesting.Action) (bool, runtime.Object, error) {
			crd := &unstructured.Unstructured{}
			crd.SetName(action.(k8stesting.GetAction).GetName())
			return true, crd, nil
		})
	}

	tests := []struct {
		name      string
		setupMock func(*dynamicfake.FakeDynamicClient)
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "all CRDs present",
			setupMock: allCRDsPresent,
			wantErr:   false,
		},
		{
			name: "backend CRD missing",
			setupMock: func(client *dynamicfake.FakeDynamicClient) {
				client.PrependReactor("get", "customresourcedefinitions", func(action k8stesting.Action) (bool, runtime.Object, error) {
					name := action.(k8stesting.GetAction).GetName()
					if name == "backends.gateway.envoyproxy.io" {
						return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "apiextensions.k8s.io", Resource: "customresourcedefinitions"}, name)
					}
					crd := &unstructured.Unstructured{}
					crd.SetName(name)
					return true, crd, nil
				})
			},
			wantErr: true,
			errMsg:  "Backend CRD not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := logmock.New(t)
			client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
			config := appsecconfig.Config{Product: appsecconfig.Product{Mode: appsecconfig.InjectionModeSidecar}}
			pattern := newTestEnvoyGatewayPattern(t, client, logger, config)
			tt.setupMock(client)

			err := pattern.IsInjectionPossible(ctx)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
