// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helper

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/viperconfig"
)

func TestGetViperCombine(t *testing.T) {
	// One setting comes from the yaml file
	configData := `network_path:
  collector:
    workers: 8
`
	// One setting comes from an env var
	t.Setenv("TEST_NETWORK_PATH_COLLECTOR_INPUT_CHAN_SIZE", "23456")

	// Create the config's defaults
	cfg := viperconfig.NewViperConfig("test", "TEST", strings.NewReplacer(".", "_"))
	cfg.SetConfigType("yaml")
	cfg.BindEnvAndSetDefault("network_path.collector.input_chan_size", 100000)
	cfg.BindEnvAndSetDefault("network_path.collector.workers", 4)
	cfg.BindEnvAndSetDefault("network_path.collector.max_ttl", 64)

	cfg.BuildSchema()
	err := cfg.ReadConfig(strings.NewReader(configData))
	require.NoError(t, err)

	// Can access individual settings okay
	assert.Equal(t, 23456, cfg.GetInt("network_path.collector.input_chan_size"))
	assert.Equal(t, 8, cfg.GetInt("network_path.collector.workers"))
	assert.Equal(t, 64, cfg.GetInt("network_path.collector.max_ttl"))

	// Viper's .Get method has a long-standing known issue: only returns a single layer
	expect := map[string]interface{}{
		"collector": map[string]interface{}{
			"workers": 8,
		},
	}
	actual := cfg.Get("network_path")
	assert.Equal(t, expect, actual)

	// GetViperCombine correctly combines all the layers
	expect = map[string]interface{}{
		"collector": map[string]interface{}{
			"input_chan_size": 23456,
			"workers":         8,
			"max_ttl":         64,
		},
	}
	actual = GetViperCombine(cfg, "network_path")
	assert.Equal(t, expect, actual)
}

func TestGetViperCombineEmptySection(t *testing.T) {
	// One setting comes from the yaml file
	configData := `network_path:
  collector:
`
	// Create the config's defaults
	cfg := viperconfig.NewViperConfig("test", "TEST", strings.NewReplacer(".", "_"))
	cfg.SetConfigType("yaml")

	cfg.BuildSchema()
	err := cfg.ReadConfig(strings.NewReader(configData))
	require.NoError(t, err)

	// GetViperCombine correctly combines all the layers
	expect := map[string]interface{}{
		"collector": nil,
	}
	actual := GetViperCombine(cfg, "network_path")
	assert.Equal(t, expect, actual)
}

func TestGetViperCombineWithoutSection(t *testing.T) {
	// One setting comes from the yaml file
	configData := `logs_config:
`
	// One setting comes from an env var
	t.Setenv("TEST_NETWORK_PATH_COLLECTOR_INPUT_CHAN_SIZE", "23456")
	t.Setenv("TEST_NETWORK_PATH_COLLECTOR_WORKERS", "8")

	// Create the config's defaults
	cfg := viperconfig.NewViperConfig("test", "TEST", strings.NewReplacer(".", "_"))
	cfg.SetConfigType("yaml")
	cfg.BindEnvAndSetDefault("network_path.collector.input_chan_size", 100000)
	cfg.BindEnv("network_path.collector.workers") //nolint:forbidigo // used to test behavior

	cfg.BuildSchema()
	err := cfg.ReadConfig(strings.NewReader(configData))
	require.NoError(t, err)

	// Can access individual settings okay
	assert.Equal(t, 23456, cfg.GetInt("network_path.collector.input_chan_size"))
	assert.Equal(t, 8, cfg.GetInt("network_path.collector.workers"))

	// GetViperCombine correctly combines all the layers
	expect := map[string]interface{}{
		"collector": map[string]interface{}{
			"input_chan_size": 23456,
			"workers":         "8",
		},
	}
	actual := GetViperCombine(cfg, "network_path")
	assert.Equal(t, expect, actual)
}

func TestGetViperCombineWithoutDefaults(t *testing.T) {
	// One setting comes from the yaml file
	configData := `logs_config:
`
	// One setting comes from an env var
	t.Setenv("TEST_NETWORK_PATH_COLLECTOR_INPUT_CHAN_SIZE", "23456")
	t.Setenv("TEST_NETWORK_PATH_COLLECTOR_WORKERS", "8")

	// Create the config's defaults
	cfg := viperconfig.NewViperConfig("test", "TEST", strings.NewReplacer(".", "_"))
	cfg.SetConfigType("yaml")
	cfg.BindEnv("network_path.collector.input_chan_size") //nolint:forbidigo // used to test behavior
	cfg.BindEnv("network_path.collector.workers")         //nolint:forbidigo // used to test behavior

	cfg.BuildSchema()
	err := cfg.ReadConfig(strings.NewReader(configData))
	require.NoError(t, err)

	// Can access individual settings okay
	assert.Equal(t, 23456, cfg.GetInt("network_path.collector.input_chan_size"))
	assert.Equal(t, 8, cfg.GetInt("network_path.collector.workers"))

	// GetViperCombine correctly combines all the layers
	expect := map[string]interface{}{
		"collector": map[string]interface{}{
			"input_chan_size": "23456",
			"workers":         "8",
		},
	}
	actual := GetViperCombine(cfg, "network_path")
	assert.Equal(t, expect, actual)
}
