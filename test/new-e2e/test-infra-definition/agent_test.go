// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type agentSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestAgentSuite(t *testing.T) {
	e2e.Run(t, &agentSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

func (v *agentSuite) TestAgentCommandNoArg() {
	version := v.Env().Agent.Client.Version()

	match, err := regexp.MatchString("^Agent .* - Commit: .* - Serialization version: .* - Go version: .*$", strings.TrimSuffix(version, "\n"))
	assert.NoError(v.T(), err)
	assert.True(v.T(), match)
}

func (v *agentSuite) TestAgentCommandWithArg() {
	status := v.Env().Agent.Client.Status(agentclient.WithArgs([]string{"-h", "-n"}))
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
		v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentParams...)))
		config := v.Env().Agent.Client.Config()
		re := regexp.MustCompile(`.*log_level:(.*)\n`)
		matches := re.FindStringSubmatch(config)
		require.NotEmpty(v.T(), matches)
		require.Equal(v.T(), param.expectedConfig, matches[0])
	}
}

func (v *agentSuite) TestWithTelemetry() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithTelemetry())))

	require.EventuallyWithT(v.T(), func(collect *assert.CollectT) {
		status := v.Env().Agent.Client.Status()
		if !assert.Contains(v.T(), status.Content, "go_expvar") {
			v.T().Log("not yet")
		}
	}, 5*time.Minute, 10*time.Second)

	require.EventuallyWithT(v.T(), func(collect *assert.CollectT) {
		v.UpdateEnv(awshost.ProvisionerNoFakeIntake())
		status := v.Env().Agent.Client.Status()
		assert.NotContains(v.T(), status.Content, "go_expvar")
	}, 5*time.Minute, 10*time.Second)
}

func (v *agentSuite) TestWithLogs() {
	config := v.Env().Agent.Client.Config()
	require.Contains(v.T(), config, "logs_enabled: false")

	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithLogs())))
	config = v.Env().Agent.Client.Config()
	require.Contains(v.T(), config, "logs_enabled: true")
}
