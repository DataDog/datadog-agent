// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"context"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/stretchr/testify/assert"
)

func TestCollect(t *testing.T) {
	ctx := context.Background()
	config.Datadog().SetWithoutSource("ignore_autoconf", []string{"ignored"})
	paths := []string{"tests", "foo/bar"}
	ResetReader(paths)
	provider := NewFileConfigProvider()
	configs, err := provider.Collect(ctx)

	assert.Nil(t, err)

	// count how many configs were found for a given check
	get := func(name string) []integration.Config {
		out := []integration.Config{}
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

	// logs files don't override default files
	assert.Equal(t, 2, len(get("corge")))

	// metric files not collected in root directory
	assert.Equal(t, 0, len(get("metrics")))

	// logs files collected in root directory
	assert.Equal(t, 1, len(get("logs-agent_only")))

	// ignored autoconf file not collected
	assert.Equal(t, 0, len(get("ignored")))

	// total number of configurations found
	assert.Equal(t, 16, len(configs))

	// incorrect configs get saved in the Errors map (invalid.yaml & notaconfig.yaml & ad_deprecated.yaml)
	assert.Equal(t, 3, len(provider.Errors))
}

func TestEnvVarReplacement(t *testing.T) {
	ctx := context.Background()
	t.Setenv("test_envvar_key", "test_value")
	os.Unsetenv("test_envvar_not_set")

	paths := []string{"tests"}
	ResetReader(paths)
	provider := NewFileConfigProvider()
	configs, err := provider.Collect(ctx)

	assert.Nil(t, err)

	get := func(name string) []integration.Config {
		out := []integration.Config{}
		for _, c := range configs {
			if c.Name == name {
				out = append(out, c)
			}
		}
		return out
	}

	rc := get("envvars")
	assert.Len(t, rc, 1)
	assert.Contains(t, string(rc[0].InitConfig), "test_value")
	assert.Contains(t, string(rc[0].Instances[0]), "test_value")
	assert.Len(t, rc[0].Instances, 2)
	assert.Contains(t, string(rc[0].Instances[1]), "test_envvar_not_set")
}
