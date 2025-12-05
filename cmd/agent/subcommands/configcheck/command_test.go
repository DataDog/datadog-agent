// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

func isJSON(data []byte) bool {
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

func TestConvertConfigToJSON(t *testing.T) {
	// case 1: default config
	c1 := integration.Config{}
	jsonConfig1 := convertCheckConfigToJSON(c1, []string{})

	assert.Equal(t, "", jsonConfig1.Name)
	assert.Equal(t, "", jsonConfig1.InitConfig)
	assert.Equal(t, "", jsonConfig1.MetricConfig)
	assert.Equal(t, "", jsonConfig1.Logs)
	assert.Empty(t, jsonConfig1.Instances)
	assert.Equal(t, "Unknown provider", jsonConfig1.Provider)
	assert.Equal(t, "Unknown configuration source", jsonConfig1.Source)

	// case 2: realistic check config
	c2 := integration.Config{
		Name:         "check name",
		Instances:    []integration.Data{integration.Data(`{"name":"instance name"}`)},
		InitConfig:   integration.Data("init config"),
		MetricConfig: integration.Data("metrics config"),
		LogsConfig:   integration.Data("logs config"),
		Provider:     "file",
		Source:       "file:/path/to/config.yaml",
	}
	jsonConfig2 := convertCheckConfigToJSON(c2, []string{"123"})

	assert.Equal(t, "check name", jsonConfig2.Name)
	assert.Equal(t, "init config", jsonConfig2.InitConfig)
	assert.Equal(t, "metrics config", jsonConfig2.MetricConfig)
	assert.Equal(t, "logs config", jsonConfig2.Logs)
	assert.Equal(t, 1, len(jsonConfig2.Instances))
	assert.Equal(t, "123", jsonConfig2.Instances[0].ID)
	assert.Equal(t, `{"name":"instance name"}`, jsonConfig2.Instances[0].Config)
	assert.Equal(t, "file", jsonConfig2.Provider)
	assert.Equal(t, "file:/path/to/config.yaml", jsonConfig2.Source)
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
		InitConfig:   "abc: def",
		MetricConfig: "abc: def",
		Logs:         "abc: def",
	}

	expected := `{"check_name":"check name","provider":"file","source":"file:/path/to/config.yaml","instances":[{"id":"0","config":""},{"id":"123","config":"name: instance123\nvalue: 456"}],"init_config":"abc: def","metric_config":"abc: def","logs":"abc: def"}`

	var b bytes.Buffer

	err := printJSON(&b, c, false)
	assert.NoError(t, err)

	assert.True(t, isJSON(b.Bytes()))
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
		InitConfig:   "abc: def",
		MetricConfig: "abc: def",
		Logs:         "abc: def",
	}

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
  "init_config": "abc: def",
  "metric_config": "abc: def",
  "logs": "abc: def"
}`

	var b bytes.Buffer

	err := printJSON(&b, c, true)
	assert.NoError(t, err)

	assert.True(t, isJSON(b.Bytes()))
	assert.Equal(t, expected, b.String())
}
