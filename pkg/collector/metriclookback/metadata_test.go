// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestWithShadowExecutionModeAddsInternalMetadataToCopy(t *testing.T) {
	source := integration.Config{
		Name:       "cpu",
		InitConfig: integration.Data("loader: python\n"),
		Instances: []integration.Data{
			integration.Data("name: first\ntags:\n  - env:test\n"),
			integration.Data("name: second\n"),
		},
	}
	originalConfig := source
	originalConfig.InitConfig = append(integration.Data(nil), source.InitConfig...)
	originalConfig.Instances = append([]integration.Data(nil), source.Instances...)
	for i := range originalConfig.Instances {
		originalConfig.Instances[i] = append(integration.Data(nil), source.Instances[i]...)
	}

	shadowInstance, err := WithShadowExecutionMode(source.Instances[0])

	require.NoError(t, err)
	assert.Equal(t, originalConfig, source)
	assert.NotEqual(t, source.Instances[0], shadowInstance)

	raw := integration.RawMap{}
	require.NoError(t, yaml.Unmarshal(shadowInstance, &raw))
	assert.Equal(t, "first", raw["name"])
	assert.Equal(t, []interface{}{"env:test"}, raw["tags"])
	assert.Equal(t, integration.RawMap{"execution_mode": "shadow"}, raw["_datadog"])
}

func TestWithShadowExecutionModePreservesExistingInternalMetadata(t *testing.T) {
	instance := integration.Data("_datadog:\n  existing: value\nname: cpu\n")

	shadowInstance, err := WithShadowExecutionMode(instance)

	require.NoError(t, err)
	raw := integration.RawMap{}
	require.NoError(t, yaml.Unmarshal(shadowInstance, &raw))
	assert.Equal(t, integration.RawMap{
		"existing":       "value",
		"execution_mode": "shadow",
	}, raw["_datadog"])
}

func TestWithShadowExecutionModeOverridesUserExecutionMode(t *testing.T) {
	instance := integration.Data("_datadog:\n  execution_mode: normal\nname: cpu\n")

	shadowInstance, err := WithShadowExecutionMode(instance)

	require.NoError(t, err)
	raw := integration.RawMap{}
	require.NoError(t, yaml.Unmarshal(shadowInstance, &raw))
	assert.Equal(t, integration.RawMap{"execution_mode": "shadow"}, raw["_datadog"])
}

func TestWithShadowExecutionModeReturnsParseErrors(t *testing.T) {
	_, err := WithShadowExecutionMode(integration.Data("invalid: ["))

	require.Error(t, err)
}
