// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package legacy

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAgentConfig(t *testing.T) {
	// load configuration from Go
	agentConfigGo, err := GetAgentConfig("./tests/datadog.conf")
	require.NoError(t, err)

	pyConfig := map[string]string{}
	data, err := os.ReadFile("./tests/config.json")
	require.NoError(t, err)
	err = json.Unmarshal(data, &pyConfig)
	require.NoError(t, err)

	// ensure we've all the items
	for key, value := range pyConfig {
		goValue, found := agentConfigGo[key]

		// histogram_aggregates value was converted from string to list
		// of strings in Agent6.
		if key == "histogram_aggregates" {
			goValue = "['max', 'median', 'avg', 'count']"
		}
		// histogram_percentiles were converted from string to float
		// by the config module in agent5. In agent6 this is now the
		// responsibility of the histogram class.
		// The value is overwritten anyway: we're just testing the
		// default value.
		if key == "histogram_percentiles" {
			goValue = "[0.95]"
		}

		if value != goValue {
			t.Log("invalid config for key " + key + ": " + value + " != " + goValue)
		}
		assert.True(t, found)
		assert.Equal(t, value, goValue)
	}
	require.True(t, len(pyConfig) > 0)
	require.Equal(t, "1234", agentConfigGo["trace.api.api_key"])
	require.Equal(t, "http://ip.url", agentConfigGo["trace.api.endpoint"])
}
