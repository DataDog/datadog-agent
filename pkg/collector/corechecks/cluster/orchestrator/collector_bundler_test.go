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

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/inventory"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/discovery"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	orchcfg "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
)

func TestImportBuiltinCollectors(t *testing.T) {
	cfg := mockconfig.New(t)
	cfg.SetWithoutSource("orchestrator_explorer.terminated_pods.enabled", true)
	cfg.SetWithoutSource("orchestrator_explorer.custom_resources.datadog.enabled", true)

	// add resources to discovery cache to ensure that collectors are supported
	collectorDiscovery := &discovery.DiscoveryCollector{}
	collectorDiscovery.SetCache(
		discovery.DiscoveryCache{
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
			k8s.NewUnassignedPodCollector(nil, nil, nil, utils.GetMetadataAsTags(cfg)),
			k8s.NewCRDCollector(),
		},
		inventory: inventory.NewCollectorInventory(cfg, nil, nil),
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
			cfg.SetWithoutSource("orchestrator_explorer.custom_resources.datadog.enabled", testCase.enabled)

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
		name          string
		enabled       bool
		unassignedPod bool
		expected      collectors.K8sCollector
	}{
		{
			name:          "Terminated pods collector enabled",
			enabled:       true,
			unassignedPod: true,
			expected:      k8s.NewTerminatedPodCollector(nil, nil, nil, utils.GetMetadataAsTags(cfg)),
		},
		{
			name:          "Terminated pods collector disabled",
			enabled:       false,
			unassignedPod: true,
			expected:      nil,
		},
		{
			name:          "Terminated pods collector enabled without unassigned pod collector",
			enabled:       true,
			unassignedPod: false,
			expected:      nil,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg.SetWithoutSource("orchestrator_explorer.terminated_pods.enabled", testCase.enabled)

			cb := CollectorBundle{
				collectors:         []collectors.K8sCollector{},
				collectorDiscovery: collectorDiscovery,
				inventory:          inventory.NewCollectorInventory(cfg, nil, nil),
			}

			if testCase.unassignedPod {
				cb.collectors = []collectors.K8sCollector{k8s.NewUnassignedPodCollector(nil, nil, nil, utils.GetMetadataAsTags(cfg))}
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
		"datadoghq.com/v2alpha1/datadogagents",
		"datadoghq.com/v1alpha1/datadogslos",
		"datadoghq.com/v1alpha1/datadogdashboards",
		"datadoghq.com/v1alpha1/datadogagentprofiles",
		"datadoghq.com/v1alpha1/datadogmonitors",
		"datadoghq.com/v1alpha1/datadogmetrics",

		// Argo
		"argoproj.io/v1alpha1/rollouts",

		// karpenter all resources
		"karpenter.sh/v1/",
		"karpenter.k8s.aws/v1/",
		"karpenter.azure.com/v1beta1/",
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
