// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package configcheck

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func isValidJSON(data []byte) bool {
	var js json.RawMessage
	return json.Unmarshal(data, &js) == nil
}

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"configcheck", "-v"},
		run,
		func(cliParams *cliParams, _ core.BundleParams) {
			require.Equal(t, true, cliParams.verbose)
		})
}

func TestFilterCheckConfigsByName_CheckWithNameExists(t *testing.T) {
	checkResponse := integration.ConfigCheckResponse{
		Configs: []integration.ConfigResponse{{
				Config: integration.Config{Name: "cpu"},
			}, {
				Config: integration.Config{Name: "disk"},
			},
		},
	}

	// filter the configs list to only keep the "cpu" config
	err := filterCheckConfigsByName(&checkResponse, "cpu")
	assert.NoError(t, err)

	require.Len(t, checkResponse.Configs, 1)
	config := checkResponse.Configs[0].Config
	assert.Equal(t, "cpu", config.Name)
}

func TestFilterCheckConfigsByName_NoCheckWithName(t *testing.T) {
	checkResponse := integration.ConfigCheckResponse{
		Configs: []integration.ConfigResponse{{
				Config: integration.Config{Name: "cpu"},
			}, {
				Config: integration.Config{Name: "disk"},
			},
		},
	}

	// no filtering is done on the config check response since the "memory" config is not
	err := filterCheckConfigsByName(&checkResponse, "memory")
	assert.Error(t, err)
}

func TestConvertConfigToJSON_DefaultValues(t *testing.T) {
	// convert a config with default value for all its fields
	jsonConfig := convertCheckConfigToJSON(integration.Config{}, []string{})

	assert.Equal(t, "", jsonConfig.Name)
	assert.Equal(t, "", jsonConfig.InitConfig)
	assert.Equal(t, "", jsonConfig.MetricConfig)
	assert.Equal(t, "", jsonConfig.Logs)
	assert.Empty(t, jsonConfig.Instances)
	assert.Equal(t, "Unknown provider", jsonConfig.Provider)
	assert.Equal(t, "Unknown configuration source", jsonConfig.Source)
}

func TestConvertConfigToJSON_InitializedValues(t *testing.T) {
	// config with initialized values
	c := integration.Config{
		Name:         "check name",
		Instances:    []integration.Data{integration.Data(`{"name":"instance name"}`)},
		InitConfig:   integration.Data("init config"),
		MetricConfig: integration.Data("metrics config"),
		LogsConfig:   integration.Data("logs config"),
		Provider:     "file",
		Source:       "file:/path/to/config.yaml",
	}
	jsonConfig := convertCheckConfigToJSON(c, []string{"123"})

	assert.Equal(t, "check name", jsonConfig.Name)
	assert.Equal(t, "init config", jsonConfig.InitConfig)
	assert.Equal(t, "metrics config", jsonConfig.MetricConfig)
	assert.Equal(t, "logs config", jsonConfig.Logs)

	require.Len(t, jsonConfig.Instances, 1)
	assert.Equal(t, "123", jsonConfig.Instances[0].ID)
	assert.Equal(t, `{"name":"instance name"}`, jsonConfig.Instances[0].Config)

	assert.Equal(t, "file", jsonConfig.Provider)
	assert.Equal(t, "file:/path/to/config.yaml", jsonConfig.Source)
}

func TestPrintJSON(t *testing.T) {
	c := checkConfig{
		Name:     "check name",
		Provider: "file",
		Source:   "file:/path/to/config.yaml",
		Instances: []instance{
			{
				ID:     "0",
				Config: "",
			},
			{
				ID: "123",
				Config: `name: instance123
value: 456`,
			},
		},
		InitConfig:   "foo: bar",
		MetricConfig: "foo: bar",
		Logs:         "foo: bar",
	}

	// raw json terminated by a newline
	expected := `{"check_name":"check name","provider":"file","source":"file:/path/to/config.yaml","instances":[{"id":"0","config":""},{"id":"123","config":"name: instance123\nvalue: 456"}],"init_config":"foo: bar","metric_config":"foo: bar","logs":"foo: bar"}
`

	var b bytes.Buffer

	err := printJSON(&b, c, false)
	require.NoError(t, err)

	require.True(t, isValidJSON(b.Bytes()))
	assert.Equal(t, expected, b.String())
}

func TestPrettyPrintJSON(t *testing.T) {
	c := checkConfig{
		Name:     "check name",
		Provider: "file",
		Source:   "file:/path/to/config.yaml",
		Instances: []instance{
			{
				ID:     "0",
				Config: "",
			},
			{
				ID: "123",
				Config: `name: instance123
value: 456`,
			},
		},
		InitConfig:   "foo: bar",
		MetricConfig: "foo: bar",
		Logs:         "foo: bar",
	}

	// pretty-formatted json terminated by a newline
	expected := `{
  "check_name": "check name",
  "provider": "file",
  "source": "file:/path/to/config.yaml",
  "instances": [
    {
      "id": "0",
      "config": ""
    },
    {
      "id": "123",
      "config": "name: instance123\nvalue: 456"
    }
  ],
  "init_config": "foo: bar",
  "metric_config": "foo: bar",
  "logs": "foo: bar"
}
`

	var b bytes.Buffer

	err := printJSON(&b, c, true)
	require.NoError(t, err)

	require.True(t, isValidJSON(b.Bytes()))
	assert.Equal(t, expected, b.String())
}
