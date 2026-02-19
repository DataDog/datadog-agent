// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestNewFilterConfig_CELFallback(t *testing.T) {
	// When configuration option is defined through datadog.yaml,
	// the config component loads the value as an object.
	t.Run("unmarshal cel workload exclude object", func(t *testing.T) {

		mockConfig := configmock.New(t)

		// Set up valid CEL config that should unmarshal successfully
		celConfig := []map[string]interface{}{
			{
				"products": []string{"metrics"},
				"rules": map[string][]string{
					"container": {"container.name == 'test'"},
				},
			},
		}
		mockConfig.SetInTest("cel_workload_exclude", celConfig)

		filterConfig, err := NewFilterConfig(mockConfig)
		require.NoError(t, err)
		assert.NotNil(t, filterConfig)
		assert.NotNil(t, filterConfig.CELProductRules)
		assert.Contains(t, filterConfig.CELProductRules, workloadfilter.ProductMetrics)
	})

	// When configuration option is defined through envvar,
	// the config component loads the value as a string.
	t.Run("unmarshal cel workload exclude JSON string", func(t *testing.T) {
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
		mockConfig.SetInTest("cel_workload_exclude", jsonConfig)

		filterConfig, err := NewFilterConfig(mockConfig)
		require.NoError(t, err)
		assert.NotNil(t, filterConfig)
		assert.NotNil(t, filterConfig.CELProductRules)
		assert.Contains(t, filterConfig.CELProductRules, workloadfilter.ProductMetrics)

		containerRules := filterConfig.GetCELRulesForProduct(workloadfilter.ProductMetrics, workloadfilter.ContainerType)
		assert.Equal(t, "container.name == 'json-test'", containerRules)
		podRules := filterConfig.GetCELRulesForProduct(workloadfilter.ProductMetrics, workloadfilter.PodType)
		assert.Equal(t, "pod.namespace == 'test-ns'", podRules)
		serviceRules := filterConfig.GetCELRulesForProduct(workloadfilter.ProductMetrics, workloadfilter.KubeServiceType)
		assert.Equal(t, "service.name == 'test-service'", serviceRules)
	})

	t.Run("unmarshal cel workload exclude invalid string", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetInTest("cel_workload_exclude", "invalid string data")

		filterConfig, err := NewFilterConfig(mockConfig)
		assert.Error(t, err)
		assert.Nil(t, filterConfig)
	})

	t.Run("unmarshal cel workload exclude empty config", func(t *testing.T) {
		mockConfig := configmock.New(t)

		filterConfig, err := NewFilterConfig(mockConfig)
		require.NoError(t, err)
		assert.NotNil(t, filterConfig)
		assert.Empty(t, filterConfig.CELProductRules)
	})
}
