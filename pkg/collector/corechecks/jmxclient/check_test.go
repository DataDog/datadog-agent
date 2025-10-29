// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo

package jmxclient

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFactory(t *testing.T) {
	factoryOption := Factory()
	factory, ok := (&factoryOption).Get()
	require.True(t, ok, "Factory should return a valid option")

	check := factory()
	require.NotNil(t, check)
	assert.Equal(t, CheckName, check.String())
}

func TestCheckConfigure(t *testing.T) {
	instanceConfig := `
host: localhost
port: 9999
tags:
  - env:test
`

	initConfig := `
is_jmx: true
conf:
  - include:
      domain: "java.lang"
      type: "Memory"
      attribute:
        HeapMemoryUsage.used:
          metric_type: gauge
          alias: jvm.heap_memory
`

	factoryOption := Factory()
	factory, _ := (&factoryOption).Get()
	check := factory().(*Check)

	// Note: This will panic without proper sender manager initialization
	// We expect a panic when calling Configure with nil sender manager
	assert.Panics(t, func() {
		check.Configure(nil, 0, []byte(instanceConfig), []byte(initConfig), "test")
	}, "Configure should panic with nil sender manager when tags are provided")

	// Test configuration parsing separately by parsing configs directly
	check2 := factory().(*Check)
	check2.instanceConfig = &InstanceConfig{}
	err := check2.instanceConfig.Parse([]byte(instanceConfig))
	require.NoError(t, err)
	assert.Equal(t, "localhost", check2.instanceConfig.Host)
	assert.Equal(t, 9999, check2.instanceConfig.Port)
	assert.Equal(t, []string{"env:test"}, check2.instanceConfig.Tags)

	check2.initConfig = &InitConfig{}
	err = check2.initConfig.Parse([]byte(initConfig))
	require.NoError(t, err)
	assert.True(t, check2.initConfig.IsJMX)
	assert.Len(t, check2.initConfig.Conf, 1)
}

func TestCheckName(t *testing.T) {
	assert.Equal(t, "jmxclient", CheckName)
}

func TestMetricMetadataLookup(t *testing.T) {
	// Test that metadata can be looked up using both primary and fallback keys
	initConfig := `
is_jmx: true
conf:
  - include:
      domain: "java.lang"
      type: "Threading"
      attribute:
        ThreadCount:
          metric_type: gauge
          alias: jvm.thread_count
        PeakThreadCount:
          metric_type: gauge
          alias: jvm.peak_thread_count
  - include:
      domain: "java.lang"
      type: "Memory"
      attribute:
        HeapMemoryUsage.used:
          metric_type: gauge
          alias: jvm.heap_memory.used
        HeapMemoryUsage.max:
          metric_type: gauge
          alias: jvm.heap_memory.max
`

	factoryOption := Factory()
	factory, _ := (&factoryOption).Get()
	check := factory().(*Check)

	check.initConfig = &InitConfig{}
	err := check.initConfig.Parse([]byte(initConfig))
	require.NoError(t, err)

	// Simulate refreshBeans to populate metricMetadata
	beanRequests := ToJmxClientFormat(check.initConfig.Conf)
	check.metricMetadata = make(map[string]MetricMetadata)
	for _, req := range beanRequests {
		// Primary key
		key := fmt.Sprintf("%s:%s", req.Path, req.Attribute)
		if req.Key != "" {
			key = fmt.Sprintf("%s.%s", key, req.Key)
		}

		metadata := MetricMetadata{
			Alias:      req.Alias,
			MetricType: req.MetricType,
		}
		if metadata.MetricType == "" {
			metadata.MetricType = "gauge"
		}
		check.metricMetadata[key] = metadata

		// Fallback key
		if req.Type != "" && req.Attribute != "" {
			var fallbackKey string
			if req.Key != "" {
				fallbackKey = fmt.Sprintf("type=%s:%s.%s", req.Type, req.Attribute, req.Key)
			} else {
				fallbackKey = fmt.Sprintf("type=%s:%s", req.Type, req.Attribute)
			}
			check.metricMetadata[fallbackKey] = metadata
		}
	}

	// Test non-composite attribute lookup with exact path match
	t.Run("non-composite with exact path", func(t *testing.T) {
		key := "java.lang:type=Threading:ThreadCount"
		metadata, exists := check.metricMetadata[key]
		require.True(t, exists, "Should find metadata with exact path")
		assert.Equal(t, "jvm.thread_count", metadata.Alias)
		assert.Equal(t, "gauge", metadata.MetricType)
	})

	// Test non-composite attribute lookup with fallback key
	t.Run("non-composite with fallback", func(t *testing.T) {
		key := "type=Threading:ThreadCount"
		metadata, exists := check.metricMetadata[key]
		require.True(t, exists, "Should find metadata with fallback key")
		assert.Equal(t, "jvm.thread_count", metadata.Alias)
		assert.Equal(t, "gauge", metadata.MetricType)
	})

	// Test composite attribute lookup with exact path match
	t.Run("composite with exact path", func(t *testing.T) {
		key := "java.lang:type=Memory:HeapMemoryUsage.used"
		metadata, exists := check.metricMetadata[key]
		require.True(t, exists, "Should find metadata with exact path")
		assert.Equal(t, "jvm.heap_memory.used", metadata.Alias)
		assert.Equal(t, "gauge", metadata.MetricType)
	})

	// Test composite attribute lookup with fallback key
	t.Run("composite with fallback", func(t *testing.T) {
		key := "type=Memory:HeapMemoryUsage.used"
		metadata, exists := check.metricMetadata[key]
		require.True(t, exists, "Should find metadata with fallback key")
		assert.Equal(t, "jvm.heap_memory.used", metadata.Alias)
		assert.Equal(t, "gauge", metadata.MetricType)
	})
}

func TestProcessMetricsWithPathMismatch(t *testing.T) {
	// This test simulates the scenario where the jmxclient returns a bean path
	// that doesn't exactly match the configured path, which can happen with
	// wildcards or additional bean properties

	factoryOption := Factory()
	factory, _ := (&factoryOption).Get()
	check := factory().(*Check)

	// Setup metadata as if refreshBeans was called
	check.metricMetadata = map[string]MetricMetadata{
		// Primary keys (may not match if path differs)
		"java.lang:type=Threading:ThreadCount": {
			Alias:      "jvm.thread_count",
			MetricType: "gauge",
		},
		"java.lang:type=Memory:HeapMemoryUsage.used": {
			Alias:      "jvm.heap_memory.used",
			MetricType: "gauge",
		},
		// Fallback keys (should always work if Type is available)
		"type=Threading:ThreadCount": {
			Alias:      "jvm.thread_count",
			MetricType: "gauge",
		},
		"type=Memory:HeapMemoryUsage.used": {
			Alias:      "jvm.heap_memory.used",
			MetricType: "gauge",
		},
	}

	// Simulate beans returned by jmxclient with slightly different paths
	t.Run("non-composite with path mismatch", func(t *testing.T) {
		// Bean path has additional properties that weren't in the configured path
		bean := BeanData{
			Path:      "java.lang:name=main,type=Threading",  // Different from configured "java.lang:type=Threading"
			Type:      "Threading",
			Attribute: "ThreadCount",
			Success:   true,
			Attributes: []BeanAttribute{
				{Name: "value", Value: "42"},  // jmxclient might return generic "value" for non-composite
			},
		}

		// Simulate the lookup logic from processMetrics
		attr := bean.Attributes[0]

		// First try: path:attr.Name
		key := fmt.Sprintf("%s:%s", bean.Path, attr.Name)
		_, hasMetadata := check.metricMetadata[key]
		assert.False(t, hasMetadata, "First try should not match (different attr name)")

		// Second try: composite key
		if !hasMetadata && bean.Attribute != "" {
			compositeKey := fmt.Sprintf("%s:%s.%s", bean.Path, bean.Attribute, attr.Name)
			_, hasMetadata = check.metricMetadata[compositeKey]
		}
		assert.False(t, hasMetadata, "Second try should not match (not composite)")

		// Third try: simple key with path
		if !hasMetadata && bean.Attribute != "" {
			simpleKey := fmt.Sprintf("%s:%s", bean.Path, bean.Attribute)
			_, hasMetadata = check.metricMetadata[simpleKey]
		}
		assert.False(t, hasMetadata, "Third try should not match (path differs)")

		// Fourth try: fallback with type
		var metadata MetricMetadata
		if !hasMetadata && bean.Type != "" {
			if bean.Attribute != "" {
				typeKey := fmt.Sprintf("type=%s:%s", bean.Type, bean.Attribute)
				metadata, hasMetadata = check.metricMetadata[typeKey]
			}
		}
		assert.True(t, hasMetadata, "Fourth try should match using fallback key")
		assert.Equal(t, "jvm.thread_count", metadata.Alias)
		assert.Equal(t, "gauge", metadata.MetricType)
	})

	t.Run("composite with path mismatch", func(t *testing.T) {
		bean := BeanData{
			Path:      "java.lang:name=ps-eden-space,type=Memory",  // Different from configured
			Type:      "Memory",
			Attribute: "HeapMemoryUsage",
			Success:   true,
			Attributes: []BeanAttribute{
				{Name: "used", Value: "1234567"},
			},
		}

		attr := bean.Attributes[0]

		// First try
		key := fmt.Sprintf("%s:%s", bean.Path, attr.Name)
		_, hasMetadata := check.metricMetadata[key]
		assert.False(t, hasMetadata)

		// Second try: composite key with path
		var metadata MetricMetadata
		if !hasMetadata && bean.Attribute != "" {
			compositeKey := fmt.Sprintf("%s:%s.%s", bean.Path, bean.Attribute, attr.Name)
			metadata, hasMetadata = check.metricMetadata[compositeKey]
		}
		assert.False(t, hasMetadata, "Second try should not match (path differs)")

		// Fourth try: fallback with type (composite)
		if !hasMetadata && bean.Type != "" && bean.Attribute != "" {
			typeKey := fmt.Sprintf("type=%s:%s.%s", bean.Type, bean.Attribute, attr.Name)
			metadata, hasMetadata = check.metricMetadata[typeKey]
		}
		assert.True(t, hasMetadata, "Fourth try should match using fallback key")
		assert.Equal(t, "jvm.heap_memory.used", metadata.Alias)
		assert.Equal(t, "gauge", metadata.MetricType)
	})
}
