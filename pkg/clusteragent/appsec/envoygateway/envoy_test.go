// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package envoygateway

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

// newTestEnvoyGatewayPattern creates a test instance with mocked dependencies
func newTestEnvoyGatewayPattern(_ *testing.T, client dynamic.Interface, logger log.Component, config appsecconfig.Config) *envoyGatewayInjectionPattern {
	// Like production config, created resources carry the managed-by label
	labels := map[string]string{"app.kubernetes.io/managed-by": appsecconfig.ManagedByLabelValue}
	maps.Copy(labels, config.CommonLabels)
	config.CommonLabels = labels
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
	policy.SetLabels(map[string]string{"app.kubernetes.io/managed-by": appsecconfig.ManagedByLabelValue})
	return policy
}

// newTestReferenceGrant creates the per-namespace ReferenceGrant for fromNamespace
func newTestReferenceGrant(namespace, fromNamespace string) *unstructured.Unstructured {
	grant := newTestLegacyReferenceGrant(namespace, fromNamespace)
	grant.SetName(referenceGrantName(fromNamespace))
	return grant
}

// newTestLegacyReferenceGrant creates the shared ReferenceGrant older versions created, listing fromNamespaces
func newTestLegacyReferenceGrant(namespace string, fromNamespaces ...string) *unstructured.Unstructured {
	from := make([]any, 0, len(fromNamespaces))
	for _, ns := range fromNamespaces {
		from = append(from, map[string]any{
			"group":     "gateway.envoyproxy.io",
			"kind":      "EnvoyExtensionPolicy",
			"namespace": ns,
		})
	}
	grant := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1beta1",
			"kind":       "ReferenceGrant",
			"metadata": map[string]any{
				"name":      legacyReferenceGrantName,
				"namespace": namespace,
				"labels":    map[string]any{"app.kubernetes.io/managed-by": appsecconfig.ManagedByLabelValue},
			},
			"spec": map[string]any{
				"from": from,
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
			"labels":    map[string]any{"app.kubernetes.io/managed-by": appsecconfig.ManagedByLabelValue},
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
	assert.Equal(t, referenceGrantName("test-ns"), createdGrant.GetName())
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

	assert.Equal(t, []string{"test-ns"}, grantFromNamespaces(t, client, referenceGrantName("test-ns")))
}

// grantFromNamespaces returns the spec.from namespaces of the named ReferenceGrant in the processor namespace
func grantFromNamespaces(t *testing.T, client dynamic.Interface, name string) []string {
	t.Helper()
	grant, err := client.Resource(referenceGrantGVR).Namespace("datadog").Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)
	from, found, err := unstructured.NestedSlice(grant.Object, "spec", "from")
	require.NoError(t, err, "spec.from must stay a list")
	require.True(t, found)
	var namespaces []string
	for _, entry := range from {
		ref, ok := entry.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "gateway.envoyproxy.io", ref["group"])
		assert.Equal(t, "EnvoyExtensionPolicy", ref["kind"])
		ns, ok := ref["namespace"].(string)
		require.True(t, ok)
		namespaces = append(namespaces, ns)
	}
	return namespaces
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
	assert.Equal(t, referenceGrantName("test-ns2"), createdGrant.GetName())

	assert.Equal(t, 2, countPolicyCreate)
	assert.Equal(t, 2, countRefGrantCreate)
	assert.Equal(t, 0, countRefGrantPatch)
}

// The ReferenceGrant tests below check the stored objects rather than counting client calls: the
// fake client applies JSON patches like the API server, so a patch that corrupts spec.from shows up.

func TestAdded_GatewaysInManyNamespacesGetOneReferenceGrantEach(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

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

	// The fake client does not enforce the 16-entry spec.from limit, so check that each namespace gets its own grant
	var namespaces []string
	for i := range 20 {
		namespaces = append(namespaces, fmt.Sprintf("test-ns%d", i))
	}
	for _, ns := range namespaces {
		require.NoError(t, pattern.Added(ctx, newTestGateway(ns, "test-gateway")))
	}

	for _, ns := range namespaces {
		_, err := client.Resource(extensionGVR).Namespace(ns).Get(ctx, extProcName, metav1.GetOptions{})
		require.NoError(t, err, "EnvoyExtensionPolicy should exist in namespace %s", ns)
		assert.Equal(t, []string{ns}, grantFromNamespaces(t, client, referenceGrantName(ns)))
	}

	require.NoError(t, pattern.RemoveNamespaceToGrant(ctx, "test-ns2"))
	_, err := client.Resource(referenceGrantGVR).Namespace("datadog").Get(ctx, referenceGrantName("test-ns2"), metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err), "ReferenceGrant of test-ns2 should be deleted")
	assert.Equal(t, []string{"test-ns3"}, grantFromNamespaces(t, client, referenceGrantName("test-ns3")))
}

func TestRemoveNamespaceToGrant_DrainsLegacySharedGrant(t *testing.T) {
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), newTestLegacyReferenceGrant("datadog", "test-ns1", "test-ns2"))

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
	}
	pattern := newTestEnvoyGatewayPattern(t, client, logmock.New(t), config)

	require.NoError(t, pattern.RemoveNamespaceToGrant(ctx, "test-ns1"))
	assert.Equal(t, []string{"test-ns2"}, grantFromNamespaces(t, client, legacyReferenceGrantName))

	require.NoError(t, pattern.RemoveNamespaceToGrant(ctx, "test-ns3"))
	assert.Equal(t, []string{"test-ns2"}, grantFromNamespaces(t, client, legacyReferenceGrantName))

	require.NoError(t, pattern.RemoveNamespaceToGrant(ctx, "test-ns2"))
	_, err := client.Resource(referenceGrantGVR).Namespace("datadog").Get(ctx, legacyReferenceGrantName, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err), "legacy ReferenceGrant should be deleted with its last namespace")
}

func TestAdded_RetryAfterPolicyFailureKeepsSingleGrant(t *testing.T) {
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewayListKinds())

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
	}
	pattern := newTestEnvoyGatewayPattern(t, client, logmock.New(t), config)
	gateway1 := newTestGateway("test-ns1", "test-gateway")
	gateway2 := newTestGateway("test-ns2", "test-gateway")

	require.NoError(t, pattern.Added(ctx, gateway1))

	// The retry exercises the existing-grant path after policy creation fails
	failOnce := true
	client.PrependReactor("create", "envoyextensionpolicies", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		if failOnce && action.GetNamespace() == "test-ns2" {
			failOnce = false
			return true, nil, errors.New("transient error")
		}
		return false, nil, nil
	})
	require.Error(t, pattern.Added(ctx, gateway2))
	require.NoError(t, pattern.Added(ctx, gateway2))
	assert.Equal(t, []string{"test-ns2"}, grantFromNamespaces(t, client, referenceGrantName("test-ns2")))

	client.PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &unstructured.UnstructuredList{Items: []unstructured.Unstructured{*gateway2}}, nil
	})
	require.NoError(t, pattern.Deleted(ctx, gateway2))
	_, err := client.Resource(referenceGrantGVR).Namespace("datadog").Get(ctx, referenceGrantName("test-ns2"), metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err), "ReferenceGrant of test-ns2 should be deleted")
	assert.Equal(t, []string{"test-ns1"}, grantFromNamespaces(t, client, referenceGrantName("test-ns1")))
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
	existingGrant := newTestLegacyReferenceGrant("datadog", "test-ns")

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
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, envoyGatewayListKinds()) // No existing policy

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

// TestDeleted_OnlyDeletesResourcesCreatedByInjector verifies that deleting the last Gateway of a
// namespace leaves an EnvoyExtensionPolicy and ReferenceGrant the injector did not create in place,
// and still removes ones created before the managed-by label existed (component label only).
func TestDeleted_OnlyDeletesResourcesCreatedByInjector(t *testing.T) {
	for _, tc := range []struct {
		name    string
		labels  map[string]string
		deleted bool
	}{
		{name: "not created by the injector", labels: map[string]string{"app": "someone-else"}, deleted: false},
		{name: "created before the managed-by label", labels: map[string]string{"app.kubernetes.io/component": "datadog-appsec-injector"}, deleted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			policy := newTestEnvoyExtensionPolicy("test-ns")
			policy.SetLabels(tc.labels)
			grant := newTestReferenceGrant("datadog", "test-ns")
			grant.SetLabels(tc.labels)
			legacyGrant := newTestLegacyReferenceGrant("datadog", "test-ns")
			legacyGrant.SetLabels(tc.labels)
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewayListKinds(), policy, grant, legacyGrant)
			config := appsecconfig.Config{
				Product: appsecconfig.Product{
					Processor: appsecconfig.Processor{ServiceName: "appsec-processor", Namespace: "datadog", Port: 8080},
				},
			}
			pattern := newTestEnvoyGatewayPattern(t, client, logmock.New(t), config)

			// The deleted Gateway was the last one of the namespace
			require.NoError(t, pattern.Deleted(ctx, newTestGateway("test-ns", "test-gateway")))

			_, policyErr := client.Resource(extensionGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
			_, grantErr := client.Resource(referenceGrantGVR).Namespace("datadog").Get(ctx, referenceGrantName("test-ns"), metav1.GetOptions{})
			_, legacyGrantErr := client.Resource(referenceGrantGVR).Namespace("datadog").Get(ctx, legacyReferenceGrantName, metav1.GetOptions{})
			if tc.deleted {
				assert.True(t, apierrors.IsNotFound(policyErr), "EnvoyExtensionPolicy created by the injector must be deleted")
				assert.True(t, apierrors.IsNotFound(grantErr), "ReferenceGrant created by the injector must be deleted")
				assert.True(t, apierrors.IsNotFound(legacyGrantErr), "legacy ReferenceGrant created by the injector must be deleted")
			} else {
				require.NoError(t, policyErr, "EnvoyExtensionPolicy not created by the injector must be left in place")
				require.NoError(t, grantErr, "ReferenceGrant not created by the injector must be left in place")
				require.NoError(t, legacyGrantErr, "legacy ReferenceGrant not created by the injector must be left in place")
			}
		})
	}
}

func TestRemoveNamespaceToGrant_LeavesForeignGrant(t *testing.T) {
	ctx := context.Background()
	grant := newTestReferenceGrant("datadog", "test-ns")
	grant.SetLabels(map[string]string{"app": "someone-else"})
	legacyGrant := newTestLegacyReferenceGrant("datadog", "test-ns", "other-ns")
	legacyGrant.SetLabels(map[string]string{"app": "someone-else"})
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewayListKinds(), grant, legacyGrant)
	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{ServiceName: "appsec-processor", Namespace: "datadog", Port: 8080},
		},
	}
	pattern := newTestEnvoyGatewayPattern(t, client, logmock.New(t), config)

	require.NoError(t, pattern.RemoveNamespaceToGrant(ctx, "test-ns"))

	_, err := client.Resource(referenceGrantGVR).Namespace("datadog").Get(ctx, referenceGrantName("test-ns"), metav1.GetOptions{})
	require.NoError(t, err, "ReferenceGrant not created by the injector must be left in place")
	assert.Equal(t, []string{"test-ns", "other-ns"}, grantFromNamespaces(t, client, legacyReferenceGrantName), "legacy ReferenceGrant not created by the injector must be left untouched")
}

func TestDeleted_SidecarForeignPolicyStillDeletesOurBackend(t *testing.T) {
	ctx := context.Background()
	policy := newTestEnvoyExtensionPolicy("test-ns")
	policy.SetLabels(map[string]string{"app": "someone-else"})
	backend := newTestBackend("test-ns", "/var/run/datadog/extproc.sock")
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewayListKinds(), policy, backend)
	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Mode:    appsecconfig.InjectionModeSidecar,
			Sidecar: appsecconfig.Sidecar{UDSPath: "/var/run/datadog/extproc.sock"},
		},
	}
	pattern := newTestEnvoyGatewayPattern(t, client, logmock.New(t), config)

	// The deleted Gateway was the last one of the namespace
	require.NoError(t, pattern.Deleted(ctx, newTestGateway("test-ns", "test-gateway")))

	_, err := client.Resource(extensionGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.NoError(t, err, "EnvoyExtensionPolicy not created by the injector must be left in place")
	_, err = client.Resource(backendGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err), "Backend created by the injector must still be deleted")
}

func TestAddNamespaceToGrant_ForeignLegacyGrant(t *testing.T) {
	ctx := context.Background()
	legacyGrant := newTestLegacyReferenceGrant("datadog", "other-ns")
	legacyGrant.SetLabels(map[string]string{"app": "someone-else"})
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewayListKinds(), legacyGrant)
	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{ServiceName: "appsec-processor", Namespace: "datadog", Port: 8080},
		},
	}
	pattern := newTestEnvoyGatewayPattern(t, client, logmock.New(t), config)

	require.NoError(t, pattern.Added(ctx, newTestGateway("test-ns", "test-gateway")))

	_, err := client.Resource(extensionGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"test-ns"}, grantFromNamespaces(t, client, referenceGrantName("test-ns")))
	assert.Equal(t, []string{"other-ns"}, grantFromNamespaces(t, client, legacyReferenceGrantName), "legacy ReferenceGrant not created by the injector must be left untouched")
}

func TestAddNamespaceToGrant_ExistingGrantMustAuthorizeTheReference(t *testing.T) {
	from := func(namespace string) []any {
		return []any{map[string]any{"group": "gateway.envoyproxy.io", "kind": "EnvoyExtensionPolicy", "namespace": namespace}}
	}
	to := func(service map[string]any) []any { return []any{service} }
	for name, tc := range map[string]struct {
		from, to []any
		wantErr  bool
	}{
		"matching grant":               {from: from("test-ns"), to: to(map[string]any{"kind": "Service", "name": "appsec-processor"})},
		"grant for any Service":        {from: from("test-ns"), to: to(map[string]any{"kind": "Service"})},
		"grant from another namespace": {from: from("other-ns"), to: to(map[string]any{"kind": "Service", "name": "appsec-processor"}), wantErr: true},
		"grant to another Service":     {from: from("test-ns"), to: to(map[string]any{"kind": "Service", "name": "other-service"}), wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			grant := newTestReferenceGrant("datadog", "test-ns")
			grant.Object["spec"] = map[string]any{"from": tc.from, "to": tc.to}
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewayListKinds(), grant)
			config := appsecconfig.Config{
				Product: appsecconfig.Product{
					Processor: appsecconfig.Processor{ServiceName: "appsec-processor", Namespace: "datadog", Port: 8080},
				},
			}
			pattern := newTestEnvoyGatewayPattern(t, client, logmock.New(t), config)

			err := pattern.Added(ctx, newTestGateway("test-ns", "test-gateway"))

			_, getErr := client.Resource(extensionGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
			if tc.wantErr {
				require.Error(t, err)
				assert.True(t, apierrors.IsNotFound(getErr), "no EnvoyExtensionPolicy must be created when the grant does not authorize it")
			} else {
				require.NoError(t, err)
				require.NoError(t, getErr)
			}
		})
	}
}

func TestRemoveNamespaceToGrant_DeletesWithPreconditions(t *testing.T) {
	ctx := context.Background()
	grant := newTestReferenceGrant("datadog", "test-ns")
	grant.SetUID("grant-uid")
	grant.SetResourceVersion("11")
	legacyGrant := newTestLegacyReferenceGrant("datadog", "test-ns")
	legacyGrant.SetUID("legacy-grant-uid")
	legacyGrant.SetResourceVersion("22")
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewayListKinds(), grant, legacyGrant)
	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{ServiceName: "appsec-processor", Namespace: "datadog", Port: 8080},
		},
	}
	pattern := newTestEnvoyGatewayPattern(t, client, logmock.New(t), config)

	preconditions := map[string]metav1.Preconditions{}
	client.PrependReactor("delete", "referencegrants", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteAction := action.(k8stesting.DeleteAction)
		if p := deleteAction.GetDeleteOptions().Preconditions; p != nil {
			preconditions[deleteAction.GetName()] = *p
		}
		return false, nil, nil
	})

	require.NoError(t, pattern.RemoveNamespaceToGrant(ctx, "test-ns"))
	assert.Equal(t, map[string]metav1.Preconditions{
		referenceGrantName("test-ns"): {UID: ptr.To(types.UID("grant-uid")), ResourceVersion: ptr.To("11")},
		legacyReferenceGrantName:      {UID: ptr.To(types.UID("legacy-grant-uid")), ResourceVersion: ptr.To("22")},
	}, preconditions)
}

func TestDeleted_RetryCleansUpGrantAfterPolicyDeleted(t *testing.T) {
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewayListKinds(),
		newTestEnvoyExtensionPolicy("test-ns"), newTestReferenceGrant("datadog", "test-ns"))
	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{ServiceName: "appsec-processor", Namespace: "datadog", Port: 8080},
		},
	}
	pattern := newTestEnvoyGatewayPattern(t, client, logmock.New(t), config)
	gateway := newTestGateway("test-ns", "test-gateway")
	client.PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &unstructured.UnstructuredList{Items: []unstructured.Unstructured{*gateway}}, nil
	})
	failOnce := true
	client.PrependReactor("delete", "referencegrants", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		if failOnce {
			failOnce = false
			return true, nil, errors.New("transient error")
		}
		return false, nil, nil
	})

	require.Error(t, pattern.Deleted(ctx, gateway))
	_, err := client.Resource(extensionGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err), "the first attempt deletes the policy")

	require.NoError(t, pattern.Deleted(ctx, gateway))
	_, err = client.Resource(referenceGrantGVR).Namespace("datadog").Get(ctx, referenceGrantName("test-ns"), metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err), "the retry must delete the grant even though the policy is gone")
}

func TestAddNamespaceToGrant_RetryDoesNotRecreateExistingGrant(t *testing.T) {
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewayListKinds(), newTestReferenceGrant("datadog", "test-ns"))
	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{ServiceName: "appsec-processor", Namespace: "datadog", Port: 8080},
		},
	}
	pattern := newTestEnvoyGatewayPattern(t, client, logmock.New(t), config)
	gr := schema.GroupResource{Group: referenceGrantGVR.Group, Resource: referenceGrantGVR.Resource}
	client.PrependReactor("create", "referencegrants", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, apierrors.NewForbidden(gr, referenceGrantName("test-ns"), errors.New("exceeded quota: grants"))
	})

	require.NoError(t, pattern.Added(ctx, newTestGateway("test-ns", "test-gateway")))
	_, err := client.Resource(extensionGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
	require.NoError(t, err)
}

func TestAddNamespaceToGrant_TerminatingGrantIsRetried(t *testing.T) {
	ctx := context.Background()
	grant := newTestReferenceGrant("datadog", "test-ns")
	grant.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
	grant.SetFinalizers([]string{"example.com/finalizer"})
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewayListKinds(), grant)
	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{ServiceName: "appsec-processor", Namespace: "datadog", Port: 8080},
		},
	}
	pattern := newTestEnvoyGatewayPattern(t, client, logmock.New(t), config)

	addErr := pattern.Added(ctx, newTestGateway("test-ns", "test-gateway"))
	require.Error(t, addErr)
	assert.False(t, apierrors.IsForbidden(addErr) || apierrors.IsInvalid(addErr), "the injector drops Forbidden and Invalid errors, so a terminating grant must report a retryable error")
	_, err := client.Resource(extensionGVR).Namespace("test-ns").Get(ctx, extProcName, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err), "no EnvoyExtensionPolicy must rely on a grant being deleted")
}

func TestRemoveNamespaceToGrant_LegacyPatchIsGuardedByResourceVersion(t *testing.T) {
	ctx := context.Background()
	legacyGrant := newTestLegacyReferenceGrant("datadog", "test-ns1", "test-ns2", "test-ns3")
	legacyGrant.SetResourceVersion("42")
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), envoyGatewayListKinds(), legacyGrant)
	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{ServiceName: "appsec-processor", Namespace: "datadog", Port: 8080},
		},
	}
	pattern := newTestEnvoyGatewayPattern(t, client, logmock.New(t), config)

	var patch k8stesting.PatchAction
	client.PrependReactor("patch", "referencegrants", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		patch = action.(k8stesting.PatchAction)
		return true, legacyGrant, nil
	})

	require.NoError(t, pattern.RemoveNamespaceToGrant(ctx, "test-ns2"))
	require.NotNil(t, patch)
	assert.Equal(t, types.MergePatchType, patch.GetPatchType())
	assert.JSONEq(t, `{
		"metadata": {"resourceVersion": "42"},
		"spec": {"from": [
			{"group": "gateway.envoyproxy.io", "kind": "EnvoyExtensionPolicy", "namespace": "test-ns1"},
			{"group": "gateway.envoyproxy.io", "kind": "EnvoyExtensionPolicy", "namespace": "test-ns3"}
		]}
	}`, string(patch.GetPatch()))
}
