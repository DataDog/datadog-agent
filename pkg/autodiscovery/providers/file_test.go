// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package providers

import (
	"testing"

	adconfig "github.com/DataDog/datadog-agent/pkg/autodiscovery/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCheckConfig(t *testing.T) {
	// file does not exist
	config, err := GetCheckConfigFromFile("foo", "")
	assert.NotNil(t, err)

	// file contains invalid Yaml
	config, err = GetCheckConfigFromFile("foo", "tests/invalid.yaml")
	assert.NotNil(t, err)

	// valid yaml, invalid configuration file
	config, err = GetCheckConfigFromFile("foo", "tests/notaconfig.yaml")
	assert.NotNil(t, err)
	assert.Equal(t, len(config.Instances), 0)

	// valid metric file
	config, err = GetCheckConfigFromFile("foo", "tests/metrics.yaml")
	assert.Nil(t, err)
	assert.NotNil(t, config.MetricConfig)

	// valid logs-agent file
	config, err = GetCheckConfigFromFile("foo", "tests/logs-agent_only.yaml")
	assert.Nil(t, err)
	assert.NotNil(t, config.LogsConfig)

	// valid configuration file
	config, err = GetCheckConfigFromFile("foo", "tests/testcheck.yaml")
	require.Nil(t, err)
	assert.Equal(t, config.Name, "foo")
	assert.Equal(t, []byte(config.InitConfig), []byte("- test: 21\n"))
	assert.Equal(t, len(config.Instances), 1)
	assert.Equal(t, []byte(config.Instances[0]), []byte("foo: bar\n"))
	assert.Len(t, config.ADIdentifiers, 0)
	assert.Nil(t, config.MetricConfig)

	// autodiscovery
	config, err = GetCheckConfigFromFile("foo", "tests/ad.yaml")
	require.Nil(t, err)
	assert.Equal(t, config.ADIdentifiers, []string{"foo_id", "bar_id"})

	// autodiscovery: check if we correctly refuse to load if a 'docker_images' section is present
	config, err = GetCheckConfigFromFile("foo", "tests/ad_deprecated.yaml")
	assert.NotNil(t, err)
}

func TestNewYamlConfigProvider(t *testing.T) {
	paths := []string{"foo", "bar", "foo/bar"}
	provider := NewFileConfigProvider(paths)
	assert.Equal(t, len(provider.paths), len(paths))

	for i, p := range provider.paths {
		assert.Equal(t, p, paths[i])
	}
	assert.Zero(t, len(provider.Errors))
}

func TestCollect(t *testing.T) {
	paths := []string{"tests", "foo/bar"}
	provider := NewFileConfigProvider(paths)
	configs, err := provider.Collect()

	assert.Nil(t, err)

	// count how many configs were found for a given check
	get := func(name string) []adconfig.Config {
		out := []adconfig.Config{}
		for _, c := range configs {
			if c.Name == name {
				out = append(out, c)
			}
		}
		return out
	}

	// the regular configs
	assert.Equal(t, 3, len(get("testcheck")))
	assert.Equal(t, 1, len(get("ad")))

	// default configs must be picked up
	assert.Equal(t, 1, len(get("bar")))

	// regular configs override default ones
	rc := get("foo")
	assert.Equal(t, 1, len(rc))
	assert.Contains(t, string(rc[0].InitConfig), "IsNotOnTheDefaultFile")

	// nested configs override default ones
	nc := get("baz")
	assert.Equal(t, 1, len(nc))
	assert.Contains(t, string(nc[0].InitConfig), "IsNotOnTheDefaultFile")

	// default config in subdir
	assert.Equal(t, 1, len(get("nested_default")))

	// metric files don't override default files
	assert.Equal(t, 2, len(get("qux")))

	// metric files not collected in root directory
	assert.Equal(t, 0, len(get("metrics")))

	// total number of configurations found
	assert.Equal(t, 11, len(configs))

	// incorrect configs get saved in the Errors map (invalid.yaml & notaconfig.yaml & ad_deprecated.yaml)
	assert.Equal(t, 3, len(provider.Errors))
}
