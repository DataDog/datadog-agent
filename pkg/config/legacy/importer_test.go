// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package legacy

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/py"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/go-ini/ini"
	python "github.com/sbinet/go-python"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAgentConfig(t *testing.T) {
	py.Initialize("tests")
	python.PyGILState_Ensure()

	// load configuration with the old Python code
	configModule := python.PyImport_ImportModule("config")
	if configModule == nil {
		_, err, _ := python.PyErr_Fetch()
		fmt.Println(python.PyString_AsString(err.Str()))
	}
	require.NotNil(t, configModule)
	agentConfigPy := configModule.CallMethod("main")
	require.NotNil(t, agentConfigPy)

	// load configuration from Go
	agentConfigGo, err := GetAgentConfig("./tests/datadog.conf")
	require.Nil(t, err)

	// ensure we've all the items
	key := new(python.PyObject)
	value := new(python.PyObject)
	pos := 0
	for python.PyDict_Next(agentConfigPy, &pos, &key, &value) {
		keyStr := python.PyString_AS_STRING(key.Str())

		valueStr := python.PyString_AS_STRING(value.Str())

		goValue, found := agentConfigGo[keyStr]
		// histogram_aggregates value was converted from string to list
		// of strings in Agent6.
		if keyStr == "histogram_aggregates" {
			goValue = "['max', 'median', 'avg', 'count']"
		}
		// histogram_percentiles were converted from string to float
		// by the config module in agent5. In agent6 this is now the
		// responsibility of the histogram class.
		// The value is overwritten anyway: we're just testing the
		// default value.
		if keyStr == "histogram_percentiles" {
			goValue = "[0.95]"
		}

		if valueStr != goValue {
			t.Log(keyStr)
		}
		assert.True(t, found)
		assert.Equal(t, valueStr, goValue)
	}
	assert.True(t, pos > 0)
}

func TestLoad(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		oldcfg := config.Datadog
		defer func() { config.Datadog = oldcfg }()
		assert := assert.New(t)
		err := Load("./tests/datadog.conf", nil)

		assert.NoError(err)
		assert.False(config.Datadog.GetBool("apm_config.enabled"))
		assert.False(config.Datadog.IsSet("apm_config.connection_limit"))
		assert.Equal("my-env", config.Datadog.GetString("apm_config.env"))
		assert.Equal(8126, config.Datadog.GetInt("apm_config.receiver_port"))
		assert.Equal(1.2, config.Datadog.GetFloat64("apm_config.extra_sample_rate"))
		assert.Equal(10., config.Datadog.GetFloat64("apm_config.max_traces_per_second"))
		assert.Equal([]string{
			"GET|POST /healthcheck",
			"GET /V1",
		}, config.Datadog.GetStringSlice("apm_config.ignore_resources"))
	})

	t.Run("post", func(t *testing.T) {
		oldcfg := config.Datadog
		defer func() { config.Datadog = oldcfg }()
		assert := assert.New(t)
		err := Load("./tests/datadog.conf", func(f *ini.File, cfg *config.LegacyConfigConverter) error {
			if section := f.Section("trace.receiver"); section.HasKey("connection_limit") {
				v, err := section.Key("connection_limit").Int()
				if err != nil {
					return err
				}
				cfg.Set("apm_config.connection_limit", v)
			}
			return nil
		})

		assert.NoError(err)
		assert.Equal(2000, config.Datadog.GetInt("apm_config.connection_limit"))
	})
}
