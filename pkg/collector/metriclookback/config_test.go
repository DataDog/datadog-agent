// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

func TestDeriveShadowConfigsFromSystemWideConfig(t *testing.T) {
	source := integration.Config{
		Name:       "cpu",
		InitConfig: integration.Data("loader: core\ninit: true"),
		Instances: []integration.Data{
			integration.Data("tags:\n  - instance:one\n"),
			integration.Data("tags:\n  - instance:two\n"),
		},
		Source:   "file:cpu",
		Provider: "file",
	}

	shadowConfigs := DeriveShadowConfigs([]integration.Config{source}, Options{ShadowChecksEnabled: true})

	require.Len(t, shadowConfigs, 2)
	for i, shadowConfig := range shadowConfigs {
		expectedNormalID := checkid.BuildID(source.Name, source.FastDigest(), source.Instances[i], source.InitConfig)
		assert.Equal(t, source, shadowConfig.SourceConfig)
		assert.Equal(t, source.Digest(), shadowConfig.SourceConfigDigest)
		assert.Equal(t, i, shadowConfig.InstanceIndex)
		assert.Equal(t, source.Instances[i], shadowConfig.Instance)
		assert.Equal(t, expectedNormalID, shadowConfig.SourceCheckID)
		assert.Equal(t, checkid.ID(string(expectedNormalID)+ShadowIDSuffix), shadowConfig.ShadowCheckID)
	}
}

func TestDeriveShadowConfigsHonorsCheckNameAllowList(t *testing.T) {
	configs := []integration.Config{
		{
			Name:       "cpu",
			InitConfig: integration.Data("loader: core\n"),
			Instances:  []integration.Data{integration.Data("metric_lookback:\n  enabled: true\n")},
		},
		{
			Name:       "disk",
			InitConfig: integration.Data("loader: core\n"),
			Instances:  []integration.Data{integration.Data("{}")},
		},
	}

	shadowConfigs := DeriveShadowConfigs(configs, Options{ShadowChecksEnabled: true, ChecksToShadow: []string{"disk"}})

	require.Len(t, shadowConfigs, 1)
	assert.Equal(t, "disk", shadowConfigs[0].SourceConfig.Name)
}

func TestDeriveShadowConfigsUsesPerInstanceEnablement(t *testing.T) {
	source := integration.Config{
		Name:       "cpu",
		InitConfig: integration.Data("loader: core\n"),
		Instances: []integration.Data{
			integration.Data("metric_lookback:\n  enabled: true\n"),
			integration.Data("metric_lookback:\n  enabled: false\n"),
			integration.Data("metric_lookback: true\n"),
			integration.Data("tags:\n  - instance:unset\n"),
		},
	}

	shadowConfigs := DeriveShadowConfigs([]integration.Config{source}, Options{})

	require.Len(t, shadowConfigs, 1)
	assert.Equal(t, 0, shadowConfigs[0].InstanceIndex)
}

func TestDeriveShadowConfigsAllowsPerInstanceOptOutFromSystemWideEnablement(t *testing.T) {
	source := integration.Config{
		Name:       "cpu",
		InitConfig: integration.Data("loader: core\n"),
		Instances: []integration.Data{
			integration.Data("name: included\n"),
			integration.Data("name: excluded\nmetric_lookback:\n  enabled: false\n"),
		},
	}

	shadowConfigs := DeriveShadowConfigs([]integration.Config{source}, Options{ShadowChecksEnabled: true})

	require.Len(t, shadowConfigs, 1)
	assert.Equal(t, 0, shadowConfigs[0].InstanceIndex)
}

func TestDeriveShadowConfigsInheritsSystemWideEnablementWhenInstanceEnabledIsUnset(t *testing.T) {
	source := integration.Config{
		Name:       "cpu",
		InitConfig: integration.Data("loader: core\n"),
		Instances: []integration.Data{
			integration.Data("name: empty\nmetric_lookback: {}\n"),
			integration.Data("name: future\nmetric_lookback:\n  future_option: true\n"),
			integration.Data("name: disabled\nmetric_lookback:\n  enabled: false\n"),
		},
	}

	shadowConfigs := DeriveShadowConfigs([]integration.Config{source}, Options{ShadowChecksEnabled: true})

	require.Len(t, shadowConfigs, 2)
	assert.Equal(t, 0, shadowConfigs[0].InstanceIndex)
	assert.Equal(t, 1, shadowConfigs[1].InstanceIndex)
}

func TestDeriveShadowConfigsSkipsUnsupportedConfigs(t *testing.T) {
	configs := []integration.Config{
		{
			Name:         "cluster",
			ClusterCheck: true,
			InitConfig:   integration.Data("loader: core\n"),
			Instances:    []integration.Data{integration.Data("{}")},
		},
		{
			Name:            "metrics_excluded",
			MetricsExcluded: true,
			InitConfig:      integration.Data("loader: core\n"),
			Instances:       []integration.Data{integration.Data("{}")},
		},
		{
			Name:       "logs_only",
			InitConfig: integration.Data("loader: core\n"),
			LogsConfig: integration.Data("[]"),
		},
		{
			Name:      "python_explicit",
			Instances: []integration.Data{integration.Data("loader: python\nmetric_lookback:\n  enabled: true\n")},
		},
		{
			Name:      "jmx_explicit",
			Instances: []integration.Data{integration.Data("loader: jmx\nmetric_lookback:\n  enabled: true\n")},
		},
		{
			Name:      "jmx_flag",
			Instances: []integration.Data{integration.Data("is_jmx: true\nmetric_lookback:\n  enabled: true\n")},
		},
		{
			Name:      "core_explicit",
			Instances: []integration.Data{integration.Data("loader: " + goCheckLoaderName + "\nmetric_lookback:\n  enabled: true\n")},
		},
	}

	shadowConfigs := DeriveShadowConfigs(configs, Options{ShadowChecksEnabled: true})

	require.Len(t, shadowConfigs, 1)
	assert.Equal(t, "core_explicit", shadowConfigs[0].SourceConfig.Name)
}

func TestDeriveShadowConfigsSkipsEmptyLoaderConfigs(t *testing.T) {
	source := integration.Config{
		Name: "cpu",
		Instances: []integration.Data{
			integration.Data("metric_lookback:\n  enabled: true\n"),
			integration.Data("loader: core\nmetric_lookback:\n  enabled: true\n"),
		},
	}

	shadowConfigs := DeriveShadowConfigs([]integration.Config{source}, Options{ShadowChecksEnabled: true})

	require.Len(t, shadowConfigs, 1)
	assert.Equal(t, 1, shadowConfigs[0].InstanceIndex)
}

func TestDeriveShadowConfigsHonorsInstanceLoaderOverride(t *testing.T) {
	source := integration.Config{
		Name:       "cpu",
		InitConfig: integration.Data("loader: python\n"),
		Instances: []integration.Data{
			integration.Data("loader: core\nmetric_lookback:\n  enabled: true\n"),
			integration.Data("metric_lookback:\n  enabled: true\n"),
		},
	}

	shadowConfigs := DeriveShadowConfigs([]integration.Config{source}, Options{ShadowChecksEnabled: true})

	require.Len(t, shadowConfigs, 1)
	assert.Equal(t, 0, shadowConfigs[0].InstanceIndex)
}

func TestDeriveShadowConfigsSkipsInstanceLoaderOverrideFromCore(t *testing.T) {
	source := integration.Config{
		Name:       "cpu",
		InitConfig: integration.Data("loader: core\n"),
		Instances: []integration.Data{
			integration.Data("loader: python\nmetric_lookback:\n  enabled: true\n"),
			integration.Data("metric_lookback:\n  enabled: true\n"),
		},
	}

	shadowConfigs := DeriveShadowConfigs([]integration.Config{source}, Options{ShadowChecksEnabled: true})

	require.Len(t, shadowConfigs, 1)
	assert.Equal(t, 1, shadowConfigs[0].InstanceIndex)
}

func TestDeriveShadowConfigsDoesNotMutateSourceConfig(t *testing.T) {
	source := integration.Config{
		Name:       "cpu",
		InitConfig: integration.Data("loader: core\n"),
		Instances: []integration.Data{
			integration.Data("metric_lookback:\n  enabled: true\n"),
		},
	}
	originalDigest := source.Digest()
	originalFastDigest := source.FastDigest()
	originalInstance := append(integration.Data(nil), source.Instances[0]...)

	shadowConfigs := DeriveShadowConfigs([]integration.Config{source}, Options{ShadowChecksEnabled: true})

	require.Len(t, shadowConfigs, 1)
	assert.Equal(t, originalDigest, source.Digest())
	assert.Equal(t, originalFastDigest, source.FastDigest())
	assert.Equal(t, originalInstance, source.Instances[0])
	assert.Equal(t, source, shadowConfigs[0].SourceConfig)
}
