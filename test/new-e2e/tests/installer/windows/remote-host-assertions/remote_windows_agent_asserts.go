// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package assertions

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

// RemoteWindowsAgentAssertions is a type that extends the SuiteAssertions to add assertions
// executing on a RemoteHost's Agent.
type RemoteWindowsAgentAssertions struct {
	*RemoteWindowsBinaryAssertions
	agentClient agentclient.Agent
}

// WithConfigValueEqual checks if the Agent runtime config has a specific configuration value
func (r *RemoteWindowsAgentAssertions) WithConfigValueEqual(key string, expectedValue string) *RemoteWindowsAgentAssertions {
	r.context.T().Helper()
	value, err := r.agentClient.ConfigWithError(agentclient.WithArgs([]string{"get", key}))
	r.require.NoError(err)
	value = strings.TrimSpace(value)
	// Extract just the value part after "is set to: "
	valueParts := strings.Split(value, "is set to: ")
	r.require.Len(valueParts, 2, "unexpected config output format")
	actualValue := strings.TrimSpace(valueParts[1])
	r.require.Equal(expectedValue, actualValue, "expected config value %s to be %v, but got %v", key, expectedValue, actualValue)
	return r
}
