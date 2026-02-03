// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parse

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/structure"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

func TestGetProductConfigs(t *testing.T) {
	config := []workloadfilter.RuleBundle{
		{
			Products: []workloadfilter.Product{workloadfilter.ProductMetrics, workloadfilter.ProductLogs},
			Rules: map[workloadfilter.ResourceType][]string{
				workloadfilter.ResourceType("containers"): {
					"container.name.matches('web-.*')",
					"container.pod.namespace != 'testing'",
				},
			},
		},
		{
			Products: []workloadfilter.Product{workloadfilter.ProductSBOM},
			Rules: map[workloadfilter.ResourceType][]string{
				workloadfilter.ResourceType("containers"): {
					"container.image.reference.contains('alpine')",
				},
				// This should be deleted as pods are not supported for SBOM
				workloadfilter.ResourceType("pods"): {
					"pod.name.matches('agent.*')",
				},
			},
		},
		{
			Products: []workloadfilter.Product{workloadfilter.ProductGlobal},
			Rules: map[workloadfilter.ResourceType][]string{
				workloadfilter.ResourceType("kube_services"): {
					"kube_services.name == 'backend'",
				},
			},
		},
		{
			// This should be deleted as an unrecognized product
			Products: []workloadfilter.Product{workloadfilter.Product("non-existent")},
			Rules: map[workloadfilter.ResourceType][]string{
				workloadfilter.ResourceType("containers"): {
					"container.name == 'should-not-matter'",
				},
			},
		},
	}

	results, errs := GetProductConfigs(config)
	assert.Len(t, errs, 2) // should have 2 errors: 1 for SBOM pod rule, 1 for unrecognized product

	// Should have results for all 4 products (metrics, logs, sbom, global)
	assert.Len(t, results, 4)

	// Check metrics product
	metricsRules, exists := results[workloadfilter.ProductMetrics]
	require.True(t, exists)
	assert.Len(t, metricsRules[workloadfilter.ContainerType], 2)

	// Check logs product
	logsRules, exists := results[workloadfilter.ProductLogs]
	require.True(t, exists)
	assert.Len(t, logsRules[workloadfilter.ContainerType], 2)

	// Check SBOM product
	sbomRules, exists := results[workloadfilter.ProductSBOM]
	require.True(t, exists)
	assert.Len(t, sbomRules[workloadfilter.ContainerType], 1)
	assert.Len(t, sbomRules[workloadfilter.PodType], 0) // should drop invalid pod rule

	// Check global product
	globalRules, exists := results[workloadfilter.ProductGlobal]
	require.True(t, exists)
	assert.Len(t, globalRules[workloadfilter.KubeServiceType], 1)
}

func TestConsolidateRulesByProduct(t *testing.T) {
	config := []workloadfilter.RuleBundle{
		{
			Products: []workloadfilter.Product{workloadfilter.ProductMetrics},
			Rules: map[workloadfilter.ResourceType][]string{
				workloadfilter.ResourceType("containers"): {"rule1", "rule2"},
			},
		},
		{
			Products: []workloadfilter.Product{workloadfilter.ProductMetrics},
			Rules: map[workloadfilter.ResourceType][]string{
				workloadfilter.ResourceType("containers"): {"rule3"},
				workloadfilter.ResourceType("pods"):       {"pod_rule1"},
			},
		},
	}

	consolidated := consolidateRulesByProduct(config)

	require.Len(t, consolidated, 1)
	metricsRules := consolidated[workloadfilter.ProductMetrics]

	// Should have consolidated all container rules
	assert.Equal(t, []string{"rule1", "rule2", "rule3"}, metricsRules[workloadfilter.ContainerType])
	assert.Equal(t, []string{"pod_rule1"}, metricsRules[workloadfilter.PodType])
}

func TestYAMLUnmarshal(t *testing.T) {
	yamlConfig := `
cel_workload_exclude:
- products: ["metrics"]
  rules:
    kube_services: ["true"]
    pods: ["false"]
- products:
    - logs
    - sbom
  rules:
    containers:
      - "container.name != 67"
`
	configComponent := configcomp.NewMockFromYAML(t, yamlConfig)
	var filterConfig []workloadfilter.RuleBundle
	err := structure.UnmarshalKey(configComponent, "cel_workload_exclude", &filterConfig)

	require.NoError(t, err)
	assert.Len(t, filterConfig, 2)

	assert.Contains(t, filterConfig, workloadfilter.RuleBundle{
		Products: []workloadfilter.Product{workloadfilter.ProductMetrics},
		Rules: map[workloadfilter.ResourceType][]string{
			workloadfilter.ResourceType("kube_services"): {"true"},
			workloadfilter.ResourceType("pods"):          {"false"},
		},
	})

	assert.Contains(t, filterConfig, workloadfilter.RuleBundle{
		Products: []workloadfilter.Product{workloadfilter.ProductLogs, workloadfilter.ProductSBOM},
		Rules: map[workloadfilter.ResourceType][]string{
			workloadfilter.ResourceType("containers"): {"container.name != 67"},
		},
	})
}
