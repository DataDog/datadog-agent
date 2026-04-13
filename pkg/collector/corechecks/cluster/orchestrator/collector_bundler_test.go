// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/inventory"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/discovery"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	orchcfg "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// createMockAPIClient creates a mock APIClient with fake Kubernetes and dynamic clients
// configured with the necessary custom resource types for testing.
func createMockAPIClient() *apiserver.APIClient {
	client := k8sfake.NewClientset()
	scheme := runtime.NewScheme()

	// Register custom resource types that the tests expect
	dynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			// Datadog resources
			{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogmetrics"}:        "DatadogMetricList",
			{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogmonitors"}:       "DatadogMonitorList",
			{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogslos"}:           "DatadogSloList",
			{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogdashboards"}:     "DatadogDashboardList",
			{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogagentprofiles"}:  "DatadogAgentProfileList",
			{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogpodautoscalers"}: "DatadogPodAutoscalerList",
			{Group: "datadoghq.com", Version: "v1alpha2", Resource: "datadogpodautoscalers"}: "DatadogPodAutoscalerList",
			{Group: "datadoghq.com", Version: "v2alpha1", Resource: "datadogagents"}:         "DatadogAgentList",
			// Third-party resources
			{Group: "argoproj.io", Version: "v1alpha1", Resource: "rollouts"}: "RolloutList",
		})

	return &apiserver.APIClient{
		Cl:        client,
		DynamicCl: dynamicClient,
	}
}

func TestImportBuiltinCollectors(t *testing.T) {
	cfg := mockconfig.New(t)
	cfg.SetInTest("orchestrator_explorer.terminated_pods.enabled", true)
	cfg.SetInTest("orchestrator_explorer.custom_resources.ootb.enabled", true)

	// Set up discovery cache with supported resources
	collectorDiscovery := &discovery.DiscoveryCollector{}
	collectorDiscovery.SetCache(discovery.DiscoveryCache{
		CollectorForVersion: map[discovery.CollectorVersion]struct{}{
			{GroupVersion: "v1", Kind: "pods"}:                                      {},
			{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogmetrics"}:        {},
			{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogmonitors"}:       {},
			{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogslos"}:           {},
			{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogdashboards"}:     {},
			{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogagentprofiles"}:  {},
			{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogpodautoscalers"}: {},
			{GroupVersion: "datadoghq.com/v1alpha2", Kind: "datadogpodautoscalers"}: {},
			{GroupVersion: "datadoghq.com/v2alpha1", Kind: "datadogagents"}:         {},
		},
		Groups: []*v1.APIGroup{
			{
				Name: "datadoghq.com",
				Versions: []v1.GroupVersionForDiscovery{
					{Version: "v1alpha1"},
					{Version: "v1alpha2"},
					{Version: "v2alpha1"},
				},
				PreferredVersion: v1.GroupVersionForDiscovery{
					GroupVersion: "datadoghq.com/v2alpha1",
					Version:      "v2alpha1",
				},
			},
		},
	})

	cb := CollectorBundle{
		collectorDiscovery:  collectorDiscovery,
		activatedCollectors: make(map[string]struct{}),
		collectors: []collectors.K8sCollector{
			k8s.NewUnassignedPodCollector(nil, nil, nil),
			k8s.NewCRDCollector(),
		},
		inventory: inventory.NewCollectorInventory(cfg, nil, nil),
		runCfg: &collectors.CollectorRunConfig{
			K8sCollectorRunConfig: collectors.K8sCollectorRunConfig{
				APIClient: createMockAPIClient(),
			},
		},
	}

	cb.importBuiltinCollectors()
	names := make([]string, 0, len(cb.collectors))
	for _, collector := range cb.collectors {
		names = append(names, collector.Metadata().FullName())
	}

	expected := []string{
		"v1/pods",
		"v1/terminated-pods",
		"apiextensions.k8s.io/v1/customresourcedefinitions",
		"datadoghq.com/v1alpha1/datadogmetrics",
		"datadoghq.com/v1alpha1/datadogmonitors",
		"datadoghq.com/v1alpha1/datadogslos",
		"datadoghq.com/v1alpha1/datadogdashboards",
		"datadoghq.com/v1alpha1/datadogagentprofiles",
		"datadoghq.com/v1alpha2/datadogpodautoscalers", // preferred version selected
		"datadoghq.com/v2alpha1/datadogagents",
	}
	require.ElementsMatch(t, expected, names)
}

func TestGetDatadogCustomResourceCollectors(t *testing.T) {
	collectorDiscovery := &discovery.DiscoveryCollector{}
	cb := CollectorBundle{
		check: &OrchestratorCheck{
			orchestratorConfig: &orchcfg.OrchestratorConfig{},
		},
		collectors:         []collectors.K8sCollector{},
		collectorDiscovery: collectorDiscovery,
		inventory:          inventory.NewCollectorInventory(mockconfig.New(t), nil, nil),
		runCfg: &collectors.CollectorRunConfig{
			K8sCollectorRunConfig: collectors.K8sCollectorRunConfig{
				APIClient: createMockAPIClient(),
			},
		},
	}

	for _, testCase := range []struct {
		name               string
		enabled            bool
		hasCrdCollectors   bool
		supportedResources discovery.DiscoveryCache
		expected           []string
	}{
		{
			name:             "Datadog CR collectors enabled with all supported resources",
			enabled:          true,
			hasCrdCollectors: true,
			supportedResources: discovery.DiscoveryCache{
				CollectorForVersion: map[discovery.CollectorVersion]struct{}{
					{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogmetrics"}:        {},
					{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogmonitors"}:       {},
					{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogslos"}:           {},
					{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogdashboards"}:     {},
					{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogagentprofiles"}:  {},
					{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogpodautoscalers"}: {},
					{GroupVersion: "datadoghq.com/v1alpha2", Kind: "datadogpodautoscalers"}: {},
					{GroupVersion: "datadoghq.com/v2alpha1", Kind: "datadogagents"}:         {},
				},
				Groups: []*v1.APIGroup{
					{
						Name: "datadoghq.com",
						Versions: []v1.GroupVersionForDiscovery{
							{Version: "v1alpha1"},
							{Version: "v1alpha2"},
							{Version: "v2alpha1"},
						},
						PreferredVersion: v1.GroupVersionForDiscovery{
							GroupVersion: "datadoghq.com/v1alpha2",
							Version:      "v1alpha2",
						},
					},
				},
			},
			expected: []string{
				"datadoghq.com/v1alpha1/datadogmetrics",
				"datadoghq.com/v1alpha1/datadogmonitors",
				"datadoghq.com/v1alpha1/datadogslos",
				"datadoghq.com/v1alpha1/datadogdashboards",
				"datadoghq.com/v1alpha1/datadogagentprofiles",
				"datadoghq.com/v1alpha2/datadogpodautoscalers", // preferred version selected
				"datadoghq.com/v2alpha1/datadogagents",
			},
		},
		{
			name:             "Datadog CR collectors enabled with no supported resources",
			enabled:          true,
			hasCrdCollectors: true,
			supportedResources: discovery.DiscoveryCache{
				CollectorForVersion: map[discovery.CollectorVersion]struct{}{},
				Groups:              []*v1.APIGroup{},
			},
			expected: []string{},
		},
		{
			name:             "Datadog CR collectors enabled with partial supported resources",
			enabled:          true,
			hasCrdCollectors: true,
			supportedResources: discovery.DiscoveryCache{
				CollectorForVersion: map[discovery.CollectorVersion]struct{}{
					{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogmetrics"}:  {},
					{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogmonitors"}: {},
				},
				Groups: []*v1.APIGroup{
					{
						Name: "datadoghq.com",
						Versions: []v1.GroupVersionForDiscovery{
							{Version: "v1alpha1"},
						},
						PreferredVersion: v1.GroupVersionForDiscovery{
							GroupVersion: "datadoghq.com/v1alpha1",
							Version:      "v1alpha1",
						},
					},
				},
			},
			expected: []string{
				"datadoghq.com/v1alpha1/datadogmetrics",
				"datadoghq.com/v1alpha1/datadogmonitors",
			},
		},
		{
			name:             "Datadog CR collectors disabled",
			enabled:          false,
			hasCrdCollectors: true,
			supportedResources: discovery.DiscoveryCache{
				CollectorForVersion: map[discovery.CollectorVersion]struct{}{
					{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogmetrics"}:        {},
					{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogmonitors"}:       {},
					{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogpodautoscalers"}: {},
					{GroupVersion: "datadoghq.com/v1alpha2", Kind: "datadogpodautoscalers"}: {},
					{GroupVersion: "datadoghq.com/v2alpha1", Kind: "datadogagents"}:         {},
				},
				Groups: []*v1.APIGroup{
					{
						Name: "datadoghq.com",
						Versions: []v1.GroupVersionForDiscovery{
							{Version: "v1alpha1"},
							{Version: "v1alpha2"},
							{Version: "v2alpha1"},
						},
						PreferredVersion: v1.GroupVersionForDiscovery{
							GroupVersion: "datadoghq.com/v1alpha2",
							Version:      "v1alpha2",
						},
					},
				},
			},
			expected: []string{},
		},
		{
			name:             "Datadog CR collectors enabled without CRD collectors",
			enabled:          true,
			hasCrdCollectors: false,
			supportedResources: discovery.DiscoveryCache{
				CollectorForVersion: map[discovery.CollectorVersion]struct{}{
					{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogmetrics"}:        {},
					{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogmonitors"}:       {},
					{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogpodautoscalers"}: {},
					{GroupVersion: "datadoghq.com/v1alpha2", Kind: "datadogpodautoscalers"}: {},
					{GroupVersion: "datadoghq.com/v2alpha1", Kind: "datadogagents"}:         {},
				},
				Groups: []*v1.APIGroup{
					{
						Name: "datadoghq.com",
						Versions: []v1.GroupVersionForDiscovery{
							{Version: "v1alpha1"},
							{Version: "v1alpha2"},
							{Version: "v2alpha1"},
						},
						PreferredVersion: v1.GroupVersionForDiscovery{
							GroupVersion: "datadoghq.com/v1alpha2",
							Version:      "v1alpha2",
						},
					},
				},
			},
			expected: []string{},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := mockconfig.New(t)
			cfg.SetInTest("orchestrator_explorer.custom_resources.ootb.enabled", testCase.enabled)

			collectorDiscovery.SetCache(testCase.supportedResources)

			cb.collectors = []collectors.K8sCollector{}
			if testCase.hasCrdCollectors {
				cb.collectors = []collectors.K8sCollector{k8s.NewCRDCollector()}
			}

			crCollectors := cb.getBuiltinCustomResourceCollectors()
			require.Equal(t, len(testCase.expected), len(crCollectors))

			names := make([]string, 0, len(crCollectors))
			for _, collector := range crCollectors {
				names = append(names, collector.Metadata().FullName())
			}
			require.ElementsMatch(t, testCase.expected, names)
		})
	}
}

func TestGetTerminatedPodCollector(t *testing.T) {
	cfg := mockconfig.New(t)

	// add pod to discovery cache to ensure that pod collector is supported
	collectorDiscovery := &discovery.DiscoveryCollector{}
	collectorDiscovery.SetCache(
		discovery.DiscoveryCache{
			CollectorForVersion: map[discovery.CollectorVersion]struct{}{
				{GroupVersion: "v1", Kind: "pods"}: {},
			},
		})

	for _, testCase := range []struct {
		name                          string
		terminatedPodsEnabled         bool
		terminatedPodsImprovedEnabled bool
		unassignedPod                 bool
		expected                      collectors.K8sCollector
	}{
		{
			name:                          "Terminated pods collector enabled",
			terminatedPodsEnabled:         true,
			terminatedPodsImprovedEnabled: false,
			unassignedPod:                 true,
			expected:                      k8s.NewTerminatedPodCollector(nil, nil, nil),
		},
		{
			name:                          "Terminated pods improved collector enabled",
			terminatedPodsEnabled:         false,
			terminatedPodsImprovedEnabled: true,
			unassignedPod:                 true,
			expected:                      k8s.NewImprovedTerminatedPodCollector(nil, nil, nil),
		},
		{
			name:                          "Terminated pods improved collector takes precedence",
			terminatedPodsEnabled:         true,
			terminatedPodsImprovedEnabled: true,
			unassignedPod:                 true,
			expected:                      k8s.NewImprovedTerminatedPodCollector(nil, nil, nil),
		},
		{
			name:                          "Terminated pods collector disabled",
			terminatedPodsEnabled:         false,
			terminatedPodsImprovedEnabled: false,
			unassignedPod:                 true,
			expected:                      nil,
		},
		{
			name:                          "Terminated pods collector enabled without unassigned pod collector",
			terminatedPodsEnabled:         true,
			terminatedPodsImprovedEnabled: false,
			unassignedPod:                 false,
			expected:                      nil,
		},
		{
			name:                          "Terminated pods improved collector enabled without unassigned pod collector",
			terminatedPodsEnabled:         false,
			terminatedPodsImprovedEnabled: true,
			unassignedPod:                 false,
			expected:                      nil,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg.SetInTest("orchestrator_explorer.terminated_pods.enabled", testCase.terminatedPodsEnabled)
			cfg.SetInTest("orchestrator_explorer.terminated_pods_improved.enabled", testCase.terminatedPodsImprovedEnabled)

			cb := CollectorBundle{
				collectors:         []collectors.K8sCollector{},
				collectorDiscovery: collectorDiscovery,
				inventory:          inventory.NewCollectorInventory(cfg, nil, nil),
			}

			if testCase.unassignedPod {
				cb.collectors = []collectors.K8sCollector{k8s.NewUnassignedPodCollector(nil, nil, nil)}
			}

			collector := cb.getTerminatedPodCollector()
			if testCase.expected == nil {
				require.Nil(t, collector)
			} else {
				require.Equal(t, testCase.expected.Metadata().FullName(), collector.Metadata().FullName())
			}
		})
	}
}

func TestNewBuiltinCRDConfigs(t *testing.T) {
	configs := newBuiltinCRDConfigs()

	// Expected configurations (group/version/kind)
	expectedConfigs := []string{
		// Datadog resources
		"datadoghq.com/v1alpha2/datadogpodautoscalers",
		"datadoghq.com/v1alpha2/datadogpodautoscalerclusterprofiles",
		"datadoghq.com/v2alpha1/datadogagents",
		"datadoghq.com/v1alpha1/datadogslos",
		"datadoghq.com/v1alpha1/datadogdashboards",
		"datadoghq.com/v1alpha1/datadogagentprofiles",
		"datadoghq.com/v1alpha1/datadogmonitors",
		"datadoghq.com/v1alpha1/datadogmetrics",

		// Argo
		"argoproj.io/v1alpha1/rollouts",
		"argoproj.io/v1alpha1/applications",
		"argoproj.io/v1alpha1/applicationsets",

		// Flux
		"source.toolkit.fluxcd.io/v1/buckets",
		"source.toolkit.fluxcd.io/v1/helmcharts",
		"source.toolkit.fluxcd.io/v1/externalartifacts",
		"source.toolkit.fluxcd.io/v1/gitrepositories",
		"source.toolkit.fluxcd.io/v1/helmrepositories",
		"source.toolkit.fluxcd.io/v1/ocirepositories",
		"kustomize.toolkit.fluxcd.io/v1/kustomizations",

		// karpenter all resources
		"karpenter.sh/v1/",
		"karpenter.k8s.aws/v1/",
		"karpenter.azure.com/v1beta1/",

		// EKS Auto Mode NodeClass resource
		"eks.amazonaws.com/v1/nodeclasses",

		// Gateway API
		"gateway.networking.k8s.io/v1/gateways",
		"gateway.networking.k8s.io/v1/httproutes",
		"gateway.networking.k8s.io/v1/grpcroutes",
		"gateway.networking.k8s.io/v1alpha2/tlsroutes",
		"gateway.networking.k8s.io/v1alpha1/listenersets",

		// Service mesh — Istio
		"networking.istio.io/v1/virtualservices",
		"networking.istio.io/v1/gateways",
		"networking.istio.io/v1/destinationrules",
		"networking.istio.io/v1/serviceentries",
		"networking.istio.io/v1/sidecars",

		// Service mesh — other vendors (group-level)
		"gateway.envoyproxy.io/v1alpha1/",
		"traefik.containo.us/v1alpha1/",
		"policy.linkerd.io/v1beta3/",
		"consul.hashicorp.com/v1alpha1/",
		"mesh.consul.hashicorp.com/v2beta1/",
		"kuma.io/v1alpha1/",

		// Ingress controllers — NGINX
		"k8s.nginx.org/v1/virtualservers",
		"k8s.nginx.org/v1/virtualserverroutes",

		// Ingress controllers — Traefik
		"traefik.io/v1alpha1/ingressroutes",

		// Ingress controllers — other vendors (group-level)
		"configuration.konghq.com/v1/",
		"core.haproxy.org/v1alpha2/",
		"ingress.v1.haproxy.org/v1/",
	}

	// Verify all expected configs are present
	foundConfigs := make([]string, 0, len(configs))
	for _, config := range configs {
		gvk := config.group + "/" + config.preferredVersion + "/" + config.kind
		foundConfigs = append(foundConfigs, gvk)

		// Verify config structure
		require.NotEmpty(t, config.group, "Group should not be empty")
		require.NotEmpty(t, config.preferredVersion, "Version should not be empty")
	}

	require.ElementsMatch(t, expectedConfigs, foundConfigs)
}

func TestNewBuiltinCRDConfigsPerFamilyFlags(t *testing.T) {
	for _, testCase := range []struct {
		name                string
		ootbEnabled         bool
		gatewayAPI          bool
		serviceMesh         bool
		ingressControllers  bool
		expectedGatewayAPI  int // 5 entries
		expectedServiceMesh int // 11 entries (5 Istio + 6 group-level)
		expectedIngress     int // 6 entries (2 NGINX + 1 Traefik + 3 group-level)
	}{
		{
			name:                "all families enabled",
			ootbEnabled:         true,
			gatewayAPI:          true,
			serviceMesh:         true,
			ingressControllers:  true,
			expectedGatewayAPI:  5,
			expectedServiceMesh: 11,
			expectedIngress:     6,
		},
		{
			name:                "gateway_api disabled",
			ootbEnabled:         true,
			gatewayAPI:          false,
			serviceMesh:         true,
			ingressControllers:  true,
			expectedGatewayAPI:  0,
			expectedServiceMesh: 11,
			expectedIngress:     6,
		},
		{
			name:                "service_mesh disabled",
			ootbEnabled:         true,
			gatewayAPI:          true,
			serviceMesh:         false,
			ingressControllers:  true,
			expectedGatewayAPI:  5,
			expectedServiceMesh: 0,
			expectedIngress:     6,
		},
		{
			name:                "ingress_controllers disabled",
			ootbEnabled:         true,
			gatewayAPI:          true,
			serviceMesh:         true,
			ingressControllers:  false,
			expectedGatewayAPI:  5,
			expectedServiceMesh: 11,
			expectedIngress:     0,
		},
		{
			name:                "global ootb disabled overrides all",
			ootbEnabled:         false,
			gatewayAPI:          true,
			serviceMesh:         true,
			ingressControllers:  true,
			expectedGatewayAPI:  0,
			expectedServiceMesh: 0,
			expectedIngress:     0,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := mockconfig.New(t)
			cfg.SetInTest("orchestrator_explorer.custom_resources.ootb.enabled", testCase.ootbEnabled)
			cfg.SetInTest("orchestrator_explorer.custom_resources.ootb.gateway_api", testCase.gatewayAPI)
			cfg.SetInTest("orchestrator_explorer.custom_resources.ootb.service_mesh", testCase.serviceMesh)
			cfg.SetInTest("orchestrator_explorer.custom_resources.ootb.ingress_controllers", testCase.ingressControllers)

			configs := newBuiltinCRDConfigs()

			gatewayAPIGroups := map[string]bool{GatewayAPIGroup: true}
			serviceMeshGroups := map[string]bool{
				IstioNetworkingAPIGroup: true, EnvoyGatewayAPIGroup: true,
				TraefikLegacyAPIGroup: true, LinkerdPolicyAPIGroup: true,
				ConsulAPIGroup: true, ConsulMeshAPIGroup: true, KumaAPIGroup: true,
			}
			ingressGroups := map[string]bool{
				NginxAPIGroup: true, TraefikAPIGroup: true, KongAPIGroup: true,
				HAProxyCoreAPIGroup: true, HAProxyIngressV1APIGroup: true,
			}

			var gatewayCount, meshCount, ingressCount int
			for _, c := range configs {
				if !c.enabled {
					continue
				}
				switch {
				case gatewayAPIGroups[c.group]:
					gatewayCount++
				case serviceMeshGroups[c.group]:
					meshCount++
				case ingressGroups[c.group]:
					ingressCount++
				}
			}

			require.Equal(t, testCase.expectedGatewayAPI, gatewayCount, "gateway API count mismatch")
			require.Equal(t, testCase.expectedServiceMesh, meshCount, "service mesh count mismatch")
			require.Equal(t, testCase.expectedIngress, ingressCount, "ingress controllers count mismatch")
		})
	}
}

func TestFilterCRCollectorsByPermission(t *testing.T) {
	// Create test collectors
	c1, err := k8s.NewCRCollector("datadogmetrics", "datadoghq.com/v1alpha1")
	require.NoError(t, err)
	c2, err := k8s.NewCRCollector("datadogmonitors", "datadoghq.com/v1alpha1")
	require.NoError(t, err)
	c3, err := k8s.NewCRCollector("datadogagents", "datadoghq.com/v1alpha2")
	require.NoError(t, err)

	// Test case 1: All collectors have permissions
	t.Run("All collectors have permissions", func(t *testing.T) {
		permissionMap := map[string]bool{
			"datadoghq.com/v1alpha1/datadogmetrics":  false, // false means NOT forbidden
			"datadoghq.com/v1alpha1/datadogmonitors": false,
			"datadoghq.com/v1alpha2/datadogagents":   false,
		}

		isForbidden := func(gvr schema.GroupVersionResource) bool {
			key := gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
			return permissionMap[key]
		}

		input := []collectors.K8sCollector{c1, c2, c3}
		result := filterCRCollectorsByPermission(input, isForbidden)

		require.Len(t, result, 3)
		names := make([]string, len(result))
		for i, collector := range result {
			names[i] = collector.Metadata().FullName()
		}
		require.ElementsMatch(t, []string{
			"datadoghq.com/v1alpha1/datadogmetrics",
			"datadoghq.com/v1alpha1/datadogmonitors",
			"datadoghq.com/v1alpha2/datadogagents",
		}, names)
	})

	// Test case 2: No collectors have permissions
	t.Run("No collectors have permissions", func(t *testing.T) {
		isForbidden := func(_ schema.GroupVersionResource) bool {
			return true // All resources are forbidden
		}

		input := []collectors.K8sCollector{c1, c2, c3}
		result := filterCRCollectorsByPermission(input, isForbidden)

		require.Len(t, result, 0)
	})

	// Test case 3: Mixed permissions
	t.Run("Mixed permissions", func(t *testing.T) {
		permissionMap := map[string]bool{
			"datadoghq.com/v1alpha1/datadogmetrics":  false, // false means NOT forbidden (allowed)
			"datadoghq.com/v1alpha1/datadogmonitors": true,  // true means forbidden (not allowed)
			"datadoghq.com/v1alpha2/datadogagents":   false,
		}

		isForbidden := func(gvr schema.GroupVersionResource) bool {
			key := gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
			return permissionMap[key]
		}

		input := []collectors.K8sCollector{c1, c2, c3}
		result := filterCRCollectorsByPermission(input, isForbidden)

		require.Len(t, result, 2)
		names := make([]string, len(result))
		for i, collector := range result {
			names[i] = collector.Metadata().FullName()
		}
		require.ElementsMatch(t, []string{
			"datadoghq.com/v1alpha1/datadogmetrics",
			"datadoghq.com/v1alpha2/datadogagents",
		}, names)
	})

	// Test case 4: Empty input
	t.Run("Empty input", func(t *testing.T) {
		isForbidden := func(_ schema.GroupVersionResource) bool {
			return true // All resources are forbidden (doesn't matter for empty input)
		}
		result := filterCRCollectorsByPermission([]collectors.K8sCollector{}, isForbidden)
		require.Len(t, result, 0)
	})
}
