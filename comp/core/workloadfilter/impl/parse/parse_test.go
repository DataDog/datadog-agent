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

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

func TestGetProductConfigs(t *testing.T) {
	config := &workloadfilter.CELFilterConfig{
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
					"container.image.contains('alpine')",
				},
				workloadfilter.ResourceType("pods"): { // This should generate a warning for SBOM
					"pod.name.matches('agent.*')",
				},
			},
		},
		{
			Products: []workloadfilter.Product{workloadfilter.ProductGlobal},
			Rules: map[workloadfilter.ResourceType][]string{
				workloadfilter.ResourceType("processes"): {
					"process.name == 'systemd'",
				},
			},
		},
	}

	results := GetProductConfigs(config)

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

	// Check SBOM product (should have 1 container rule and 1 pod rule with warning logged)
	sbomRules, exists := results[workloadfilter.ProductSBOM]
	require.True(t, exists)
	assert.Len(t, sbomRules[workloadfilter.ContainerType], 1)
	assert.Len(t, sbomRules[workloadfilter.PodType], 1)

	// Check global product
	globalRules, exists := results[workloadfilter.ProductGlobal]
	require.True(t, exists)
	assert.Len(t, globalRules[workloadfilter.ProcessType], 1)
}

func TestConsolidateRulesByProduct(t *testing.T) {
	config := workloadfilter.CELFilterConfig{
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

func TestYAMLParser_ValidateConfig(t *testing.T) {
	// Set up config with invalid rule type for SBOM using YAML format
	yamlConfig := `
cel_workload_exclude:
- products:
    - sbom
  rules:
    containers:
      - "container.name == 'valid'"
    pods:
      - "pod.name == 'invalid_for_sbom'"
    kube_services:
      - "service.name == 'also_invalid_for_sbom'"
`
	configComponent := configcomp.NewMockFromYAML(t, yamlConfig)
	var filterConfig workloadfilter.CELFilterConfig
	err := configComponent.UnmarshalKey("cel_workload_exclude", &filterConfig)
	require.NoError(t, err)
}
