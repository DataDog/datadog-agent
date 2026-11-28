// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package mokka

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewParamsWithoutGPUOperator(t *testing.T) {
	params, err := NewParams()
	require.NoError(t, err)
	assert.Nil(t, params.GPUOperator, "the standalone device plugin is the default")
}

func TestWithGPUOperatorDefaults(t *testing.T) {
	params, err := NewParams(WithGPUOperator())
	require.NoError(t, err)
	require.NotNil(t, params.GPUOperator)
	assert.Equal(t, &GPUOperatorParams{
		ChartRepository: DefaultGPUOperatorChartRepository,
		Version:         DefaultGPUOperatorVersion,
		Namespace:       DefaultGPUOperatorNamespace,
	}, params.GPUOperator)
}

func TestWithGPUOperatorOptions(t *testing.T) {
	params, err := NewParams(WithGPUOperator(
		WithGPUOperatorChartRepository("https://example.com/charts"),
		WithGPUOperatorVersion("v1.2.3"),
		WithGPUOperatorNamespace("operator"),
		WithGPUOperatorDevicePluginConfig("version: v1\n"),
	))
	require.NoError(t, err)
	assert.Equal(t, &GPUOperatorParams{
		ChartRepository:    "https://example.com/charts",
		Version:            "v1.2.3",
		Namespace:          "operator",
		DevicePluginConfig: "version: v1\n",
	}, params.GPUOperator)
}

func TestGPUOperatorValuesDevicePluginConfig(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		devicePlugin := gpuOperatorValues(&GPUOperatorParams{})["devicePlugin"].(pulumi.Map)
		assert.Equal(t, pulumi.Bool(true), devicePlugin["enabled"])
		assert.NotContains(t, devicePlugin, "config", "without a config, the device plugin keeps its defaults")
	})

	t.Run("custom config", func(t *testing.T) {
		const config = "version: v1\nsharing:\n  timeSlicing:\n    resources:\n    - name: nvidia.com/gpu\n      replicas: 4\n"
		devicePlugin := gpuOperatorValues(&GPUOperatorParams{DevicePluginConfig: config})["devicePlugin"].(pulumi.Map)
		assert.Equal(t, pulumi.Map{
			"create":  pulumi.Bool(true),
			"name":    pulumi.String(devicePluginConfigName),
			"default": pulumi.String(devicePluginConfigKey),
			"data":    pulumi.Map{devicePluginConfigKey: pulumi.String(config)},
		}, devicePlugin["config"])
	})
}

func TestGPUOperatorValuesLeaveTheDriverToMokka(t *testing.T) {
	values := gpuOperatorValues(&GPUOperatorParams{})
	for _, operand := range []string{"driver", "toolkit"} {
		assert.Equal(t, pulumi.Map{"enabled": pulumi.Bool(false)}, values[operand], "Mokka replaces the %s operand", operand)
	}
	assert.Equal(t, pulumi.Map{"enabled": pulumi.Bool(true), "default": pulumi.Bool(true)}, values["cdi"])
}
