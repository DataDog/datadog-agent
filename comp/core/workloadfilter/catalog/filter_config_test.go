// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel

package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestNewFilterConfig_CELFallback(t *testing.T) {
	t.Run("successful unmarshal - no fallback needed", func(t *testing.T) {

		mockConfig := configmock.New(t)

		// Set up valid CEL config that should unmarshal successfully
		celConfig := []workloadfilter.RuleBundle{
			{
				Products: []workloadfilter.Product{workloadfilter.ProductMetrics},
				Rules: map[workloadfilter.ResourceType][]string{
					workloadfilter.ContainerType: {"container.name == 'test'"},
				},
			},
		}
		mockConfig.SetWithoutSource("cel_workload_exclude", celConfig)

		filterConfig, err := NewFilterConfig(mockConfig)
		require.NoError(t, err)
		assert.NotNil(t, filterConfig)
		assert.NotNil(t, filterConfig.CELProductRules)
		assert.Contains(t, filterConfig.CELProductRules, workloadfilter.ProductMetrics)
	})

	t.Run("unmarshal fails but YAML string fallback succeeds", func(t *testing.T) {
		mockConfig := configmock.New(t)

		// Set up YAML string like what would come from configuration files
		yamlConfig := `- products:
  - metrics
  rules:
    kube_services:
    - service.name == 'yaml-service'
    pods:
    - pod.namespace == 'yaml-ns'
    containers:
    - container.name == 'yaml-test'`
		mockConfig.SetWithoutSource("cel_workload_exclude", yamlConfig)

		filterConfig, err := NewFilterConfig(mockConfig)
		require.NoError(t, err)
		assert.NotNil(t, filterConfig)
		assert.NotNil(t, filterConfig.CELProductRules)
		assert.Contains(t, filterConfig.CELProductRules, workloadfilter.ProductMetrics)

		containerRules := filterConfig.GetCELRulesForProduct(workloadfilter.ProductMetrics, workloadfilter.ContainerType)
		assert.Equal(t, "container.name == 'yaml-test'", containerRules)
		podRules := filterConfig.GetCELRulesForProduct(workloadfilter.ProductMetrics, workloadfilter.PodType)
		assert.Equal(t, "pod.namespace == 'yaml-ns'", podRules)
		serviceRules := filterConfig.GetCELRulesForProduct(workloadfilter.ProductMetrics, workloadfilter.ServiceType)
		assert.Equal(t, "service.name == 'yaml-service'", serviceRules)
	})

	t.Run("unmarshal fails but JSON string fallback succeeds", func(t *testing.T) {
		mockConfig := configmock.New(t)
		jsonConfig := `[
			{
				"products": ["metrics"],
				"rules": {
					"kube_services": ["service.name == 'test-service'"],
					"pods": ["pod.namespace == 'test-ns'"],
					"containers": ["container.name == 'json-test'"]
				}
			}
		]`
		mockConfig.SetWithoutSource("cel_workload_exclude", jsonConfig)

		filterConfig, err := NewFilterConfig(mockConfig)
		require.NoError(t, err)
		assert.NotNil(t, filterConfig)
		assert.NotNil(t, filterConfig.CELProductRules)
		assert.Contains(t, filterConfig.CELProductRules, workloadfilter.ProductMetrics)

		containerRules := filterConfig.GetCELRulesForProduct(workloadfilter.ProductMetrics, workloadfilter.ContainerType)
		assert.Equal(t, "container.name == 'json-test'", containerRules)
		podRules := filterConfig.GetCELRulesForProduct(workloadfilter.ProductMetrics, workloadfilter.PodType)
		assert.Equal(t, "pod.namespace == 'test-ns'", podRules)
		serviceRules := filterConfig.GetCELRulesForProduct(workloadfilter.ProductMetrics, workloadfilter.ServiceType)
		assert.Equal(t, "service.name == 'test-service'", serviceRules)
	})

	t.Run("both unmarshal and fallback fail", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetWithoutSource("cel_workload_exclude", "invalid string data")

		filterConfig, err := NewFilterConfig(mockConfig)
		assert.Error(t, err)
		assert.Nil(t, filterConfig)
	})

	t.Run("no cel_workload_exclude config", func(t *testing.T) {
		mockConfig := configmock.New(t)

		filterConfig, err := NewFilterConfig(mockConfig)
		require.NoError(t, err)
		assert.NotNil(t, filterConfig)
		assert.Empty(t, filterConfig.CELProductRules)
	})
}
