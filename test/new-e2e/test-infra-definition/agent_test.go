// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"regexp"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type agentSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestAgentSuite(t *testing.T) {
	e2e.Run(t, &agentSuite{}, e2e.AgentStackDef())
}

func (v *agentSuite) TestAgentCommandNoArg() {
	version := v.Env().Agent.Version()

	match, err := regexp.MatchString("^Agent .* - Commit: .* - Serialization version: .* - Go version: .*$", strings.TrimSuffix(version, "\n"))
	assert.NoError(v.T(), err)
	assert.True(v.T(), match)
}

func (v *agentSuite) TestAgentCommandWithArg() {
	status := v.Env().Agent.Status(client.WithArgs([]string{"-h", "-n"}))
	assert.Contains(v.T(), status.Content, "Use \"datadog-agent status [command] --help\" for more information about a command.")
}

func (v *agentSuite) TestWithAgentConfig() {
	for _, param := range []struct {
		useConfig      bool
		config         string
		expectedConfig string
	}{
		{true, "log_level: debug", "log_level: debug\n"},
		{true, "", "log_level: info\n"},
		{true, "log_level: warn", "log_level: warn\n"},
		{true, "log_level: debug", "log_level: debug\n"},
		{false, "", "log_level: info\n"},
	} {
		var agentParams []agentparams.Option
		if param.useConfig {
			agentParams = append(agentParams, agentparams.WithAgentConfig(param.config))
		}
		v.UpdateEnv(e2e.AgentStackDef(e2e.WithAgentParams(agentParams...)))
		config := v.Env().Agent.Config()
		re := regexp.MustCompile(`.*log_level:(.*)\n`)
		matches := re.FindStringSubmatch(config)
		require.NotEmpty(v.T(), matches)
		require.Equal(v.T(), param.expectedConfig, matches[0])
	}
}

func (v *agentSuite) TestWithTelemetry() {
	v.UpdateEnv(e2e.AgentStackDef(e2e.WithAgentParams(agentparams.WithTelemetry())))

	status := v.Env().Agent.Status()
	require.Contains(v.T(), status.Content, "go_expvar")

	v.UpdateEnv(e2e.AgentStackDef())
	status = v.Env().Agent.Status()
	require.NotContains(v.T(), status.Content, "go_expvar")
}

func (v *agentSuite) TestWithLogs() {
	config := v.Env().Agent.Config()
	require.Contains(v.T(), config, "logs_enabled: false")

	v.UpdateEnv(e2e.AgentStackDef(e2e.WithAgentParams(agentparams.WithLogs())))
	config = v.Env().Agent.Config()
	require.Contains(v.T(), config, "logs_enabled: true")
}
