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
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestShadowPolicyOptionsFromConfig(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("metric_lookback.enabled", true)
	cfg.SetInTest("metric_lookback.enabled_checks", []string{"cpu", "disk"})

	assert.Equal(t, ShadowPolicyOptions{
		ShadowChecksEnabled: true,
		ChecksToShadow:      []string{"cpu", "disk"},
	}, ShadowPolicyOptionsFromConfig(cfg))
}

func TestSelectShadowCandidatesSelectsEnabledChecks(t *testing.T) {
	configs := []integration.Config{
		{
			Name:       "cpu",
			InitConfig: integration.Data("loader: python\n"),
			Instances: []integration.Data{
				integration.Data("name: first\n"),
				integration.Data("name: second\nmetric_lookback:\n  enabled: false\n"),
			},
		},
		{
			Name:       "ntp",
			InitConfig: integration.Data(""),
			Instances:  []integration.Data{integration.Data("name: skipped\n")},
		},
	}

	candidates, err := SelectShadowCandidates(configs, ShadowPolicyOptions{
		ShadowChecksEnabled: true,
		ChecksToShadow:      []string{"cpu"},
	})

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	candidate := candidates[0]
	assert.Equal(t, "cpu", candidate.SourceConfig.Name)
	assert.Equal(t, 0, candidate.InstanceIndex)
	assert.Equal(t, configs[0].Digest(), candidate.SourceConfigDigest)
	assert.Equal(t, checkid.BuildID("cpu", configs[0].FastDigest(), configs[0].Instances[0], configs[0].InitConfig), candidate.SourceCheckID)
	assert.Equal(t, checkid.ID(string(candidate.SourceCheckID)+":shadow"), candidate.ShadowCheckID)

	raw := integration.RawMap{}
	require.NoError(t, yaml.Unmarshal(candidate.Instance, &raw))
	assert.Equal(t, "first", raw["name"])
	assert.Equal(t, integration.RawMap{"execution_mode": "shadow"}, raw["_datadog"])
}

func TestSelectShadowCandidatesAllowsPerInstanceEnablement(t *testing.T) {
	configs := []integration.Config{
		{
			Name:       "cpu",
			InitConfig: integration.Data(""),
			Instances: []integration.Data{
				integration.Data("name: disabled\n"),
				integration.Data("name: enabled\nmetric_lookback:\n  enabled: true\n"),
			},
		},
	}

	candidates, err := SelectShadowCandidates(configs, ShadowPolicyOptions{})

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, 1, candidates[0].InstanceIndex)
	assert.Equal(t, checkid.BuildID("cpu", configs[0].FastDigest(), configs[0].Instances[1], configs[0].InitConfig), candidates[0].SourceCheckID)
}

func TestSelectShadowCandidatesSkipsNonCheckAndMetricsFilteredConfigs(t *testing.T) {
	configs := []integration.Config{
		{
			Name:            "logs-only",
			LogsConfig:      integration.Data("type: file\n"),
			MetricsExcluded: false,
		},
		{
			Name:            "cpu",
			Instances:       []integration.Data{integration.Data("name: filtered\n")},
			MetricsExcluded: true,
		},
	}

	candidates, err := SelectShadowCandidates(configs, ShadowPolicyOptions{ShadowChecksEnabled: true})

	require.NoError(t, err)
	assert.Empty(t, candidates)
}

func TestSelectShadowCandidatesDoesNotMutateSourceConfig(t *testing.T) {
	source := integration.Config{
		Name:         "cpu",
		InitConfig:   integration.Data("loader: python\n"),
		MetricConfig: integration.Data("metrics:\n  - cpu.usage\n"),
		LogsConfig:   integration.Data("type: file\n"),
		Instances: []integration.Data{
			integration.Data("name: first\ntags:\n  - env:test\n"),
		},
		ADIdentifiers: []string{"cpu-ad"},
	}
	original := cloneConfig(source)

	candidates, err := SelectShadowCandidates([]integration.Config{source}, ShadowPolicyOptions{ShadowChecksEnabled: true})

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, original, source)
	assert.Equal(t, original, candidates[0].SourceConfig)
	assert.NotEqual(t, source.Instances[0], candidates[0].Instance)

	candidates[0].SourceConfig.Instances[0][0] = 'X'
	assert.Equal(t, original, source)
}

func TestSelectShadowCandidatesIncludesPythonAndDefaultLoaderConfigs(t *testing.T) {
	configs := []integration.Config{
		{
			Name:       "python_check",
			InitConfig: integration.Data("loader: python\n"),
			Instances:  []integration.Data{integration.Data("name: py\n")},
		},
		{
			Name:       "default_loader_check",
			InitConfig: integration.Data(""),
			Instances:  []integration.Data{integration.Data("name: default\n")},
		},
	}

	candidates, err := SelectShadowCandidates(configs, ShadowPolicyOptions{ShadowChecksEnabled: true})

	require.NoError(t, err)
	require.Len(t, candidates, 2)
	assert.Equal(t, "python_check", candidates[0].SourceConfig.Name)
	assert.Equal(t, "default_loader_check", candidates[1].SourceConfig.Name)
}
