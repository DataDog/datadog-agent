// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package assertions

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"gopkg.in/yaml.v2"
)

// RemoteWindowsAgentAssertions is a type that extends the SuiteAssertions to add assertions
// executing on a RemoteHost's Agent.
type RemoteWindowsAgentAssertions struct {
	*RemoteWindowsBinaryAssertions
	agentClient agentclient.Agent
}

// RemoteWindowsAgentConfigAssertions provides assertions for Agent configuration
type RemoteWindowsAgentConfigAssertions struct {
	*RemoteWindowsAgentAssertions
	config map[interface{}]interface{}
}

// RuntimeConfig gets the Agent runtime config and returns a config assertions helper
//
// The `config get` subcommand only supports a small set of keys, so this method
// fetches the full config and unmarshals it into a map.
func (r *RemoteWindowsAgentAssertions) RuntimeConfig() *RemoteWindowsAgentConfigAssertions {
	r.context.T().Helper()
	output, err := r.agentClient.ConfigWithError()
	r.require.NoError(err)

	var config map[interface{}]interface{}
	err = yaml.Unmarshal([]byte(output), &config)
	r.require.NoError(err)

	return &RemoteWindowsAgentConfigAssertions{
		RemoteWindowsAgentAssertions: r,
		config:                       config,
	}
}

// getConfigValue navigates through nested config using dot notation and returns the final value
func (c *RemoteWindowsAgentConfigAssertions) getConfigValue(key string) interface{} {
	c.context.T().Helper()

	// Navigate through nested config using dot notation
	keys := strings.Split(key, ".")
	current := c.config

	for i, k := range keys {
		if i == len(keys)-1 {
			// Last key, return the value
			actualValue, exists := current[k]
			c.require.True(exists, "config key %s not found", key)
			return actualValue
		}

		// Navigate deeper
		next, exists := current[k]
		c.require.True(exists, "config key %s not found", key)
		nextMap, ok := next.(map[interface{}]interface{})
		c.require.True(ok, "config key %s is not a nested object", strings.Join(keys[:i+1], "."))
		current = nextMap
	}

	// This should never be reached due to the loop logic
	c.require.Fail("unexpected error navigating config key %s", key)
	return nil
}

// WithValueEqual checks if a config key has the expected value
func (c *RemoteWindowsAgentConfigAssertions) WithValueEqual(key string, expectedValue interface{}) *RemoteWindowsAgentConfigAssertions {
	c.context.T().Helper()

	actualValue := c.getConfigValue(key)
	c.require.Equal(expectedValue, actualValue, "expected config value %s to be %v, but got %v", key, expectedValue, actualValue)

	return c
}
