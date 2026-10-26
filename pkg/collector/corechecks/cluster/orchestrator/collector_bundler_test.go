// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	crdfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sdiscovery "k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
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
			{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogmetrics"}:          "DatadogMetricList",
			{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogmonitors"}:         "DatadogMonitorList",
			{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogslos"}:             "DatadogSloList",
			{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogdashboards"}:       "DatadogDashboardList",
			{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogagentprofiles"}:    "DatadogAgentProfileList",
			{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogpodautoscalers"}:   "DatadogPodAutoscalerList",
			{Group: "datadoghq.com", Version: "v1alpha2", Resource: "datadogpodautoscalers"}:   "DatadogPodAutoscalerList",
			{Group: "datadoghq.com", Version: "v2alpha1", Resource: "datadogagents"}:           "DatadogAgentList",
			{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadoginstrumentations"}: "DatadogInstrumentationList",
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
			{GroupVersion: "v1", Kind: "pods"}:                                        {},
			{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogmetrics"}:          {},
			{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogmonitors"}:         {},
			{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogslos"}:             {},
			{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogdashboards"}:       {},
			{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogagentprofiles"}:    {},
			{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadogpodautoscalers"}:   {},
			{GroupVersion: "datadoghq.com/v1alpha2", Kind: "datadogpodautoscalers"}:   {},
			{GroupVersion: "datadoghq.com/v2alpha1", Kind: "datadogagents"}:           {},
			{GroupVersion: "datadoghq.com/v1alpha1", Kind: "datadoginstrumentations"}: {},
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
		"datadoghq.com/v1alpha1/datadoginstrumentations",
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
		name                  string
		terminatedPodsEnabled bool
		unassignedPod         bool
		expected              collectors.K8sCollector
	}{
		{
			name:                  "Terminated pods collector disabled",
			terminatedPodsEnabled: false,
			unassignedPod:         true,
			expected:              nil,
		},
		{
			name:                  "Terminated pods collector enabled",
			terminatedPodsEnabled: true,
			unassignedPod:         true,
			expected:              k8s.NewTerminatedPodCollector(nil, nil, nil),
		},
		{
			name:                  "Terminated pods collector enabled without unassigned pod collector",
			terminatedPodsEnabled: true,
			unassignedPod:         false,
			expected:              nil,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg.SetInTest("orchestrator_explorer.terminated_pods.enabled", testCase.terminatedPodsEnabled)

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
		"datadoghq.com/v1alpha1/datadoginstrumentations",

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

		// Dynamo
		"nvidia.com/v1alpha1/dynamocheckpoints",
		"nvidia.com/v1beta1/dynamocomponentdeployments",
		"nvidia.com/v1beta1/dynamographdeploymentrequests",
		"nvidia.com/v1beta1/dynamographdeployments",
		"nvidia.com/v1beta1/dynamographdeploymentscalingadapters",
		"nvidia.com/v1alpha1/dynamomodels",
		"nvidia.com/v1alpha1/dynamoworkermetadatas",

		// KubeRay
		"ray.io/v1/rayclusters",
		"ray.io/v1/raycronjobs",
		"ray.io/v1/rayjobs",
		"ray.io/v1/rayservices",

		// KubeAI
		"kubeai.org/v1/models",

		// KServe
		"serving.kserve.io/v1alpha1/clusterstoragecontainers",
		"serving.kserve.io/v1alpha2/llminferenceserviceconfigs",
		"serving.kserve.io/v1alpha2/llminferenceservices",
		"serving.kserve.io/v1alpha1/localmodelcaches",
		"serving.kserve.io/v1alpha1/localmodelnamespacecaches",
		"serving.kserve.io/v1alpha1/localmodelnodegroups",
		"serving.kserve.io/v1alpha1/localmodelnodes",
		"serving.kserve.io/v1alpha1/clusterservingruntimes",
		"serving.kserve.io/v1alpha1/inferencegraphs",
		"serving.kserve.io/v1beta1/inferenceservices",
		"serving.kserve.io/v1alpha1/servingruntimes",
		"serving.kserve.io/v1alpha1/trainedmodels",

		// Gateway API Inference Extension
		"inference.networking.k8s.io/v1/inferencepools",

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

func TestBuiltinGAIECollector(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		disabled  bool
		version   string
		collected bool
	}{
		{name: "v1 collected by default", version: "v1", collected: true},
		{name: "global OOTB disabled", disabled: true, version: "v1"},
		{name: "API group absent"},
		{name: "unsupported version", version: "v1alpha2"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := mockconfig.New(t)
			if testCase.disabled {
				cfg.SetInTest("orchestrator_explorer.custom_resources.ootb.enabled", false)
			}
			// GAIE collection does not require opting into the Gateway API family.
			cfg.SetInTest("orchestrator_explorer.custom_resources.ootb.gateway_api", false)

			cache := discovery.DiscoveryCache{
				CollectorForVersion: map[discovery.CollectorVersion]struct{}{},
			}
			if testCase.version != "" {
				cache.CollectorForVersion[discovery.CollectorVersion{
					GroupVersion: GAIEAPIGroup + "/" + testCase.version,
					Kind:         "inferencepools",
				}] = struct{}{}
				cache.Groups = []*v1.APIGroup{{
					Name: GAIEAPIGroup,
					Versions: []v1.GroupVersionForDiscovery{{
						GroupVersion: GAIEAPIGroup + "/" + testCase.version,
						Version:      testCase.version,
					}},
				}}
			}
			collectorDiscovery := &discovery.DiscoveryCollector{}
			collectorDiscovery.SetCache(cache)
			cb := CollectorBundle{collectorDiscovery: collectorDiscovery}
			for _, config := range newBuiltinCRDConfigs() {
				if config.group != GAIEAPIGroup {
					continue
				}
				crCollectors := cb.collectorsForBuiltinCRD(config)
				if testCase.collected {
					require.Len(t, crCollectors, 1)
					require.Equal(t, "inference.networking.k8s.io/v1/inferencepools", crCollectors[0].Metadata().FullName())
				} else {
					require.Empty(t, crCollectors)
				}
			}
		})
	}
}

func TestNewBuiltinKServeCRDConfigs(t *testing.T) {
	cfg := mockconfig.New(t)
	cfg.SetInTest("orchestrator_explorer.custom_resources.ootb.enabled", true)

	var kserveConfigs []builtinCRDConfig
	for _, config := range newBuiltinCRDConfigs() {
		if config.group == KServeAPIGroup {
			kserveConfigs = append(kserveConfigs, config)
		}
	}

	expectedConfigs := []builtinCRDConfig{
		newBuiltinCRDConfig(KServeAPIGroup, "clusterstoragecontainers", true, "v1alpha1"),
		newBuiltinCRDConfig(KServeAPIGroup, "llminferenceserviceconfigs", true, "v1alpha2", "v1alpha1"),
		newBuiltinCRDConfig(KServeAPIGroup, "llminferenceservices", true, "v1alpha2", "v1alpha1"),
		newBuiltinCRDConfig(KServeAPIGroup, "localmodelcaches", true, "v1alpha1"),
		newBuiltinCRDConfig(KServeAPIGroup, "localmodelnamespacecaches", true, "v1alpha1"),
		newBuiltinCRDConfig(KServeAPIGroup, "localmodelnodegroups", true, "v1alpha1"),
		newBuiltinCRDConfig(KServeAPIGroup, "localmodelnodes", true, "v1alpha1"),
		newBuiltinCRDConfig(KServeAPIGroup, "clusterservingruntimes", true, "v1alpha1"),
		newBuiltinCRDConfig(KServeAPIGroup, "inferencegraphs", true, "v1alpha1"),
		newBuiltinCRDConfig(KServeAPIGroup, "inferenceservices", true, "v1beta1"),
		newBuiltinCRDConfig(KServeAPIGroup, "servingruntimes", true, "v1alpha1"),
		newBuiltinCRDConfig(KServeAPIGroup, "trainedmodels", true, "v1alpha1"),
	}

	require.ElementsMatch(t, expectedConfigs, kserveConfigs)
}

func TestNewBuiltinCRDConfigsPerFamilyFlags(t *testing.T) {
	for _, testCase := range []struct {
		name                string
		useDefaults         bool
		ootbEnabled         bool
		gatewayAPI          bool
		serviceMesh         bool
		ingressControllers  bool
		expectedGatewayAPI  int // 5 entries
		expectedServiceMesh int // 11 entries (5 Istio + 6 group-level)
		expectedIngress     int // 6 entries (2 NGINX + 1 Traefik + 3 group-level)
	}{
		{
			name:                "all families enabled by default",
			useDefaults:         true,
			expectedGatewayAPI:  5,
			expectedServiceMesh: 11,
			expectedIngress:     6,
		},
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
			if !testCase.useDefaults {
				cfg.SetInTest("orchestrator_explorer.custom_resources.ootb.enabled", testCase.ootbEnabled)
				cfg.SetInTest("orchestrator_explorer.custom_resources.ootb.gateway_api", testCase.gatewayAPI)
				cfg.SetInTest("orchestrator_explorer.custom_resources.ootb.service_mesh", testCase.serviceMesh)
				cfg.SetInTest("orchestrator_explorer.custom_resources.ootb.ingress_controllers", testCase.ingressControllers)
			}

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

func TestCollectorBundleRefreshDiscovery(t *testing.T) {
	var (
		appsGroup = discoveryGroup{
			Group: &v1.APIGroup{
				Name:             "apps",
				Versions:         []v1.GroupVersionForDiscovery{{GroupVersion: "apps/v1", Version: "v1"}},
				PreferredVersion: v1.GroupVersionForDiscovery{GroupVersion: "apps/v1", Version: "v1"},
			},
			Resources: &v1.APIResourceList{
				GroupVersion: "apps/v1",
				APIResources: []v1.APIResource{{Name: "deployments"}},
			},
		}

		batchGroup = discoveryGroup{
			Group: &v1.APIGroup{
				Name:             "batch",
				Versions:         []v1.GroupVersionForDiscovery{{GroupVersion: "batch/v1", Version: "v1"}},
				PreferredVersion: v1.GroupVersionForDiscovery{GroupVersion: "batch/v1", Version: "v1"},
			},
			Resources: &v1.APIResourceList{
				GroupVersion: "batch/v1",
				APIResources: []v1.APIResource{{Name: "cronjobs"}, {Name: "jobs"}},
			},
		}

		crGroup1 = discoveryGroup{
			Group: &v1.APIGroup{
				Name:             "group1",
				Versions:         []v1.GroupVersionForDiscovery{{GroupVersion: "group1/v1", Version: "v1"}},
				PreferredVersion: v1.GroupVersionForDiscovery{GroupVersion: "group1/v1", Version: "v1"},
			},
			Resources: &v1.APIResourceList{
				GroupVersion: "group1/v1",
				APIResources: []v1.APIResource{{Name: "cr-kind-1"}},
			},
		}

		crGroup2 = discoveryGroup{
			Group: &v1.APIGroup{
				Name:             "group2",
				Versions:         []v1.GroupVersionForDiscovery{{GroupVersion: "group2/v1", Version: "v1"}},
				PreferredVersion: v1.GroupVersionForDiscovery{GroupVersion: "group2/v1", Version: "v1"},
			},
			Resources: &v1.APIResourceList{
				GroupVersion: "group2/v1",
				APIResources: []v1.APIResource{{Name: "cr-kind-2"}},
			},
		}

		crdGroup = discoveryGroup{
			Group: &v1.APIGroup{
				Name:             "apiextensions.k8s.io",
				Versions:         []v1.GroupVersionForDiscovery{{GroupVersion: "apiextensions.k8s.io/v1", Version: "v1"}},
				PreferredVersion: v1.GroupVersionForDiscovery{GroupVersion: "apiextensions.k8s.io/v1", Version: "v1"},
			},
			Resources: &v1.APIResourceList{
				GroupVersion: "apiextensions.k8s.io/v1",
				APIResources: []v1.APIResource{{Name: "customresourcedefinitions"}},
			},
		}

		crdDatadogGroup = discoveryGroup{
			Group: &v1.APIGroup{
				Name:             "datadoghq.com",
				Versions:         []v1.GroupVersionForDiscovery{{GroupVersion: "datadoghq.com/v1alpha1", Version: "v1alpha1"}},
				PreferredVersion: v1.GroupVersionForDiscovery{GroupVersion: "datadoghq.com/v1alpha1", Version: "v1alpha1"},
			},
			Resources: &v1.APIResourceList{GroupVersion: "datadoghq.com/v1alpha1", APIResources: []v1.APIResource{{Name: "datadogmetrics"}}},
		}

		crdArgoGroup = discoveryGroup{
			Group: &v1.APIGroup{
				Name:             "argoproj.io",
				Versions:         []v1.GroupVersionForDiscovery{{GroupVersion: "argoproj.io/v1alpha1", Version: "v1alpha1"}},
				PreferredVersion: v1.GroupVersionForDiscovery{GroupVersion: "argoproj.io/v1alpha1", Version: "v1alpha1"},
			},
			Resources: &v1.APIResourceList{
				GroupVersion: "argoproj.io/v1alpha1",
				APIResources: []v1.APIResource{{Name: "rollouts"}},
			},
		}

		unsupportedGroup = discoveryGroup{
			Group: &v1.APIGroup{
				Name:             "unsupported",
				Versions:         []v1.GroupVersionForDiscovery{{GroupVersion: "unsupported/v1", Version: "v1"}},
				PreferredVersion: v1.GroupVersionForDiscovery{GroupVersion: "unsupported/v1", Version: "v1"},
			},
			Resources: &v1.APIResourceList{
				GroupVersion: "unsupported/v1",
				APIResources: []v1.APIResource{{Name: "resource-kind-1"}},
			},
		}

		crCollectors = []string{"group1/v1/cr-kind-1", "group2/v1/cr-kind-2"}
	)

	t.Run("Success - Regular collectors discovery - partial failure recovery", func(t *testing.T) {
		settings := map[string]any{"orchestrator_explorer.custom_resources.ootb.enabled": false}
		fetchMock := newDiscoveryFetchMock(
			// First fetch is a partial failure
			discoveryFetchMockResult{
				err:       newGroupDiscoveryError(schema.GroupVersion{Group: "batch", Version: "v1"}),
				groups:    []*v1.APIGroup{appsGroup.Group},
				resources: []*v1.APIResourceList{appsGroup.Resources},
			},
			// Second fetch is a success
			discoveryFetchMockResult{
				groups:    []*v1.APIGroup{appsGroup.Group, batchGroup.Group},
				resources: []*v1.APIResourceList{appsGroup.Resources, batchGroup.Resources},
			},
		)

		cb := newRetryTestBundle(t, []string{}, true, fetchMock.fetch, settings)

		// First fetch
		cb.prepare()
		expectedCollectors := collectorNamesOf(appsGroup)
		require.Equal(t, expectedCollectors, collectorNames(cb))
		require.Contains(t, warningMessages(cb), "Kubernetes API discovery failed, skipping 1 check run(s) before retrying: unable to retrieve the complete list of server APIs: batch/v1: service unavailable")
		require.Equal(t, 1, fetchMock.calls)

		// Skipped run
		cb.RefreshDiscovery()
		require.Equal(t, expectedCollectors, collectorNames(cb))
		require.Equal(t, 1, fetchMock.calls)

		// Second fetch
		cb.RefreshDiscovery()
		expectedCollectors = collectorNamesOf(appsGroup, batchGroup)
		require.Equal(t, expectedCollectors, collectorNames(cb))
		require.Equal(t, 2, fetchMock.calls)
	})
	t.Run("Success - Regular collectors discovery - repeated partial failure", func(t *testing.T) {
		settings := map[string]any{"orchestrator_explorer.custom_resources.ootb.enabled": false}
		fetchMock := newDiscoveryFetchMock(
			// Every fetch is the same partial failure
			discoveryFetchMockResult{
				err:       newGroupDiscoveryError(schema.GroupVersion{Group: "batch", Version: "v1"}),
				groups:    []*v1.APIGroup{appsGroup.Group},
				resources: []*v1.APIResourceList{appsGroup.Resources},
			},
		)

		cb := newRetryTestBundle(t, []string{}, true, fetchMock.fetch, settings)

		// First fetch
		cb.prepare()
		expectedCollectors := collectorNamesOf(appsGroup)
		require.Equal(t, expectedCollectors, collectorNames(cb))
		require.Equal(t, 1, fetchMock.calls)

		// Retries discovering nothing new skip 1, 2 then 4 runs
		expectedCalls := []int{1, 2, 2, 2, 3, 3, 3, 3, 3, 4}
		for run, calls := range expectedCalls {
			cb.RefreshDiscovery()
			require.Equal(t, calls, fetchMock.calls, "run %d", run+1)
			require.Equal(t, expectedCollectors, collectorNames(cb), "run %d", run+1)
		}
		require.Contains(t, warningMessages(cb), "Kubernetes API discovery retry failed, skipping 8 check run(s) before retrying: unable to retrieve the complete list of server APIs: batch/v1: service unavailable")
	})
	t.Run("Success - Regular collectors discovery - total failure recovery", func(t *testing.T) {
		settings := map[string]any{"orchestrator_explorer.custom_resources.ootb.enabled": false}
		fetchMock := newDiscoveryFetchMock(
			// First fetch is a total failure
			discoveryFetchMockResult{err: errors.New("discovery total failure")},
			// Second fetch is a success
			discoveryFetchMockResult{
				groups:    []*v1.APIGroup{appsGroup.Group, batchGroup.Group},
				resources: []*v1.APIResourceList{appsGroup.Resources, batchGroup.Resources},
			},
		)

		cb := newRetryTestBundle(t, []string{}, true, fetchMock.fetch, settings)

		// First fetch
		cb.prepare()
		require.Empty(t, collectorNames(cb))
		require.Contains(t, warningMessages(cb), "Kubernetes API discovery failed, skipping 1 check run(s) before retrying: discovery total failure")
		require.Equal(t, 1, fetchMock.calls)

		// Skipped run
		cb.RefreshDiscovery()
		require.Empty(t, collectorNames(cb))
		require.Equal(t, 1, fetchMock.calls)

		// Second fetch
		cb.RefreshDiscovery()
		expectedCollectors := collectorNamesOf(appsGroup, batchGroup)
		require.Equal(t, expectedCollectors, collectorNames(cb))
		require.Equal(t, 2, fetchMock.calls)
	})
	t.Run("Success - CR collectors discovery - partial failure recovery", func(t *testing.T) {
		settings := map[string]any{"orchestrator_explorer.custom_resources.ootb.enabled": false}
		fetchMock := newDiscoveryFetchMock(
			// First fetch is a partial failure
			discoveryFetchMockResult{
				err:       newGroupDiscoveryError(schema.GroupVersion{Group: "group2", Version: "v1"}),
				groups:    []*v1.APIGroup{crGroup1.Group},
				resources: []*v1.APIResourceList{crGroup1.Resources},
			},
			// Second fetch is a success
			discoveryFetchMockResult{
				groups:    []*v1.APIGroup{crGroup1.Group, crGroup2.Group},
				resources: []*v1.APIResourceList{crGroup1.Resources, crGroup2.Resources},
			},
		)

		cb := newRetryTestBundle(t, []string{}, true, fetchMock.fetch, settings)
		cb.check.instance.CRDCollectors = crCollectors

		// First fetch
		cb.prepare()
		expectedCollectors := collectorNamesOf(crGroup1)
		require.Equal(t, expectedCollectors, collectorNames(cb))
		require.Contains(t, warningMessages(cb), "Kubernetes API discovery failed, skipping 1 check run(s) before retrying: unable to retrieve the complete list of server APIs: group2/v1: service unavailable")
		require.Equal(t, 1, fetchMock.calls)

		// Skipped run
		cb.RefreshDiscovery()
		require.Equal(t, expectedCollectors, collectorNames(cb))
		require.Equal(t, 1, fetchMock.calls)

		// Second fetch
		cb.RefreshDiscovery()
		expectedCollectors = collectorNamesOf(crGroup1, crGroup2)
		require.Equal(t, expectedCollectors, collectorNames(cb))
		require.Equal(t, 2, fetchMock.calls)
	})
	t.Run("Success - CR collectors discovery - total failure recovery", func(t *testing.T) {
		settings := map[string]any{"orchestrator_explorer.custom_resources.ootb.enabled": false}
		fetchMock := newDiscoveryFetchMock(
			// First fetch is a total failure
			discoveryFetchMockResult{err: errors.New("discovery total failure")},
			// Second fetch is a success
			discoveryFetchMockResult{
				groups:    []*v1.APIGroup{crGroup1.Group, crGroup2.Group},
				resources: []*v1.APIResourceList{crGroup1.Resources, crGroup2.Resources},
			},
		)

		cb := newRetryTestBundle(t, []string{}, true, fetchMock.fetch, settings)
		cb.check.instance.CRDCollectors = crCollectors

		// First fetch
		cb.prepare()
		require.Empty(t, collectorNames(cb))
		require.Contains(t, warningMessages(cb), "Kubernetes API discovery failed, skipping 1 check run(s) before retrying: discovery total failure")
		require.Equal(t, 1, fetchMock.calls)

		// Skipped run
		cb.RefreshDiscovery()
		require.Empty(t, collectorNames(cb))
		require.Equal(t, 1, fetchMock.calls)

		// Second fetch
		cb.RefreshDiscovery()
		expectedCollectors := collectorNamesOf(crGroup1, crGroup2)
		require.Equal(t, expectedCollectors, collectorNames(cb))
		require.Equal(t, 2, fetchMock.calls)
	})
	t.Run("Success - CR builtin collectors discovery - partial failure recovery", func(t *testing.T) {
		settings := map[string]any{"orchestrator_explorer.custom_resources.ootb.enabled": true}
		fetchMock := newDiscoveryFetchMock(
			// First fetch is a partial failure
			discoveryFetchMockResult{
				err:       newGroupDiscoveryError(schema.GroupVersion{Group: "datadoghq.com", Version: "v1alpha1"}),
				groups:    []*v1.APIGroup{crdGroup.Group, crdArgoGroup.Group},
				resources: []*v1.APIResourceList{crdGroup.Resources, crdArgoGroup.Resources},
			},
			// Second fetch is a success
			discoveryFetchMockResult{
				groups:    []*v1.APIGroup{crdGroup.Group, crdArgoGroup.Group, crdDatadogGroup.Group},
				resources: []*v1.APIResourceList{crdGroup.Resources, crdArgoGroup.Resources, crdDatadogGroup.Resources},
			},
		)

		cb := newRetryTestBundle(t, []string{}, true, fetchMock.fetch, settings)

		// First fetch
		cb.prepare()
		expectedCollectors := collectorNamesOf(crdGroup, crdArgoGroup)
		require.Equal(t, expectedCollectors, collectorNames(cb))
		require.Contains(t, warningMessages(cb), "Kubernetes API discovery failed, skipping 1 check run(s) before retrying: unable to retrieve the complete list of server APIs: datadoghq.com/v1alpha1: service unavailable")
		require.Equal(t, 1, fetchMock.calls)

		// Skipped run
		cb.RefreshDiscovery()
		require.Equal(t, expectedCollectors, collectorNames(cb))
		require.Equal(t, 1, fetchMock.calls)

		// Actual retry
		cb.RefreshDiscovery()
		expectedCollectors = collectorNamesOf(crdGroup, crdArgoGroup, crdDatadogGroup)
		require.Equal(t, expectedCollectors, collectorNames(cb))
		require.Equal(t, 2, fetchMock.calls)
	})
	t.Run("Success - CR builtin collectors discovery - total failure recovery", func(t *testing.T) {
		settings := map[string]any{"orchestrator_explorer.custom_resources.ootb.enabled": true}
		fetchMock := newDiscoveryFetchMock(
			// First fetch is a total failure
			discoveryFetchMockResult{err: errors.New("discovery total failure")},
			// Second fetch is a success
			discoveryFetchMockResult{
				groups:    []*v1.APIGroup{crdGroup.Group, crdArgoGroup.Group, crdDatadogGroup.Group},
				resources: []*v1.APIResourceList{crdGroup.Resources, crdArgoGroup.Resources, crdDatadogGroup.Resources},
			},
		)

		cb := newRetryTestBundle(t, []string{}, true, fetchMock.fetch, settings)

		// First fetch
		cb.prepare()
		require.Empty(t, collectorNames(cb))
		require.Contains(t, warningMessages(cb), "Kubernetes API discovery failed, skipping 1 check run(s) before retrying: discovery total failure")
		require.Equal(t, 1, fetchMock.calls)

		// Skipped run
		cb.RefreshDiscovery()
		require.Empty(t, collectorNames(cb))
		require.Equal(t, 1, fetchMock.calls)

		// Second fetch
		cb.RefreshDiscovery()
		expectedCollectors := collectorNamesOf(crdGroup, crdArgoGroup, crdDatadogGroup)
		require.Equal(t, expectedCollectors, collectorNames(cb))
		require.Equal(t, 2, fetchMock.calls)
	})
	t.Run("Success - Unsupported group - partial failure", func(t *testing.T) {
		fetchMock := newDiscoveryFetchMock(
			// First fetch is a partial failure
			discoveryFetchMockResult{
				err:       newGroupDiscoveryError(schema.GroupVersion{Group: "unsupported", Version: "v1"}),
				groups:    []*v1.APIGroup{crdGroup.Group},
				resources: []*v1.APIResourceList{crdGroup.Resources},
			},
			// Second fetch is a success
			discoveryFetchMockResult{
				groups:    []*v1.APIGroup{crdGroup.Group, unsupportedGroup.Group},
				resources: []*v1.APIResourceList{crdGroup.Resources, unsupportedGroup.Resources},
			},
		)

		cb := newRetryTestBundle(t, nil, true, fetchMock.fetch, nil)

		// First fetch
		cb.prepare()
		require.Equal(t, collectorNamesOf(crdGroup), collectorNames(cb))
		require.Empty(t, warningMessages(cb))
		require.Equal(t, 1, fetchMock.calls)

		// What would trigger a skip run and retry with a supported group should not trigger a retry there.
		for range 2 {
			cb.RefreshDiscovery()
			require.Equal(t, collectorNamesOf(crdGroup), collectorNames(cb))
			require.Equal(t, 1, fetchMock.calls)
		}
	})
	t.Run("Success - Unsupported group - total failure", func(t *testing.T) {
		fetchMock := newDiscoveryFetchMock(
			// First fetch is a total failure
			discoveryFetchMockResult{err: errors.New("discovery total failure")},
			// Second fetch is a success
			discoveryFetchMockResult{
				groups:    []*v1.APIGroup{unsupportedGroup.Group},
				resources: []*v1.APIResourceList{unsupportedGroup.Resources},
			},
		)

		cb := newRetryTestBundle(t, nil, true, fetchMock.fetch, nil)

		// First fetch
		cb.prepare()
		require.Empty(t, collectorNames(cb))
		require.Contains(t, warningMessages(cb), "Kubernetes API discovery failed, skipping 1 check run(s) before retrying: discovery total failure")
		require.Equal(t, 1, fetchMock.calls)

		// Skipped run
		cb.RefreshDiscovery()
		require.Empty(t, collectorNames(cb))
		require.Equal(t, 1, fetchMock.calls)

		// Second fetch, no change the selected collectors
		cb.RefreshDiscovery()
		require.Empty(t, collectorNames(cb))
		require.Equal(t, 2, fetchMock.calls)
	})
	t.Run("Success - Complete discovery is not retried", func(t *testing.T) {
		fetchMock := newDiscoveryFetchMock(
			// First fetch is a success
			discoveryFetchMockResult{
				groups:    []*v1.APIGroup{appsGroup.Group},
				resources: []*v1.APIResourceList{appsGroup.Resources},
			},
		)

		configuredCollectors := collectorNamesOf(appsGroup)
		cb := newRetryTestBundle(t, configuredCollectors, true, fetchMock.fetch, nil)

		// First fetch
		cb.prepare()
		require.Equal(t, configuredCollectors, collectorNames(cb))
		require.Equal(t, 1, fetchMock.calls)

		// The next two calls must be no-ops.
		for range 2 {
			cb.RefreshDiscovery()
			require.Equal(t, configuredCollectors, collectorNames(cb))
			require.Equal(t, 1, fetchMock.calls)
		}
	})
}

func TestCollectorBundleMergeCollectors(t *testing.T) {
	var (
		cronjobV1Beta1Collector = k8s.NewCronJobV1Beta1Collector(nil)
		cronjobV1Collector      = k8s.NewCronJobV1Collector(nil)
		jobCollector            = k8s.NewJobCollector(nil)
	)

	cases := []struct {
		name     string
		previous []collectors.K8sCollector
		selected []collectors.K8sCollector
		expected []collectors.K8sCollector
	}{
		{
			name:     "Success - New selected collector",
			previous: []collectors.K8sCollector{cronjobV1Collector},
			selected: []collectors.K8sCollector{jobCollector},
			expected: []collectors.K8sCollector{cronjobV1Collector, jobCollector},
		},
		{
			name:     "Success - No previous collector",
			selected: []collectors.K8sCollector{jobCollector},
			expected: []collectors.K8sCollector{jobCollector},
		},
		{
			name:     "Success - No selected collector",
			previous: []collectors.K8sCollector{jobCollector},
			expected: []collectors.K8sCollector{jobCollector},
		},
		{
			name:     "Success - Previous collector kept for the same resource",
			previous: []collectors.K8sCollector{cronjobV1Collector},
			selected: []collectors.K8sCollector{cronjobV1Beta1Collector},
			expected: []collectors.K8sCollector{cronjobV1Collector},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cb := CollectorBundle{collectors: tt.selected}
			cb.mergeCollectors(tt.previous)
			assert.Equal(t, tt.expected, cb.collectors)
		})
	}
}

func TestCollectorBundleInitialize(t *testing.T) {
	client := k8sfake.NewClientset()
	apiClient := &apiserver.APIClient{Cl: client, InformerCl: client}
	stopCh := make(chan struct{})
	cb := CollectorBundle{
		collectors:            []collectors.K8sCollector{k8s.NewDeploymentCollector(nil)},
		initializedCollectors: map[string]struct{}{},
		runCfg: &collectors.CollectorRunConfig{
			Config: &orchcfg.OrchestratorConfig{},
			K8sCollectorRunConfig: collectors.K8sCollectorRunConfig{
				APIClient:                   apiClient,
				OrchestratorInformerFactory: getOrchestratorInformerFactory(apiClient),
			},
		},
		startedInformers: map[cache.Store]struct{}{},
		stopCh:           stopCh,
	}

	// close the stop channel when the test is done
	t.Cleanup(func() { close(stopCh) })

	// Initially selected collector is initialized.

	cb.Initialize()

	expectedInitializedCollectors := map[string]struct{}{
		"apps/v1/deployments": {},
	}
	expectedStartedInformers := map[cache.Store]struct{}{
		cb.collectors[0].Informer().GetStore(): {},
	}

	require.Equal(t, expectedInitializedCollectors, cb.initializedCollectors)
	require.Equal(t, expectedStartedInformers, cb.startedInformers)

	// A new selected collector after a discovery retry is initialized.

	cb.collectors = append(cb.collectors, k8s.NewJobCollector(nil))
	cb.Initialize()

	expectedInitializedCollectors = map[string]struct{}{
		"apps/v1/deployments": {},
		"batch/v1/jobs":       {},
	}
	expectedStartedInformers = map[cache.Store]struct{}{
		cb.collectors[0].Informer().GetStore(): {},
		cb.collectors[1].Informer().GetStore(): {},
	}

	require.Equal(t, expectedInitializedCollectors, cb.initializedCollectors)
	require.Equal(t, expectedStartedInformers, cb.startedInformers)
}

func TestCollectorBundleRefreshDiscoveryAfterCRDChange(t *testing.T) {
	var (
		crdGroup = discoveryGroup{
			Group: &v1.APIGroup{
				Name:     "apiextensions.k8s.io",
				Versions: []v1.GroupVersionForDiscovery{{GroupVersion: "apiextensions.k8s.io/v1", Version: "v1"}},
			},
			Resources: &v1.APIResourceList{
				GroupVersion: "apiextensions.k8s.io/v1",
				APIResources: []v1.APIResource{{Name: "customresourcedefinitions"}},
			},
		}
		datadogGroup = discoveryGroup{
			Group: &v1.APIGroup{
				Name:     "datadoghq.com",
				Versions: []v1.GroupVersionForDiscovery{{GroupVersion: "datadoghq.com/v1alpha1", Version: "v1alpha1"}},
			},
			Resources: &v1.APIResourceList{
				GroupVersion: "datadoghq.com/v1alpha1",
				APIResources: []v1.APIResource{{Name: "datadogmetrics"}},
			},
		}
		configuredGroup = discoveryGroup{
			Group: &v1.APIGroup{
				Name:     "group1",
				Versions: []v1.GroupVersionForDiscovery{{GroupVersion: "group1/v1", Version: "v1"}},
			},
			Resources: &v1.APIResourceList{
				GroupVersion: "group1/v1",
				APIResources: []v1.APIResource{{Name: "cr-kind-1"}},
			},
		}
		beforeCRDs = discoveryFetchMockResult{
			groups:    []*v1.APIGroup{crdGroup.Group},
			resources: []*v1.APIResourceList{crdGroup.Resources},
		}
		afterCRDs = discoveryFetchMockResult{
			groups:    []*v1.APIGroup{crdGroup.Group, datadogGroup.Group, configuredGroup.Group},
			resources: []*v1.APIResourceList{crdGroup.Resources, datadogGroup.Resources, configuredGroup.Resources},
		}
	)

	newBundle := func(t *testing.T, fetchMock *discoveryFetchMock) *CollectorBundle {
		cb := newRetryTestBundle(t, []string{"customresourcedefinitions"}, false, fetchMock.fetch, nil)
		cb.check.instance.CRDCollectors = []string{"group1/v1/cr-kind-1"}
		cb.prepare()
		require.Equal(t, collectorNamesOf(crdGroup), collectorNames(cb))
		return cb
	}

	t.Run("CRDs installed after the check started are collected", func(t *testing.T) {
		fetchMock := newDiscoveryFetchMock(beforeCRDs, afterCRDs)
		cb := newBundle(t, fetchMock)

		// Complete discovery and no CRD change, no refresh
		cb.RefreshDiscovery()
		require.Equal(t, 1, fetchMock.calls)

		cb.crdsChanged.Store(true)
		cb.RefreshDiscovery()
		require.Equal(t, 2, fetchMock.calls)
		require.Equal(t, collectorNamesOf(crdGroup, datadogGroup, configuredGroup), collectorNames(cb))
		require.False(t, cb.crdsChanged.Load())

		// The change was handled
		cb.RefreshDiscovery()
		require.Equal(t, 2, fetchMock.calls)
	})

	t.Run("Failed refresh after a CRD change is retried with a backoff", func(t *testing.T) {
		fetchMock := newDiscoveryFetchMock(beforeCRDs, discoveryFetchMockResult{err: errors.New("discovery total failure")}, afterCRDs)
		cb := newBundle(t, fetchMock)

		cb.crdsChanged.Store(true)
		cb.RefreshDiscovery()
		require.Equal(t, 2, fetchMock.calls)
		require.True(t, cb.crdsChanged.Load())
		require.Contains(t, warningMessages(cb), "Kubernetes API discovery retry failed, skipping 1 check run(s) before retrying: discovery total failure")

		// Skipped run
		cb.RefreshDiscovery()
		require.Equal(t, 2, fetchMock.calls)

		cb.RefreshDiscovery()
		require.Equal(t, 3, fetchMock.calls)
		require.Equal(t, collectorNamesOf(crdGroup, datadogGroup, configuredGroup), collectorNames(cb))
		require.False(t, cb.crdsChanged.Load())
	})

	t.Run("CRD change during incomplete discovery is handled by the same retry", func(t *testing.T) {
		partial := discoveryFetchMockResult{
			err:       newGroupDiscoveryError(schema.GroupVersion{Group: "group1", Version: "v1"}),
			groups:    beforeCRDs.groups,
			resources: beforeCRDs.resources,
		}
		fetchMock := newDiscoveryFetchMock(partial, afterCRDs)
		cb := newBundle(t, fetchMock)

		cb.crdsChanged.Store(true)

		// Skipped run of the discovery backoff
		cb.RefreshDiscovery()
		require.Equal(t, 1, fetchMock.calls)
		require.True(t, cb.crdsChanged.Load())

		cb.RefreshDiscovery()
		require.Equal(t, 2, fetchMock.calls)
		require.Equal(t, collectorNamesOf(crdGroup, datadogGroup, configuredGroup), collectorNames(cb))
		require.False(t, cb.crdsChanged.Load())

		cb.RefreshDiscovery()
		require.Equal(t, 2, fetchMock.calls)
	})
}

func TestCollectorBundleCRDChangeHandler(t *testing.T) {
	newCRD := func(group string, established bool, servedVersions ...string) *apiextensionsv1.CustomResourceDefinition {
		crd := &apiextensionsv1.CustomResourceDefinition{Spec: apiextensionsv1.CustomResourceDefinitionSpec{Group: group}}
		for _, version := range servedVersions {
			crd.Spec.Versions = append(crd.Spec.Versions, apiextensionsv1.CustomResourceDefinitionVersion{Name: version, Served: true})
		}
		crd.Spec.Versions = append(crd.Spec.Versions, apiextensionsv1.CustomResourceDefinitionVersion{Name: "v0", Served: false})
		status := apiextensionsv1.ConditionFalse
		if established {
			status = apiextensionsv1.ConditionTrue
		}
		crd.Status.Conditions = []apiextensionsv1.CustomResourceDefinitionCondition{{Type: apiextensionsv1.Established, Status: status}}
		return crd
	}

	tests := []struct {
		name     string
		add      bool
		oldObj   interface{}
		newObj   interface{}
		expected bool
	}{
		{name: "Added established CRD", add: true, newObj: newCRD("datadoghq.com", true, "v1"), expected: true},
		{name: "Added CRD not established yet", add: true, newObj: newCRD("datadoghq.com", false, "v1"), expected: false},
		{name: "Added CRD of a group not collected", add: true, newObj: newCRD("example.com", true, "v1"), expected: false},
		{name: "CRD becomes established", oldObj: newCRD("datadoghq.com", false, "v1"), newObj: newCRD("datadoghq.com", true, "v1"), expected: true},
		{name: "CRD serves a new version", oldObj: newCRD("datadoghq.com", true, "v1"), newObj: newCRD("datadoghq.com", true, "v1", "v2"), expected: true},
		{name: "CRD stops serving a version", oldObj: newCRD("datadoghq.com", true, "v1", "v2"), newObj: newCRD("datadoghq.com", true, "v1"), expected: false},
		{name: "CRD resync", oldObj: newCRD("datadoghq.com", true, "v1"), newObj: newCRD("datadoghq.com", true, "v1"), expected: false},
		{name: "Not a CRD", add: true, newObj: &v1.APIGroup{}, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := &CollectorBundle{collectedGroups: map[string]struct{}{"datadoghq.com": {}}}
			handler := cb.crdChangeHandler()
			if tt.add {
				handler.OnAdd(tt.newObj, false)
			} else {
				handler.OnUpdate(tt.oldObj, tt.newObj)
			}
			require.Equal(t, tt.expected, cb.crdsChanged.Load())
		})
	}
}

func TestCollectorBundleInitializeWatchesCRDs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := k8sfake.NewClientset()
		crdClient := crdfake.NewSimpleClientset() //nolint:staticcheck // SA1019 NewClientset has no type schema for CRDs
		apiClient := &apiserver.APIClient{Cl: client, InformerCl: client, CRDInformerClient: crdClient}
		stopCh := make(chan struct{})

		cb := CollectorBundle{
			collectors:            []collectors.K8sCollector{k8s.NewCRDCollector()},
			collectedGroups:       map[string]struct{}{"datadoghq.com": {}},
			extraSyncTimeout:      defaultExtraSyncTimeout,
			initializedCollectors: map[string]struct{}{},
			runCfg: &collectors.CollectorRunConfig{
				Config: &orchcfg.OrchestratorConfig{},
				K8sCollectorRunConfig: collectors.K8sCollectorRunConfig{
					APIClient:                   apiClient,
					OrchestratorInformerFactory: getOrchestratorInformerFactory(apiClient),
				},
			},
			startedInformers: map[cache.Store]struct{}{},
			stopCh:           stopCh,
		}

		cb.Initialize()
		synctest.Wait()
		require.False(t, cb.crdsChanged.Load())

		crd := &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: v1.ObjectMeta{Name: "datadogmetrics.datadoghq.com"},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Group:    "datadoghq.com",
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: "v1alpha1", Served: true}},
			},
			Status: apiextensionsv1.CustomResourceDefinitionStatus{
				Conditions: []apiextensionsv1.CustomResourceDefinitionCondition{{Type: apiextensionsv1.Established, Status: apiextensionsv1.ConditionTrue}},
			},
		}
		_, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Create(context.Background(), crd, v1.CreateOptions{})
		require.NoError(t, err)

		// Waits until the informer delivered the event and every bubble goroutine is idle.
		synctest.Wait()
		require.True(t, cb.crdsChanged.Load())

		// Stop the informers so the bubble goroutines exit before the test returns.
		close(stopCh)
		synctest.Wait()
	})
}

type discoveryFetchMock struct {
	calls   int
	results []discoveryFetchMockResult
}

// newDiscoveryFetchMock returns a fetch function returning results in order, the last one being repeated.
func newDiscoveryFetchMock(results ...discoveryFetchMockResult) *discoveryFetchMock {
	return &discoveryFetchMock{results: results}
}

func (f *discoveryFetchMock) fetch() ([]*v1.APIGroup, []*v1.APIResourceList, error) {
	result := f.results[min(f.calls, len(f.results)-1)]
	f.calls++
	return result.groups, result.resources, result.err
}

type discoveryFetchMockResult struct {
	err       error
	groups    []*v1.APIGroup
	resources []*v1.APIResourceList
}

type discoveryGroup struct {
	Group     *v1.APIGroup
	Resources *v1.APIResourceList
}

func collectorNames(cb *CollectorBundle) []string {
	names := make([]string, 0, len(cb.collectors))
	for _, collector := range cb.collectors {
		names = append(names, collector.Metadata().FullName())
	}
	slices.Sort(names)
	return names
}

// collectorNamesOf returns the sorted names of the collectors of the resources served by groups.
func collectorNamesOf(groups ...discoveryGroup) []string {
	var names []string
	for _, group := range groups {
		for _, resource := range group.Resources.APIResources {
			names = append(names, fmt.Sprintf("%s/%s", group.Resources.GroupVersion, resource.Name))
		}
	}
	slices.Sort(names)
	return names
}

func newGroupDiscoveryError(groupVersions ...schema.GroupVersion) error {
	groups := make(map[schema.GroupVersion]error, len(groupVersions))
	for _, gv := range groupVersions {
		groups[gv] = errors.New("service unavailable")
	}
	return &k8sdiscovery.ErrGroupDiscoveryFailed{Groups: groups}
}

func newRetryTestBundle(t *testing.T, configured []string, discoverCollectors bool, fetch discovery.ServerGroupsAndResourcesFunc, settings map[string]any) *CollectorBundle {
	cfg := mockconfig.New(t)
	for key, value := range settings {
		cfg.SetInTest(key, value)
	}

	return &CollectorBundle{
		activatedCollectors: map[string]struct{}{},
		check: &OrchestratorCheck{
			CheckBase: core.NewCheckBase(CheckName),
			instance:  &OrchestratorInstance{Collectors: configured},
		},
		collectorDiscovery: discovery.NewDiscoveryCollector(fetch),
		discoverCollectors: discoverCollectors,
		discoveryRetrier:   newRetrier(collectionInterval, maxDiscoveryRetryDelay),
		inventory:          inventory.NewCollectorInventory(cfg, nil, nil),
		runCfg: &collectors.CollectorRunConfig{
			K8sCollectorRunConfig: collectors.K8sCollectorRunConfig{
				APIClient: createMockAPIClient(),
			},
		},
	}
}

// warningMessages returns the check warnings emitted since the last call.
func warningMessages(cb *CollectorBundle) []string {
	warnings := cb.check.GetWarnings()
	messages := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		messages = append(messages, warning.Error())
	}
	return messages
}
