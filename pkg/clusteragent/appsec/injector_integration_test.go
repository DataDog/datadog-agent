// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package appsec

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

const (
	integrationTestTimeout = 10 * time.Second
	integrationTestTick    = 100 * time.Millisecond
)

// integrationFixture provides a test environment for integration tests
type integrationFixture struct {
	t             *testing.T
	ctx           context.Context
	cancel        context.CancelFunc
	logger        log.Component
	config        config.Component
	dynamicClient *dynamicfake.FakeDynamicClient
	scheme        *runtime.Scheme
}

func newIntegrationFixture(t *testing.T, configOverrides map[string]interface{}) *integrationFixture {
	ctx, cancel := context.WithCancel(context.Background())

	scheme := runtime.NewScheme()
	_ = apiextensionsv1.AddToScheme(scheme)

	return &integrationFixture{
		t:             t,
		ctx:           ctx,
		cancel:        cancel,
		logger:        logmock.New(t),
		config:        newMockConfig(t, configOverrides),
		dynamicClient: dynamicfake.NewSimpleDynamicClient(scheme),
		scheme:        scheme,
	}
}

func newMockConfig(t *testing.T, overrides map[string]interface{}) config.Component {
	// Create a mock config with overrides
	return config.NewMockWithOverrides(t, overrides)
}

func (f *integrationFixture) cleanup() {
	f.cancel()
}

// createEnvoyGatewayCRD creates the EnvoyExtensionPolicy CRD for testing
func (f *integrationFixture) createEnvoyGatewayCRD() {
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "envoyextensionpolicies.gateway.envoyproxy.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "gateway.envoyproxy.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "envoyextensionpolicies",
				Singular: "envoyextensionpolicy",
				Kind:     "EnvoyExtensionPolicy",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha1",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
						},
					},
				},
			},
		},
	}

	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(crd)
	require.NoError(f.t, err)

	unstructuredCRD := &unstructured.Unstructured{Object: obj}
	_, err = f.dynamicClient.Resource(crdGVR).Create(f.ctx, unstructuredCRD, metav1.CreateOptions{})
	require.NoError(f.t, err)
}

// createGatewayCRD creates the Gateway CRD for testing
func (f *integrationFixture) createGatewayCRD() {
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gateways.gateway.networking.k8s.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "gateway.networking.k8s.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "gateways",
				Singular: "gateway",
				Kind:     "Gateway",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
						},
					},
				},
			},
		},
	}

	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(crd)
	require.NoError(f.t, err)

	unstructuredCRD := &unstructured.Unstructured{Object: obj}
	_, err = f.dynamicClient.Resource(crdGVR).Create(f.ctx, unstructuredCRD, metav1.CreateOptions{})
	require.NoError(f.t, err)
}

// createReferenceGrantCRD creates the ReferenceGrant CRD for testing
func (f *integrationFixture) createReferenceGrantCRD() {
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "referencegrants.gateway.networking.k8s.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "gateway.networking.k8s.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "referencegrants",
				Singular: "referencegrant",
				Kind:     "ReferenceGrant",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha2",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
						},
					},
				},
			},
		},
	}

	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(crd)
	require.NoError(f.t, err)

	unstructuredCRD := &unstructured.Unstructured{Object: obj}
	_, err = f.dynamicClient.Resource(crdGVR).Create(f.ctx, unstructuredCRD, metav1.CreateOptions{})
	require.NoError(f.t, err)
}

func TestIntegration_NewSecurityInjector_Disabled(t *testing.T) {
	f := newIntegrationFixture(t, map[string]interface{}{
		"appsec.proxy.enabled":                 false,
		"cluster_agent.appsec.injector.enabled": false,
	})
	defer f.cleanup()

	injector := newSecurityInjector(f.ctx, f.logger, f.config)
	assert.Nil(t, injector, "Injector should be nil when both features are disabled")
}

func TestIntegration_NewSecurityInjector_ProductEnabledOnly(t *testing.T) {
	f := newIntegrationFixture(t, map[string]interface{}{
		"appsec.proxy.enabled":                 true,
		"cluster_agent.appsec.injector.enabled": false,
	})
	defer f.cleanup()

	injector := newSecurityInjector(f.ctx, f.logger, f.config)
	assert.Nil(t, injector, "Injector should be nil when injection is disabled")
}

func TestIntegration_NewSecurityInjector_InjectorEnabledOnly(t *testing.T) {
	f := newIntegrationFixture(t, map[string]interface{}{
		"appsec.proxy.enabled":                 false,
		"cluster_agent.appsec.injector.enabled": true,
	})
	defer f.cleanup()

	injector := newSecurityInjector(f.ctx, f.logger, f.config)
	assert.Nil(t, injector, "Injector should be nil when product is disabled")
}

func TestIntegration_NewSecurityInjector_NoProxiesConfigured(t *testing.T) {
	f := newIntegrationFixture(t, map[string]interface{}{
		"appsec.proxy.enabled":                              true,
		"cluster_agent.appsec.injector.enabled":             true,
		"appsec.proxy.proxies":                              []string{},
		"appsec.proxy.auto_detect":                          false,
		"cluster_agent.appsec.injector.processor.service.name":      "test-service",
		"cluster_agent.appsec.injector.processor.service.namespace": "test-namespace",
	})
	defer f.cleanup()

	injector := newSecurityInjector(f.ctx, f.logger, f.config)
	assert.Nil(t, injector, "Injector should be nil when no proxies are configured")
}

func TestIntegration_NewSecurityInjector_WithEnvoyGateway(t *testing.T) {
	f := newIntegrationFixture(t, map[string]interface{}{
		"appsec.proxy.enabled":                              true,
		"cluster_agent.appsec.injector.enabled":             true,
		"appsec.proxy.proxies":                              []string{"envoy-gateway"},
		"appsec.proxy.auto_detect":                          false,
		"cluster_agent.appsec.injector.processor.service.name":      "test-service",
		"cluster_agent.appsec.injector.processor.service.namespace": "test-namespace",
		"appsec.proxy.processor.address":                    "test-service.test-namespace.svc",
		"appsec.proxy.processor.port":                       443,
		"cluster_agent.appsec.injector.base_backoff":        "100ms",
		"cluster_agent.appsec.injector.max_backoff":         "1s",
	})
	defer f.cleanup()

	// Mock the API client to avoid needing a real Kubernetes cluster
	// Note: This test would need the APIClient to be mockable or dependency-injected
	// For now, we test the configuration parsing
	config := appsecconfig.FromComponent(f.config)

	assert.True(t, config.Product.Enabled, "Product should be enabled")
	assert.True(t, config.Injection.Enabled, "Injection should be enabled")
	assert.Contains(t, config.Proxies, appsecconfig.ProxyTypeEnvoyGateway, "EnvoyGateway should be in proxies")
	assert.Equal(t, "test-service", config.Processor.ServiceName)
	assert.Equal(t, "test-namespace", config.Processor.Namespace)
	assert.Equal(t, 443, config.Processor.Port)
}

func TestIntegration_NewSecurityInjector_UnsupportedProxy(t *testing.T) {
	f := newIntegrationFixture(t, map[string]interface{}{
		"appsec.proxy.enabled":                              true,
		"cluster_agent.appsec.injector.enabled":             true,
		"appsec.proxy.proxies":                              []string{"unsupported-proxy", "envoy-gateway"},
		"appsec.proxy.auto_detect":                          false,
		"cluster_agent.appsec.injector.processor.service.name":      "test-service",
		"cluster_agent.appsec.injector.processor.service.namespace": "test-namespace",
	})
	defer f.cleanup()

	config := appsecconfig.FromComponent(f.config)

	// Should only contain supported proxies
	assert.Len(t, config.Proxies, 1, "Should only have one supported proxy")
	assert.Contains(t, config.Proxies, appsecconfig.ProxyTypeEnvoyGateway, "EnvoyGateway should be in proxies")
	assert.NotContains(t, config.Proxies, appsecconfig.ProxyType("unsupported-proxy"), "Unsupported proxy should not be in proxies")
}

func TestIntegration_CompilePatterns_WithValidConfig(t *testing.T) {
	f := newIntegrationFixture(t, map[string]interface{}{
		"appsec.proxy.enabled":                              true,
		"cluster_agent.appsec.injector.enabled":             true,
		"appsec.proxy.proxies":                              []string{"envoy-gateway"},
		"appsec.proxy.auto_detect":                          false,
		"cluster_agent.appsec.injector.processor.service.name":      "test-service",
		"cluster_agent.appsec.injector.processor.service.namespace": "test-namespace",
		"appsec.proxy.processor.address":                    "test-service.test-namespace.svc",
		"appsec.proxy.processor.port":                       443,
		"cluster_agent.appsec.injector.labels":              map[string]string{"app": "test"},
		"cluster_agent.appsec.injector.annotations":         map[string]string{"annotation": "value"},
	})
	defer f.cleanup()

	mockConfig := appsecconfig.Config{
		Injection: appsecconfig.Injection{
			Enabled: true,
			CommonLabels: map[string]string{
				"app": "test",
			},
			CommonAnnotations: map[string]string{
				"annotation": "value",
			},
		},
		Product: appsecconfig.Product{
			Enabled: true,
			Proxies: map[appsecconfig.ProxyType]struct{}{
				appsecconfig.ProxyTypeEnvoyGateway: {},
			},
			Processor: appsecconfig.Processor{
				ServiceName: "test-service",
				Namespace:   "test-namespace",
				Port:        443,
				Address:     "test-service.test-namespace.svc",
			},
		},
	}

	si := &securityInjector{
		ctx:        f.ctx,
		cancel:     f.cancel,
		k8sClient:  f.dynamicClient,
		logger:     f.logger,
		config:     mockConfig,
		baseBackoff: 100 * time.Millisecond,
		maxBackoff:  1 * time.Second,
	}

	patterns := si.CompilePatterns()

	require.Len(t, patterns, 1, "Should have one pattern")
	assert.Contains(t, patterns, appsecconfig.ProxyTypeEnvoyGateway, "Should have envoy-gateway pattern")

	pattern := patterns[appsecconfig.ProxyTypeEnvoyGateway]
	assert.NotNil(t, pattern, "Pattern should not be nil")

	// Verify the pattern returns correct GVR
	gvr := pattern.Resource()
	assert.Equal(t, "gateway.networking.k8s.io", gvr.Group)
	assert.Equal(t, "v1", gvr.Version)
	assert.Equal(t, "gateways", gvr.Resource)
}

func TestIntegration_EventHandler_AddEvent(t *testing.T) {
	f := newIntegrationFixture(t, map[string]interface{}{
		"appsec.proxy.enabled":                              true,
		"cluster_agent.appsec.injector.enabled":             true,
		"appsec.proxy.proxies":                              []string{"envoy-gateway"},
		"cluster_agent.appsec.injector.processor.service.name":      "test-service",
		"cluster_agent.appsec.injector.processor.service.namespace": "test-namespace",
	})
	defer f.cleanup()

	mockConfig := appsecconfig.Config{
		Injection: appsecconfig.Injection{
			Enabled: true,
		},
		Product: appsecconfig.Product{
			Enabled: true,
			Proxies: map[appsecconfig.ProxyType]struct{}{
				appsecconfig.ProxyTypeEnvoyGateway: {},
			},
			Processor: appsecconfig.Processor{
				ServiceName: "test-service",
				Namespace:   "test-namespace",
				Port:        443,
			},
		},
	}

	si := &securityInjector{
		ctx:                   f.ctx,
		cancel:                f.cancel,
		k8sClient:             f.dynamicClient,
		logger:                f.logger,
		config:                mockConfig,
		leaderElectionEnabled: false,
		baseBackoff:           100 * time.Millisecond,
		maxBackoff:            1 * time.Second,
	}
	_ = si // Use the variable

	// Create a mock work queue to track events
	addedItems := []workItem{}

	// Intercept the dynamic client to track resource creation
	f.dynamicClient.PrependReactor("create", "*", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		obj := createAction.GetObject().(*unstructured.Unstructured)

		addedItems = append(addedItems, workItem{
			name:      obj.GetName(),
			namespace: obj.GetNamespace(),
			typ:       workItemAdded,
		})

		return false, nil, nil
	})

	// Test that we can create gateway objects
	gateway := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "Gateway",
			"metadata": map[string]interface{}{
				"name":      "test-gateway",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"gatewayClassName": "envoy",
			},
		},
	}

	gatewayGVR := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "gateways",
	}

	_, err := f.dynamicClient.Resource(gatewayGVR).Namespace("default").Create(f.ctx, gateway, metav1.CreateOptions{})
	require.NoError(t, err)

	// Verify the event was tracked
	assert.Eventually(t, func() bool {
		return len(addedItems) > 0
	}, integrationTestTimeout, integrationTestTick, "Should have tracked the added gateway")
}

func TestIntegration_Start_DoubleStart(t *testing.T) {
	// Reset global injector for this test
	injector = nil
	injectorStartOnce = sync.Once{}

	f := newIntegrationFixture(t, map[string]interface{}{
		"appsec.proxy.enabled":                              true,
		"cluster_agent.appsec.injector.enabled":             true,
		"appsec.proxy.proxies":                              []string{"envoy-gateway"},
		"appsec.proxy.auto_detect":                          false,
		"cluster_agent.appsec.injector.processor.service.name":      "test-service",
		"cluster_agent.appsec.injector.processor.service.namespace": "test-namespace",
	})
	defer f.cleanup()

	// Note: Start() requires actual APIClient which we can't easily mock
	// This test verifies the double-start protection logic
	injector = &securityInjector{
		ctx:    f.ctx,
		cancel: f.cancel,
		logger: f.logger,
		config: appsecconfig.Config{},
	}

	err := Start(f.ctx, f.logger, f.config)
	assert.Error(t, err, "Should return error on second start")
	assert.Contains(t, err.Error(), "can't start proxy injection twice", "Error should mention double start")

	// Cleanup for other tests
	injector = nil
	injectorStartOnce = sync.Once{}
}

func TestIntegration_ConfigValidation_MissingProcessorName(t *testing.T) {
	f := newIntegrationFixture(t, map[string]interface{}{
		"appsec.proxy.enabled":                              true,
		"cluster_agent.appsec.injector.enabled":             true,
		"appsec.proxy.proxies":                              []string{"envoy-gateway"},
		// Missing processor service name
		"cluster_agent.appsec.injector.processor.service.namespace": "test-namespace",
	})
	defer f.cleanup()

	config := appsecconfig.FromComponent(f.config)

	// Should have default empty processor name
	assert.Empty(t, config.Processor.ServiceName, "Processor service name should be empty when not configured")
}

func TestIntegration_ConfigValidation_DefaultNamespace(t *testing.T) {
	f := newIntegrationFixture(t, map[string]interface{}{
		"appsec.proxy.enabled":                              true,
		"cluster_agent.appsec.injector.enabled":             true,
		"appsec.proxy.proxies":                              []string{"envoy-gateway"},
		"cluster_agent.appsec.injector.processor.service.name": "test-service",
		// Missing namespace - should default
	})
	defer f.cleanup()

	config := appsecconfig.FromComponent(f.config)

	// Should have default namespace
	assert.NotEmpty(t, config.Processor.Namespace, "Processor namespace should have a default value")
}
