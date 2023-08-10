// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
)

type agentSecretSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestAgentSecretSuite(t *testing.T) {
	e2e.Run(t, &agentSecretSuite{}, e2e.AgentStackDef(nil))
}

func (v *agentSecretSuite) TestAgentSecretNotEnabledByDefault() {
	v.UpdateEnv(e2e.AgentStackDef(nil, agentparams.WithAgentConfig("")))
	// v.Env().VM.Execute("sudo systemctl restart datadog-agent")

	secret, err := v.Env().Agent.Secret()
	assert.NoError(v.T(), err)

	assert.Contains(v.T(), secret, "No secret_backend_command set")
}

func (v *agentSecretSuite) TestAgentSecretChecksExecutablePermissions() {
	v.UpdateEnv(e2e.AgentStackDef(nil, agentparams.WithAgentConfig("secret_backend_command: /usr/bin/echo")))

	output, err := v.Env().Agent.Secret()
	assert.NoError(v.T(), err)

	assert.Contains(v.T(), output, "=== Checking executable permissions ===")
	assert.Contains(v.T(), output, "Executable path: /usr/bin/echo")
	assert.Contains(v.T(), output, "Executable permissions: error: invalid executable: '/usr/bin/echo' isn't owned by this user")
}
