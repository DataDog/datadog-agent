// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package istio

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

// newTestIstioPattern creates a test instance with mocked dependencies
func newTestIstioPattern(client dynamic.Interface, logger log.Component, config appsecconfig.Config) *istioInjectionPattern {
	recorder := record.NewFakeRecorder(100)

	return &istioInjectionPattern{
		client: client,
		logger: logger,
		config: config,
		eventRecorder: eventRecorder{
			recorder: recorder,
		},
	}
}

// newTestGatewayClass creates a test GatewayClass unstructured object
func newTestGatewayClass(name, controllerName string) *unstructured.Unstructured {
	gwClass := &unstructured.Unstructured{
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
	return gwClass
}

// newTestEnvoyFilter creates a test EnvoyFilter unstructured object
func newTestEnvoyFilter(namespace string) *unstructured.Unstructured {
	filter := &unstructured.Unstructured{}
	filter.SetAPIVersion("networking.istio.io/v1alpha3")
	filter.SetKind("EnvoyFilter")
	filter.SetNamespace(namespace)
	filter.SetName(envoyFilterName)
	return filter
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
			IstioNamespace:    "istio-system",
			CommonLabels:      map[string]string{"app": "datadog"},
			CommonAnnotations: map[string]string{"managed-by": "datadog"},
		},
	}

	pattern := newTestIstioPattern(client, logger, config)
	gwClass := newTestGatewayClass("istio", istioGatewayControllerName)

	// Track created resources
	var createdFilter *unstructured.Unstructured

	client.PrependReactor("create", "envoyfilters", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdFilter = createAction.GetObject().(*unstructured.Unstructured)
		return false, createdFilter, nil
	})

	// Execute
	err := pattern.Added(ctx, gwClass)

	// Verify
	require.NoError(t, err)
	assert.NotNil(t, createdFilter, "EnvoyFilter should be created")
	assert.Equal(t, "istio-system", createdFilter.GetNamespace())
	assert.Equal(t, envoyFilterName, createdFilter.GetName())
	assert.Equal(t, "networking.istio.io/v1alpha3", createdFilter.GetAPIVersion())
}

func TestAdded_FilterAlreadyExists(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	// Pre-create the EnvoyFilter
	existingFilter := newTestEnvoyFilter("istio-system")
	client := dynamicfake.NewSimpleDynamicClient(scheme, existingFilter)

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
		Injection: appsecconfig.Injection{
			IstioNamespace: "istio-system",
		},
	}

	pattern := newTestIstioPattern(client, logger, config)
	gwClass := newTestGatewayClass("istio", istioGatewayControllerName)

	createCalled := false
	client.PrependReactor("create", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createCalled = true
		return false, nil, nil
	})

	// Execute
	err := pattern.Added(ctx, gwClass)

	// Verify - should succeed without creating anything new
	require.NoError(t, err)
	assert.False(t, createCalled, "Should not attempt to create new resources when filter already exists")
}

func TestAdded_NonIstioGatewayClass(t *testing.T) {
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
			IstioNamespace: "istio-system",
		},
	}

	pattern := newTestIstioPattern(client, logger, config)
	gwClass := newTestGatewayClass("envoy", "envoy.io/gateway-controller")

	createCalled := false
	client.PrependReactor("create", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createCalled = true
		return false, nil, nil
	})

	// Execute
	err := pattern.Added(ctx, gwClass)

	// Verify - should skip processing for non-Istio gateway
	require.NoError(t, err)
	assert.False(t, createCalled, "Should not create resources for non-Istio gateway")
}

func TestAdded_MissingControllerName(t *testing.T) {
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
			IstioNamespace: "istio-system",
		},
	}

	pattern := newTestIstioPattern(client, logger, config)

	// Gateway class without controllerName
	gwClass := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "GatewayClass",
			"metadata": map[string]any{
				"name": "test",
			},
			"spec": map[string]any{},
		},
	}

	// Execute
	err := pattern.Added(ctx, gwClass)

	// Verify - should return error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not get gateway controller name")
}

func TestAdded_GetFilterError(t *testing.T) {
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
			IstioNamespace: "istio-system",
		},
	}

	pattern := newTestIstioPattern(client, logger, config)
	gwClass := newTestGatewayClass("istio", istioGatewayControllerName)

	// Simulate a non-NotFound error when checking for existing filter
	client.PrependReactor("get", "envoyfilters", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("API server error")
	})

	// Execute
	err := pattern.Added(ctx, gwClass)

	// Verify - should return error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not check if Envoy Filter already exists")
}

func TestAdded_FilterCreationError(t *testing.T) {
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
			IstioNamespace: "istio-system",
		},
	}

	pattern := newTestIstioPattern(client, logger, config)
	gwClass := newTestGatewayClass("istio", istioGatewayControllerName)

	// Simulate error when creating EnvoyFilter
	client.PrependReactor("create", "envoyfilters", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("failed to create filter")
	})

	// Execute
	err := pattern.Added(ctx, gwClass)

	// Verify - should return error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not create Envoy Filter")
}

func TestDeleted_SuccessfulDeletion(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	// Pre-create the EnvoyFilter
	existingFilter := newTestEnvoyFilter("istio-system")
	client := dynamicfake.NewSimpleDynamicClient(scheme, existingFilter)

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
		Injection: appsecconfig.Injection{
			IstioNamespace: "istio-system",
		},
	}

	pattern := newTestIstioPattern(client, logger, config)
	gwClass := newTestGatewayClass("istio", istioGatewayControllerName)
	gwClass.SetNamespace(config.IstioNamespace)

	deletedResources := []string{}

	client.PrependReactor("delete", "*", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteAction := action.(k8stesting.DeleteAction)
		deletedResources = append(deletedResources, deleteAction.GetName())
		return false, nil, nil
	})

	// Execute
	err := pattern.Deleted(ctx, gwClass)

	// Verify
	require.NoError(t, err)
	assert.Contains(t, deletedResources, envoyFilterName, "EnvoyFilter should be deleted")
}

func TestDeleted_FilterAlreadyDeleted(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme) // No existing filter

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
		Injection: appsecconfig.Injection{
			IstioNamespace: "istio-system",
		},
	}

	pattern := newTestIstioPattern(client, logger, config)
	gwClass := newTestGatewayClass("istio", istioGatewayControllerName)
	gwClass.SetNamespace("test-ns")

	deleteCalled := false
	client.PrependReactor("delete", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteCalled = true
		return false, nil, nil
	})

	// Execute
	err := pattern.Deleted(ctx, gwClass)

	// Verify - should succeed without attempting deletion
	require.NoError(t, err)
	assert.False(t, deleteCalled, "Should not attempt to delete when filter doesn't exist")
}

func TestDeleted_NonIstioGatewayClass(t *testing.T) {
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
			IstioNamespace: "istio-system",
		},
	}

	pattern := newTestIstioPattern(client, logger, config)
	gwClass := newTestGatewayClass("envoy", "envoy.io/gateway-controller")
	gwClass.SetNamespace("test-ns")

	deleteCalled := false
	client.PrependReactor("delete", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteCalled = true
		return false, nil, nil
	})

	// Execute
	err := pattern.Deleted(ctx, gwClass)

	// Verify - should skip processing for non-Istio gateway
	require.NoError(t, err)
	assert.False(t, deleteCalled, "Should not delete resources for non-Istio gateway")
}

func TestDeleted_MissingControllerName(t *testing.T) {
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
			IstioNamespace: "istio-system",
		},
	}

	pattern := newTestIstioPattern(client, logger, config)

	// Gateway class without controllerName
	gwClass := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "GatewayClass",
			"metadata": map[string]any{
				"name":      "test",
				"namespace": "test-ns",
			},
			"spec": map[string]any{},
		},
	}

	// Execute
	err := pattern.Deleted(ctx, gwClass)

	// Verify - should return error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not get gateway controller name")
}

func TestDeleted_GetFilterError(t *testing.T) {
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
			IstioNamespace: "istio-system",
		},
	}

	pattern := newTestIstioPattern(client, logger, config)
	gwClass := newTestGatewayClass("istio", istioGatewayControllerName)
	gwClass.SetNamespace("test-ns")

	// Simulate a non-NotFound error when checking for existing filter
	client.PrependReactor("get", "envoyfilters", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("API server error")
	})

	// Execute
	err := pattern.Deleted(ctx, gwClass)

	// Verify - should return error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not check if Envoy Filter was already deleted")
}

func TestDeleted_FilterDeletionError(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	// Pre-create the EnvoyFilter
	existingFilter := newTestEnvoyFilter("istio-system")
	client := dynamicfake.NewSimpleDynamicClient(scheme, existingFilter)

	config := appsecconfig.Config{
		Product: appsecconfig.Product{
			Processor: appsecconfig.Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
		},
		Injection: appsecconfig.Injection{
			IstioNamespace: "istio-system",
		},
	}

	pattern := newTestIstioPattern(client, logger, config)
	gwClass := newTestGatewayClass("istio", istioGatewayControllerName)
	gwClass.SetNamespace(config.IstioNamespace)

	// Simulate error when deleting filter
	client.PrependReactor("delete", "envoyfilters", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("failed to delete filter")
	})

	// Execute
	err := pattern.Deleted(ctx, gwClass)

	// Verify - should return error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not delete Envoy Filter")
}
