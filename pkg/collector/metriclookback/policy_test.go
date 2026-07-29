// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestShadowPolicyOptionsFromConfig(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("metric_lookback.enabled", true)
	cfg.SetInTest("metric_lookback.enabled_checks", []string{"cpu", "disk"})
	cfg.SetInTest("metric_lookback.collection_interval", 3*time.Second)

	assert.Equal(t, ShadowPolicyOptions{
		ShadowChecksEnabled: true,
		ChecksToShadow:      []string{"cpu", "disk"},
		ShadowInterval:      3 * time.Second,
	}, ShadowPolicyOptionsFromConfig(cfg))
}

func TestShadowPolicyOptionsFromConfigDefaultsInvalidShadowInterval(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("metric_lookback.collection_interval", defaultShadowCheckInterval-time.Millisecond)

	assert.Equal(t, defaultShadowCheckInterval, ShadowPolicyOptionsFromConfig(cfg).ShadowInterval)
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

	candidates := SelectShadowCandidates(configs, ShadowPolicyOptions{
		ShadowChecksEnabled: true,
		ChecksToShadow:      []string{"cpu"},
		ShadowInterval:      2 * time.Second,
	})

	require.Len(t, candidates, 1)
	candidate := candidates[0]
	assert.Equal(t, "cpu", candidate.SourceConfig.Name)
	assert.Equal(t, 0, candidate.InstanceIndex)
	assert.Equal(t, configs[0].Digest(), candidate.SourceConfigDigest)
	assert.Equal(t, 2*time.Second, candidate.ShadowInterval)

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

	candidates := SelectShadowCandidates(configs, ShadowPolicyOptions{ShadowInterval: defaultShadowCheckInterval})

	require.Len(t, candidates, 1)
	assert.Equal(t, 1, candidates[0].InstanceIndex)
	assert.Equal(t, defaultShadowCheckInterval, candidates[0].ShadowInterval)
}

func TestSelectShadowCandidatesTreatsMalformedInstanceSettingAsUnset(t *testing.T) {
	configs := []integration.Config{
		{
			Name: "cpu",
			Instances: []integration.Data{
				integration.Data("name: malformed\nmetric_lookback: true\n"),
			},
		},
	}

	candidates := SelectShadowCandidates(configs, ShadowPolicyOptions{})
	assert.Empty(t, candidates)

	candidates = SelectShadowCandidates(configs, ShadowPolicyOptions{ShadowChecksEnabled: true})
	require.Len(t, candidates, 1)
}

func TestSelectShadowCandidatesSkipsInvalidShadowInstance(t *testing.T) {
	configs := []integration.Config{
		{
			Name: "cpu",
			Instances: []integration.Data{
				integration.Data("name: valid\n"),
				integration.Data("name: invalid\n_datadog: [\n"),
			},
		},
	}

	candidates := SelectShadowCandidates(configs, ShadowPolicyOptions{ShadowChecksEnabled: true})

	require.Len(t, candidates, 1)
	assert.Equal(t, 0, candidates[0].InstanceIndex)
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

	candidates := SelectShadowCandidates(configs, ShadowPolicyOptions{ShadowChecksEnabled: true})

	assert.Empty(t, candidates)
}

func TestSelectShadowCandidatesDoesNotMutateSourceConfig(t *testing.T) {
	expectedCELSelector := func() workloadfilter.Rules {
		return workloadfilter.Rules{
			Containers: []string{`container.name == "cpu"`},
		}
	}
	source := integration.Config{
		Name:         "cpu",
		InitConfig:   integration.Data("loader: python\n"),
		MetricConfig: integration.Data("metrics:\n  - cpu.usage\n"),
		LogsConfig:   integration.Data("type: file\n"),
		Instances: []integration.Data{
			integration.Data("name: first\ntags:\n  - env:test\n"),
		},
		ADIdentifiers: []string{"cpu-ad"},
		CELSelector:   expectedCELSelector(),
	}
	original := cloneConfig(source)

	candidates := SelectShadowCandidates([]integration.Config{source}, ShadowPolicyOptions{ShadowChecksEnabled: true})

	require.Len(t, candidates, 1)
	assert.Equal(t, original, source)
	expectedShadowConfig := original
	expectedShadowConfig.LogsConfig = nil
	assert.Equal(t, expectedShadowConfig, candidates[0].SourceConfig)
	assert.NotEqual(t, source.Instances[0], candidates[0].Instance)

	candidates[0].SourceConfig.Instances[0][0] = 'X'
	assert.Equal(t, original, source)
	candidates[0].SourceConfig.CELSelector.Containers[0] = `container.name == "mutated"`
	assert.Equal(t, original, source)
	assert.Equal(t, expectedCELSelector(), source.CELSelector)
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

	candidates := SelectShadowCandidates(configs, ShadowPolicyOptions{ShadowChecksEnabled: true})

	require.Len(t, candidates, 2)
	assert.Equal(t, "python_check", candidates[0].SourceConfig.Name)
	assert.Equal(t, "default_loader_check", candidates[1].SourceConfig.Name)
}
