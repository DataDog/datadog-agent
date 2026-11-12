// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestIsJMXInstance(t *testing.T) {

	var cases = []struct {
		instance      integration.Data
		initConfig    integration.Data
		expectedIsJmx bool
	}{
		{integration.Data("{}"), integration.Data("{}"), false},
		{integration.Data("{}"), integration.Data("{\"is_jmx\": true}"), true},
		{integration.Data("{\"is_jmx\": true}"), integration.Data("{\"is_jmx\": true}"), true},
		{integration.Data("{\"is_jmx\": true}"), integration.Data("{}"), true},
		{integration.Data("{}"), integration.Data("{\"is_jmx\": false}"), false},
		{integration.Data("{\"is_jmx\": false}"), integration.Data("{\"is_jmx\": false}"), false},
		{integration.Data("{\"is_jmx\": false}"), integration.Data("{}"), false},
		{integration.Data("{\"loader\": jmx}"), integration.Data("{}"), true},
		{integration.Data("{\"loader\": python}"), integration.Data("{}"), false},
		{integration.Data("{}"), integration.Data("{\"loader\": jmx}"), true},
		{integration.Data("{\"loader\": python}"), integration.Data("{\"loader\": jmx}"), false},
		{integration.Data("{\"loader\": jmx}"), integration.Data("{\"loader\": python}"), true},
		{integration.Data("{\"loader\": python, \"is_jmx\": true}"), integration.Data("{}"), false},
		{integration.Data("{}"), integration.Data("{\"loader\": python, \"is_jmx\": true}"), false},
		{integration.Data("{\"loader\": jmx}"), integration.Data("{\"loader\": python, \"is_jmx\": false}"), true},
		// YAML 1.1 boolean spellings are no longer interpreted as booleans
		// since the switch from go.yaml.in/yaml/v2 to go.yaml.in/yaml/v4: a
		// bare `yes` now parses as the string "yes", so it is not a bool and
		// does not mark the config as JMX. `true` and `false` still work.
		{integration.Data("is_jmx: yes\n"), integration.Data("{}"), false},
		{integration.Data("{}"), integration.Data("is_jmx: yes\n"), false},
	}

	for _, tc := range cases {
		isJmx := IsJMXInstance("name", tc.instance, tc.initConfig)
		assert.Equal(t, tc.expectedIsJmx, isJmx)
	}
}

func TestCollectDefaultMetricsLegacyBooleans(t *testing.T) {
	// Since the switch from go.yaml.in/yaml/v2 to go.yaml.in/yaml/v4,
	// collect_default_metrics is read from a generic map where YAML 1.1
	// boolean spellings are no longer booleans: `collect_default_metrics:
	// yes` stops enabling the default JMX metrics, `true` still does.
	config := func(initConfig string) integration.Config {
		return integration.Config{
			Name:       "cassandra",
			InitConfig: integration.Data(initConfig),
			Instances:  []integration.Data{{}},
		}
	}
	assert.False(t, CollectDefaultMetrics(config("collect_default_metrics: yes\n")))
	assert.True(t, CollectDefaultMetrics(config("collect_default_metrics: true\n")))
}
