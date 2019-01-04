// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package legacy

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/py"
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
	var pos = 0
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
	require.True(t, pos > 0)
	require.Equal(t, "1234", agentConfigGo["trace.api.api_key"])
	require.Equal(t, "http://ip.url", agentConfigGo["trace.api.endpoint"])
}
