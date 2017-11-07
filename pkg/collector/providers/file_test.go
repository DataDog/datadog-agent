// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package providers

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
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

	// valid configuration file
	config, err = GetCheckConfigFromFile("foo", "tests/testcheck.yaml")
	require.Nil(t, err)
	assert.Equal(t, config.Name, "foo")
	assert.Equal(t, []byte(config.InitConfig), []byte("- test: 21\n"))
	assert.Equal(t, len(config.Instances), 1)
	assert.Equal(t, []byte(config.Instances[0]), []byte("foo: bar\n"))
	assert.Len(t, config.ADIdentifiers, 0)

	// autodiscovery
	config, err = GetCheckConfigFromFile("foo", "tests/ad_legacy.yaml")
	require.Nil(t, err)
	assert.Equal(t, config.ADIdentifiers, []string{"foo", "bar"})
	config, err = GetCheckConfigFromFile("foo", "tests/ad.yaml")
	require.Nil(t, err)
	assert.Equal(t, config.ADIdentifiers, []string{"foo_id", "bar_id"})
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
	// total number of configurations found
	assert.Equal(t, 9, len(configs))

	// count how many configs were found for a given check
	get := func(name string) []check.Config {
		out := []check.Config{}
		for _, c := range configs {
			if c.Name == name {
				out = append(out, c)
			}
		}
		return out
	}

	// the regular config
	assert.Equal(t, 3, len(get("testcheck")))

	// default configs must be picked up
	assert.Equal(t, 1, len(get("bar")))

	// regular configs override default ones
	rc := get("foo")
	assert.Equal(t, 1, len(rc))
	assert.Contains(t, string(rc[0].InitConfig), "IsNotOnTheDefaultFile")

	// nested configs override default ones
	nc := get("baz")
	if assert.Equal(t, 1, len(nc)) {
		assert.Contains(t, string(nc[0].InitConfig), "IsNotOnTheDefaultFile")
	}

	// incorrect configs get saved in the Errors map (invalid.yaml & notaconfig.yaml)
	assert.Equal(t, 2, len(provider.Errors))

	// default config in subdir
	assert.Equal(t, 1, len(get("nested_default")))
}
