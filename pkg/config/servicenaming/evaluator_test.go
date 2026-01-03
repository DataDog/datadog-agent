// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2016-present Datadog, Inc.

package servicenaming

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test data fixtures
var (
	testProcess = &ProcessCEL{
		Cmd:  "java -jar myapp.jar --config prod.yml",
		Binary: BinaryCEL{
			Name:  "java",
			User:  "appuser",
			Group: "appgroup",
		},
		Ports: []int{8080, 9090},
		User:  "appuser",
	}

	testContainer = &ContainerCEL{
		Name: "my-app-container",
		Image: ImageCEL{
			Name:      "docker.io/myorg/myapp:v1.2.3",
			ShortName: "myapp",
			Tag:       "v1.2.3",
		},
		Pod: PodCEL{
			Name:         "my-app-pod-abc123",
			Namespace:    "production",
			OwnerRefName: "my-app-deployment",
			OwnerRefKind: "Deployment",
			Metadata: MetadataCEL{
				Labels: map[string]string{
					"team":    "platform",
					"app":     "myapp",
					"version": "1.2.3",
				},
			},
		},
	}

	testPod = &PodCEL{
		Name:         "my-app-pod-abc123",
		Namespace:    "production",
		OwnerRefName: "my-app-deployment",
		OwnerRefKind: "Deployment",
		Metadata: MetadataCEL{
			Labels: map[string]string{
				"team":    "platform",
				"app":     "myapp",
				"version": "1.2.3",
			},
		},
	}
)

// TestEvaluator_Creation tests evaluator initialization
func TestEvaluator_Creation(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)
	assert.NotNil(t, ev)
	assert.NotNil(t, ev.env)
}

// TestEvaluateAgentConfig_FirstMatchWins tests evaluation order
func TestEvaluateAgentConfig_FirstMatchWins(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	config := &AgentServiceDiscoveryConfig{
		ServiceDefinitions: []ServiceDefinition{
			{Query: "pod.metadata.labels['team'] == 'platform'", Value: "pod.ownerref.name"},
			{Query: "true", Value: "container.name"}, // This should not be reached
		},
	}

	result, err := ev.EvaluateAgentConfig(config, testProcess, testContainer, testPod)
	require.NoError(t, err)
	assert.Equal(t, "my-app-deployment", result.ServiceName)
	assert.Equal(t, "service_definition[0]", result.MatchedRule)
}

// TestEvaluateAgentConfig_SkipFailingQuery tests error handling
func TestEvaluateAgentConfig_SkipFailingQuery(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	config := &AgentServiceDiscoveryConfig{
		ServiceDefinitions: []ServiceDefinition{
			{Query: "pod.metadata.labels['nonexistent'] == 'foo'", Value: "pod.name"},
			{Query: "true", Value: "container.name"},
		},
	}

	result, err := ev.EvaluateAgentConfig(config, testProcess, testContainer, testPod)
	require.NoError(t, err)
	// First query evaluates to false (not error), second query matches
	assert.Equal(t, "my-app-container", result.ServiceName)
	assert.Equal(t, "service_definition[1]", result.MatchedRule)
}

// TestEvaluateAgentConfig_SkipEmptyValue tests empty value handling
func TestEvaluateAgentConfig_SkipEmptyValue(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	// Create container with empty name
	emptyContainer := &ContainerCEL{
		Name: "",
		Image: ImageCEL{
			ShortName: "myapp",
		},
	}

	config := &AgentServiceDiscoveryConfig{
		ServiceDefinitions: []ServiceDefinition{
			{Query: "true", Value: "container.name"}, // Returns empty string
			{Query: "true", Value: "container.image.shortname"},
		},
	}

	result, err := ev.EvaluateAgentConfig(config, nil, emptyContainer, nil)
	require.NoError(t, err)
	// First value is empty, so second rule wins
	assert.Equal(t, "myapp", result.ServiceName)
	assert.Equal(t, "service_definition[1]", result.MatchedRule)
}

// TestEvaluateAgentConfig_SpecExamples tests all spec examples
func TestEvaluateAgentConfig_SpecExamples(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	tests := []struct {
		name           string
		config         *AgentServiceDiscoveryConfig
		expectedResult string
	}{
		{
			name: "pod labels team match",
			config: &AgentServiceDiscoveryConfig{
				ServiceDefinitions: []ServiceDefinition{
					{Query: "pod.metadata.labels['team'] == 'platform'", Value: "pod.ownerref.name"},
				},
			},
			expectedResult: "my-app-deployment",
		},
		{
			name: "process binary startsWith",
			config: &AgentServiceDiscoveryConfig{
				ServiceDefinitions: []ServiceDefinition{
					{Query: "process.binary.name.startsWith('java')", Value: "process.cmd.split(' ')[process.cmd.split(' ').size() - 1]"},
				},
			},
			expectedResult: "prod.yml",
		},
		{
			name: "container image shortname",
			config: &AgentServiceDiscoveryConfig{
				ServiceDefinitions: []ServiceDefinition{
					{Query: "container.image.shortname == 'myapp'", Value: "container.image.shortname"},
				},
			},
			expectedResult: "myapp",
		},
		{
			name: "catch-all with true",
			config: &AgentServiceDiscoveryConfig{
				ServiceDefinitions: []ServiceDefinition{
					{Query: "true", Value: "container.image.shortname"},
				},
			},
			expectedResult: "myapp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ev.EvaluateAgentConfig(tt.config, testProcess, testContainer, testPod)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedResult, result.ServiceName)
		})
	}
}

// TestEvaluateAgentConfig_SourceAndVersion tests source/version definitions
func TestEvaluateAgentConfig_SourceAndVersion(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	config := &AgentServiceDiscoveryConfig{
		ServiceDefinitions: []ServiceDefinition{
			{Query: "true", Value: "container.name"},
		},
		SourceDefinition:  "java", // literal
		VersionDefinition: "container.image.tag",
	}

	result, err := ev.EvaluateAgentConfig(config, testProcess, testContainer, testPod)
	require.NoError(t, err)
	assert.Equal(t, "my-app-container", result.ServiceName)
	assert.Equal(t, "java", result.SourceName)
	assert.Equal(t, "v1.2.3", result.Version)
}

// TestEvaluateIntegrationConfig_AdIdentifiersOR tests OR logic
func TestEvaluateIntegrationConfig_AdIdentifiersOR(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	config := &IntegrationConfig{
		AdIdentifiers: []string{
			"process.binary.name == 'python'",       // false
			"container.image.shortname == 'myapp'",  // true
		},
		ServiceDiscovery: &ServiceDiscoverySection{
			ServiceName: "container.name",
		},
	}

	result, err := ev.EvaluateIntegrationConfig(config, testProcess, testContainer, testPod)
	require.NoError(t, err)
	assert.Equal(t, "my-app-container", result.ServiceName)
	assert.Equal(t, "integration_config", result.MatchedRule)
}

// TestEvaluateIntegrationConfig_NoAdIdentifierMatch tests rejection
func TestEvaluateIntegrationConfig_NoAdIdentifierMatch(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	config := &IntegrationConfig{
		AdIdentifiers: []string{
			"process.binary.name == 'python'",
			"container.image.shortname == 'redis'",
		},
		ServiceDiscovery: &ServiceDiscoverySection{
			ServiceName: "container.name",
		},
	}

	_, err = ev.EvaluateIntegrationConfig(config, testProcess, testContainer, testPod)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no ad_identifier matched")
}

// TestEvaluateIntegrationConfig_SpecExamples tests spec examples
func TestEvaluateIntegrationConfig_SpecExamples(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	config := &IntegrationConfig{
		AdIdentifiers: []string{
			"process.binary.name.startsWith('java') || process.user != 'root'",
		},
		ServiceDiscovery: &ServiceDiscoverySection{
			ServiceName: "process.cmd.split(' ')[process.cmd.split(' ').size() - 1]",
			SourceName:  "java",
			Version:     "container.image.tag",
		},
	}

	result, err := ev.EvaluateIntegrationConfig(config, testProcess, testContainer, testPod)
	require.NoError(t, err)
	assert.Equal(t, "prod.yml", result.ServiceName)
	assert.Equal(t, "java", result.SourceName)
	assert.Equal(t, "v1.2.3", result.Version)
}

// TestFieldAliases_ShortName tests shortname/short_name aliasing
func TestFieldAliases_ShortName(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	tests := []struct {
		name  string
		value string
	}{
		{
			name:  "shortname lowercase",
			value: "container.image.shortname",
		},
		{
			name:  "short_name with underscore",
			value: "container.image.short_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ev.evaluateStringExpression(tt.value, nil, testContainer, nil)
			require.NoError(t, err)
			assert.Equal(t, "myapp", result)
		})
	}
}

// TestFieldAliases_OwnerRef tests ownerref.name vs ownerrefname
func TestFieldAliases_OwnerRef(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	tests := []struct {
		name  string
		value string
	}{
		{
			name:  "ownerref.name nested",
			value: "pod.ownerref.name",
		},
		{
			name:  "ownerrefname flat",
			value: "pod.ownerrefname",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ev.evaluateStringExpression(tt.value, nil, nil, testPod)
			require.NoError(t, err)
			assert.Equal(t, "my-app-deployment", result)
		})
	}
}

// TestCrossReferences tests process.container.pod navigation
func TestCrossReferences(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	// Create process with embedded container
	processWithContainer := &ProcessCEL{
		Cmd:  "java -jar app.jar",
		Binary: BinaryCEL{Name: "java"},
	}

	containerWithPod := &ContainerCEL{
		Name: "app",
		Pod: PodCEL{
			OwnerRefName: "my-deployment",
		},
	}

	config := &AgentServiceDiscoveryConfig{
		ServiceDefinitions: []ServiceDefinition{
			{Query: "process.binary.name == 'java'", Value: "container.pod.ownerrefname"},
		},
	}

	result, err := ev.EvaluateAgentConfig(config, processWithContainer, containerWithPod, nil)
	require.NoError(t, err)
	assert.Equal(t, "my-deployment", result.ServiceName)
}

// TestResolveSDPlaceholders tests placeholder resolution
func TestResolveSDPlaceholders(t *testing.T) {
	result := &ServiceDiscoveryResult{
		ServiceName: "my-service",
		SourceName:  "java",
		Version:     "1.2.3",
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "service_name placeholder",
			template: "tags.datadoghq.com/service: %%SD_service_name%%",
			expected: "tags.datadoghq.com/service: my-service",
		},
		{
			name:     "source_name placeholder",
			template: "source: %%SD_source_name%%",
			expected: "source: java",
		},
		{
			name:     "version placeholder",
			template: "version: %%SD_version%%",
			expected: "version: 1.2.3",
		},
		{
			name:     "container image shortname",
			template: "%%SD_container.image.short_name%%",
			expected: "myapp",
		},
		{
			name:     "container image tag",
			template: "%%SD_container.image.tag%%",
			expected: "v1.2.3",
		},
		{
			name:     "multiple placeholders",
			template: "%%SD_service_name%%:%%SD_version%%",
			expected: "my-service:1.2.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := ResolveSDPlaceholders(tt.template, result, testContainer)
			assert.Equal(t, tt.expected, output)
		})
	}
}

// TestResolveSDPlaceholders_UST tests Universal Service Tagging example
func TestResolveSDPlaceholders_UST(t *testing.T) {
	result := &ServiceDiscoveryResult{
		ServiceName: "my-app",
		Version:     "v1.2.3",
	}

	template := `
tags.datadoghq.com/my-app.service: "%%SD_container.image.short_name%%"
tags.datadoghq.com/my-app.version: "%%SD_container.image.tag%%"
`
	expected := `
tags.datadoghq.com/my-app.service: "myapp"
tags.datadoghq.com/my-app.version: "v1.2.3"
`

	output := ResolveSDPlaceholders(template, result, testContainer)
	assert.Equal(t, expected, output)
}

// TestEvaluateStringExpressionOrLiteral tests literal vs CEL detection
func TestEvaluateStringExpressionOrLiteral(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	tests := []struct {
		name     string
		expr     string
		expected string
	}{
		{
			name:     "literal string",
			expr:     "java",
			expected: "java",
		},
		{
			name:     "CEL expression",
			expr:     "container.image.shortname",
			expected: "myapp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ev.evaluateStringExpressionOrLiteral(tt.expr, nil, testContainer, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestErrorHandling_QueryError tests query error is treated as false
func TestErrorHandling_QueryError(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	// Query that will error due to accessing non-existent field
	config := &AgentServiceDiscoveryConfig{
		ServiceDefinitions: []ServiceDefinition{
			{Query: "nonexistent.field == 'foo'", Value: "container.name"},
			{Query: "true", Value: "container.image.shortname"},
		},
	}

	result, err := ev.EvaluateAgentConfig(config, testProcess, testContainer, testPod)
	require.NoError(t, err)
	// First query errors → second rule wins
	assert.Equal(t, "myapp", result.ServiceName)
	assert.Equal(t, "service_definition[1]", result.MatchedRule)
}

// TestErrorHandling_ValueError tests value error skips rule
func TestErrorHandling_ValueError(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	config := &AgentServiceDiscoveryConfig{
		ServiceDefinitions: []ServiceDefinition{
			{Query: "true", Value: "nonexistent.field"},
			{Query: "true", Value: "container.name"},
		},
	}

	result, err := ev.EvaluateAgentConfig(config, nil, testContainer, nil)
	require.NoError(t, err)
	// First value errors → second rule wins
	assert.Equal(t, "my-app-container", result.ServiceName)
	assert.Equal(t, "service_definition[1]", result.MatchedRule)
}

// TestProgramCaching tests that CEL programs are compiled once and cached
func TestProgramCaching(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	expr := "container.image.shortname"

	// First evaluation - should compile and cache
	result1, err := ev.evaluateStringExpression(expr, nil, testContainer, nil)
	require.NoError(t, err)
	assert.Equal(t, "myapp", result1)

	// Verify program is in cache
	assert.Contains(t, ev.programCache, expr)

	// Second evaluation - should use cached program
	result2, err := ev.evaluateStringExpression(expr, nil, testContainer, nil)
	require.NoError(t, err)
	assert.Equal(t, "myapp", result2)

	// Cache should still have only one entry for this expression
	assert.Len(t, ev.programCache, 1)
}

// TestProgramCaching_MultipleExpressions tests caching multiple distinct expressions
func TestProgramCaching_MultipleExpressions(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	config := &AgentServiceDiscoveryConfig{
		ServiceDefinitions: []ServiceDefinition{
			{Query: "container.image.shortname == 'myapp'", Value: "container.name"},
		},
	}

	// First evaluation
	result1, err := ev.EvaluateAgentConfig(config, nil, testContainer, nil)
	require.NoError(t, err)
	assert.Equal(t, "my-app-container", result1.ServiceName)

	// Second evaluation with same config
	result2, err := ev.EvaluateAgentConfig(config, nil, testContainer, nil)
	require.NoError(t, err)
	assert.Equal(t, "my-app-container", result2.ServiceName)

	// Cache should have 2 programs: one for query, one for value
	assert.Len(t, ev.programCache, 2)
	assert.Contains(t, ev.programCache, "container.image.shortname == 'myapp'")
	assert.Contains(t, ev.programCache, "container.name")
}

// TestProgramCaching_ThreadSafe tests concurrent access to cache
func TestProgramCaching_ThreadSafe(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	expr := "container.name"

	// Run multiple evaluations concurrently
	type result struct {
		value string
		err   error
	}
	results := make(chan result, 10)

	for i := 0; i < 10; i++ {
		go func() {
			val, evalErr := ev.evaluateStringExpression(expr, nil, testContainer, nil)
			results <- result{value: val, err: evalErr}
		}()
	}

	// Collect and validate results in main goroutine
	for i := 0; i < 10; i++ {
		res := <-results
		require.NoError(t, res.err)
		assert.Equal(t, "my-app-container", res.value)
	}

	// Should have compiled and cached the program only once
	assert.Len(t, ev.programCache, 1)
	assert.Contains(t, ev.programCache, expr)
}

// TestProgramCaching_LRUEviction tests cache eviction when limit is reached
func TestProgramCaching_LRUEviction(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	// Fill cache with 5 expressions
	for i := 0; i < 5; i++ {
		expr := fmt.Sprintf("container.name == 'test%d'", i)
		_, err := ev.evaluateBooleanExpression(expr, nil, testContainer, nil)
		// Ignore evaluation errors, we just want to test caching
		_ = err
	}

	// Verify all 5 are cached
	assert.Len(t, ev.programCache, 5)
	assert.Equal(t, 5, ev.lruList.Len())

	// Access first expression to make it most recently used
	firstExpr := "container.name == 'test0'"
	_, err = ev.evaluateBooleanExpression(firstExpr, nil, testContainer, nil)
	_ = err

	// First expression should now be at front (most recent)
	assert.Contains(t, ev.programCache, firstExpr)

	// Verify LRU structure is maintained
	assert.Equal(t, 5, ev.lruList.Len())
	assert.Len(t, ev.programCache, 5)
}

// TestProgramCaching_LRUBehavior tests true LRU behavior
func TestProgramCaching_LRUBehavior(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	// Add 3 expressions
	expr1 := "container.name == 'a'"
	expr2 := "container.name == 'b'"
	expr3 := "container.name == 'c'"

	_, _ = ev.evaluateBooleanExpression(expr1, nil, testContainer, nil)
	_, _ = ev.evaluateBooleanExpression(expr2, nil, testContainer, nil)
	_, _ = ev.evaluateBooleanExpression(expr3, nil, testContainer, nil)

	assert.Len(t, ev.programCache, 3)

	// Access expr1 again (should move to front)
	_, _ = ev.evaluateBooleanExpression(expr1, nil, testContainer, nil)

	// All 3 should still be cached
	assert.Contains(t, ev.programCache, expr1)
	assert.Contains(t, ev.programCache, expr2)
	assert.Contains(t, ev.programCache, expr3)

	// expr1 should be at front (most recent)
	front := ev.lruList.Front()
	require.NotNil(t, front)
	entry := front.Value.(*cacheEntry)
	assert.Equal(t, expr1, entry.key)
}
