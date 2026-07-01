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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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

// newFakeDynamicClient creates a fake dynamic client with proper list kind registrations
// for all resources that may be listed in tests (gateways.networking.istio.io and gatewayclasses.gateway.networking.k8s.io)
func newFakeDynamicClient(scheme *runtime.Scheme, objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			istioGatewayGVR: "GatewayList",
			gatewayClassGVR: "GatewayClassList",
		},
		objects...)
}

// newTestNativeGatewayPattern creates a test instance with mocked dependencies
func newTestNativeGatewayPattern(client dynamic.Interface, logger log.Component, config appsecconfig.Config) *istioNativeGatewayPattern {
	recorder := record.NewFakeRecorder(100)

	return &istioNativeGatewayPattern{
		istioInjectionPattern: &istioInjectionPattern{
			client: client,
			logger: logger,
			config: config,
			eventRecorder: eventRecorder{
				recorder: recorder,
			},
		},
	}
}

// newTestIstioGateway creates a test networking.istio.io/v1 Gateway unstructured object
func newTestIstioGateway(name, namespace string, selector map[string]any) *unstructured.Unstructured {
	gw := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "networking.istio.io/v1",
			"kind":       "Gateway",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"selector": selector,
				"servers": []any{
					map[string]any{
						"port": map[string]any{
							"number":   int64(80),
							"name":     "http",
							"protocol": "HTTP",
						},
						"hosts": []any{"*.example.com"},
					},
				},
			},
		},
	}
	return gw
}

func TestGateway_Added_SuccessfulCreation(t *testing.T) {
	ctx := context.Background()
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

	pattern := newTestNativeGatewayPattern(client, logger, config)
	gw := newTestIstioGateway("test-gateway", "default", map[string]any{"app": "istio-gateway"})

	var createdFilter *unstructured.Unstructured
	client.PrependReactor("create", "envoyfilters", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdFilter = createAction.GetObject().(*unstructured.Unstructured)
		return false, createdFilter, nil
	})

	err := pattern.Added(ctx, gw)

	require.NoError(t, err)
	assert.NotNil(t, createdFilter, "EnvoyFilter should be created")
	assert.Equal(t, "istio-system", createdFilter.GetNamespace())
	assert.Equal(t, envoyFilterName, createdFilter.GetName())
	assert.Equal(t, "networking.istio.io/v1alpha3", createdFilter.GetAPIVersion())
}

func TestGateway_Added_FilterAlreadyExists(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	existingFilter := newTestEnvoyFilter("istio-system")
	client := newFakeDynamicClient(scheme, existingFilter)

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

	pattern := newTestNativeGatewayPattern(client, logger, config)
	gw := newTestIstioGateway("test-gateway", "default", map[string]any{"app": "istio-gateway"})

	createCalled := false
	client.PrependReactor("create", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createCalled = true
		return false, nil, nil
	})

	err := pattern.Added(ctx, gw)

	require.NoError(t, err)
	assert.False(t, createCalled, "Should not attempt to create new resources when filter already exists")
}

func TestGateway_Deleted_SuccessfulDeletion(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	existingFilter := newTestEnvoyFilter("istio-system")
	client := newFakeDynamicClient(scheme, existingFilter)

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

	pattern := newTestNativeGatewayPattern(client, logger, config)
	gw := newTestIstioGateway("test-gateway", "default", map[string]any{"app": "istio-gateway"})

	deletedResources := []string{}
	client.PrependReactor("delete", "*", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteAction := action.(k8stesting.DeleteAction)
		deletedResources = append(deletedResources, deleteAction.GetName())
		return false, nil, nil
	})

	err := pattern.Deleted(ctx, gw)

	require.NoError(t, err)
	assert.Contains(t, deletedResources, envoyFilterName, "EnvoyFilter should be deleted")
}

func TestGateway_Deleted_OtherGatewaysExist(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	existingFilter := newTestEnvoyFilter("istio-system")
	client := newFakeDynamicClient(scheme, existingFilter)

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

	pattern := newTestNativeGatewayPattern(client, logger, config)
	gw := newTestIstioGateway("test-gateway", "default", map[string]any{"app": "istio-gateway"})
	otherGateway := newTestIstioGateway("other-gateway", "other-ns", map[string]any{"app": "other"})

	// Return the other gateway when listing
	client.PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{*otherGateway},
		}, nil
	})

	deleteCalled := false
	client.PrependReactor("delete", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteCalled = true
		return false, nil, nil
	})

	err := pattern.Deleted(ctx, gw)

	require.NoError(t, err)
	assert.False(t, deleteCalled, "Should not delete EnvoyFilter when other Istio Gateways exist")
}

func TestGateway_Deleted_GatewayClassStillExists(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	existingFilter := newTestEnvoyFilter("istio-system")
	istioGatewayClass := newTestGatewayClass("istio", istioGatewayControllerName)
	client := newFakeDynamicClient(scheme, existingFilter, istioGatewayClass)

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

	pattern := newTestNativeGatewayPattern(client, logger, config)
	gw := newTestIstioGateway("test-gateway", "default", map[string]any{"app": "istio-gateway"})

	deleteCalled := false
	client.PrependReactor("delete", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteCalled = true
		return false, nil, nil
	})

	err := pattern.Deleted(ctx, gw)

	require.NoError(t, err)
	assert.False(t, deleteCalled, "Should not delete EnvoyFilter when Istio GatewayClasses still exist")
}

func TestGateway_IsInjectionPossible(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*dynamicfake.FakeDynamicClient)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "both CRDs present",
			setupMock: func(client *dynamicfake.FakeDynamicClient) {
				client.PrependReactor("get", "customresourcedefinitions", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					getAction := action.(k8stesting.GetAction)
					crd := &unstructured.Unstructured{}
					crd.SetName(getAction.GetName())
					return true, crd, nil
				})
			},
			wantErr: false,
		},
		{
			name: "envoyfilter CRD missing",
			setupMock: func(client *dynamicfake.FakeDynamicClient) {
				client.PrependReactor("get", "customresourcedefinitions", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					getAction := action.(k8stesting.GetAction)
					if getAction.GetName() == "envoyfilters.networking.istio.io" {
						return true, nil, errors.New("not found")
					}
					crd := &unstructured.Unstructured{}
					crd.SetName(getAction.GetName())
					return true, crd, nil
				})
			},
			wantErr: true,
			errMsg:  "error getting EnvoyFilter CRD",
		},
		{
			name: "istio gateway CRD missing",
			setupMock: func(client *dynamicfake.FakeDynamicClient) {
				client.PrependReactor("get", "customresourcedefinitions", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					getAction := action.(k8stesting.GetAction)
					if getAction.GetName() == "gateways.networking.istio.io" {
						return true, nil, errors.New("not found")
					}
					crd := &unstructured.Unstructured{}
					crd.SetName(getAction.GetName())
					return true, crd, nil
				})
			},
			wantErr: true,
			errMsg:  "error getting Istio Gateway CRD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := logmock.New(t)
			scheme := runtime.NewScheme()
			client := newFakeDynamicClient(scheme)

			config := appsecconfig.Config{
				Injection: appsecconfig.Injection{
					IstioNamespace: "istio-system",
				},
			}

			pattern := newTestNativeGatewayPattern(client, logger, config)
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

func TestGateway_Added_GetFilterError(t *testing.T) {
	ctx := context.Background()
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
			IstioNamespace: "istio-system",
		},
	}

	pattern := newTestNativeGatewayPattern(client, logger, config)
	gw := newTestIstioGateway("test-gateway", "default", map[string]any{"app": "istio-gateway"})

	client.PrependReactor("get", "envoyfilters", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("API server error")
	})

	err := pattern.Added(ctx, gw)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not check if Envoy Filter already exists")
}

func TestGateway_Added_FilterCreationError(t *testing.T) {
	ctx := context.Background()
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
			IstioNamespace: "istio-system",
		},
	}

	pattern := newTestNativeGatewayPattern(client, logger, config)
	gw := newTestIstioGateway("test-gateway", "default", map[string]any{"app": "istio-gateway"})

	client.PrependReactor("create", "envoyfilters", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("failed to create filter")
	})

	err := pattern.Added(ctx, gw)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not create Envoy Filter")
}

// TestGateway_Deleted_CleanupMode tests that cleanupPattern can delete the EnvoyFilter
// even when multiple native gateways are still present in the cluster.
// cleanupPattern calls Deleted on live (not yet deleted) resources, so each call sees
// the others still present. The fix detects this case via a Get of the "deleted" object.
func TestGateway_Deleted_CleanupMode_MultipleGateways(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	existingFilter := newTestEnvoyFilter("istio-system")
	gwA := newTestIstioGateway("gateway-a", "default", map[string]any{"app": "gw"})
	gwB := newTestIstioGateway("gateway-b", "default", map[string]any{"app": "gw"})
	client := newFakeDynamicClient(scheme, existingFilter, gwA, gwB)

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

	pattern := newTestNativeGatewayPattern(client, logger, config)

	// Simulate cleanupPattern: call Deleted for gwA while both gateways still exist
	// Get(gwA) will return found (not NotFound), so we're in cleanup mode → skip coordination
	client.PrependReactor("get", "gateways", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		getAction := action.(k8stesting.GetAction)
		if getAction.GetName() == "gateway-a" {
			return true, gwA, nil // Still in cluster (cleanup mode)
		}
		return false, nil, nil
	})

	deletedResources := []string{}
	client.PrependReactor("delete", "*", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteAction := action.(k8stesting.DeleteAction)
		deletedResources = append(deletedResources, deleteAction.GetName())
		return false, nil, nil
	})

	err := pattern.Deleted(ctx, gwA)

	require.NoError(t, err)
	assert.Contains(t, deletedResources, envoyFilterName,
		"EnvoyFilter should be deleted in cleanup mode even when other gateways still exist")
}

// TestGateway_Deleted_APIErrorInContextDetection verifies that a transient API error during
// cleanup-mode detection causes Deleted() to return an error rather than silently entering
// cleanup mode and prematurely deleting the shared EnvoyFilter.
func TestGateway_Deleted_APIErrorInContextDetection(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	existingFilter := newTestEnvoyFilter("istio-system")
	client := newFakeDynamicClient(scheme, existingFilter)

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

	pattern := newTestNativeGatewayPattern(client, logger, config)
	gw := newTestIstioGateway("test-gateway", "default", map[string]any{"app": "istio-gateway"})

	// Simulate a transient API error on the context-detection Get call
	client.PrependReactor("get", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("connection reset by peer")
	})

	deleteCalled := false
	client.PrependReactor("delete", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteCalled = true
		return false, nil, nil
	})

	err := pattern.Deleted(ctx, gw)

	require.Error(t, err, "Should return error when context detection API call fails")
	assert.Contains(t, err.Error(), "could not determine calling context")
	assert.False(t, deleteCalled, "Should NOT delete EnvoyFilter when context cannot be determined")
}

// TestGateway_Deleted_WatchEvent_OtherGatewayExists tests that watch events still respect
// the coordination check: if gateway-a was actually deleted but gateway-b still exists,
// the EnvoyFilter must NOT be deleted.
func TestGateway_Deleted_WatchEvent_OtherGatewayExists(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	existingFilter := newTestEnvoyFilter("istio-system")
	gwB := newTestIstioGateway("gateway-b", "default", map[string]any{"app": "gw"})
	client := newFakeDynamicClient(scheme, existingFilter)

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

	pattern := newTestNativeGatewayPattern(client, logger, config)
	gwA := newTestIstioGateway("gateway-a", "default", map[string]any{"app": "gw"})

	// Simulate watch event: Get(gateway-a) returns NotFound (was actually deleted)
	// List(gateways) returns [gateway-b] (still present)
	client.PrependReactor("get", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, k8serrors.NewNotFound(schema.GroupResource{Group: "networking.istio.io", Resource: "gateways"}, "gateway-a")
	})
	client.PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{*gwB},
		}, nil
	})

	deleteCalled := false
	client.PrependReactor("delete", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteCalled = true
		return false, nil, nil
	})

	err := pattern.Deleted(ctx, gwA)

	require.NoError(t, err)
	assert.False(t, deleteCalled,
		"EnvoyFilter must NOT be deleted in watch-event mode when another gateway still exists")
}

// TestDeleted_SkipsWhenNativeGatewaysExist tests that the K8s GatewayClass pattern
// skips EnvoyFilter deletion when Istio native Gateways still exist (cross-pattern coordination)
func TestDeleted_SkipsWhenNativeGatewaysExist(t *testing.T) {
	ctx := context.Background()
	logger := logmock.New(t)
	scheme := runtime.NewScheme()

	existingFilter := newTestEnvoyFilter("istio-system")
	client := newFakeDynamicClient(scheme, existingFilter)

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

	// Use the original istio pattern (K8s GatewayClass watcher)
	pattern := newTestIstioPattern(client, logger, config)
	gwClass := newTestGatewayClass("istio", istioGatewayControllerName)

	nativeGateway := newTestIstioGateway("native-gw", "default", map[string]any{"app": "istio-gateway"})

	// Return the native gateway when listing
	client.PrependReactor("list", "gateways", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{*nativeGateway},
		}, nil
	})

	deleteCalled := false
	client.PrependReactor("delete", "*", func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		deleteCalled = true
		return false, nil, nil
	})

	err := pattern.Deleted(ctx, gwClass)

	require.NoError(t, err)
	assert.False(t, deleteCalled, "Should not delete EnvoyFilter when Istio native gateways still exist")
}
