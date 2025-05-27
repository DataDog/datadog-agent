// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package assertions

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

// RemoteWindowsAgentAssertions is a type that extends the SuiteAssertions to add assertions
// executing on a RemoteHost's Agent.
type RemoteWindowsAgentAssertions struct {
	*RemoteWindowsBinaryAssertions
	agentClient agentclient.Agent
}

// HasConfigValue checks if the agent has a specific configuration value
func (r *RemoteWindowsAgentAssertions) WithConfigValueEqual(key string, expectedValue string) *RemoteWindowsAgentAssertions {
	r.context.T().Helper()
	value, err := r.agentClient.ConfigWithError(agentclient.WithArgs([]string{"get", key}))
	r.require.NoError(err)
	value = strings.TrimSpace(value)
	wrappedExpectedValue := fmt.Sprintf("%s is set to: %s", key, expectedValue)
	r.require.Equal(wrappedExpectedValue, value, "expected config value %s to be %v, but got %v", key, expectedValue, value)
	return r
}
