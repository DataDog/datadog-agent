// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/stretchr/testify/assert"
)

type agentSecretSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestAgentSecretSuite(t *testing.T) {
	e2e.Run(t, &agentSecretSuite{}, e2e.AgentStackDef())
}

func (v *agentSecretSuite) TestAgentSecretNotEnabledByDefault() {
	secret := v.Env().Agent.Secret()

	assert.Contains(v.T(), secret, "No secret_backend_command set")
}

func (v *agentSecretSuite) TestAgentSecretChecksExecutablePermissions() {
	v.UpdateEnv(e2e.AgentStackDef(e2e.WithAgentParams(agentparams.WithAgentConfig("secret_backend_command: /usr/bin/echo"))))

	output := v.Env().Agent.Secret()

	assert.Contains(v.T(), output, "=== Checking executable permissions ===")
	assert.Contains(v.T(), output, "Executable path: /usr/bin/echo")
	assert.Contains(v.T(), output, "Executable permissions: error: invalid executable: '/usr/bin/echo' isn't owned by this user")
}

func (v *agentSecretSuite) TestAgentSecretCorrectPermissions() {
	secretScript := `#!/usr/bin/env sh
printf '{"alias_secret": {"value": "a_super_secret_string"}}\n'`
	config := `secret_backend_command: /tmp/bin/secret.sh
host_aliases:
  - ENC[alias_secret]`

	v.UpdateEnv(e2e.AgentStackDef(e2e.WithAgentParams(agentparams.WithFile("/tmp/bin/secret.sh", secretScript, false))))
	v.Env().VM.Execute(`sudo sh -c "chown dd-agent:dd-agent /tmp/bin/secret.sh && chmod 700 /tmp/bin/secret.sh"`)
	v.UpdateEnv(e2e.AgentStackDef(e2e.WithAgentParams(agentparams.WithFile("/tmp/bin/secret.sh", secretScript, false), agentparams.WithAgentConfig(config))))

	output := v.Env().Agent.Secret()

	assert.Contains(v.T(), output, "=== Checking executable permissions ===")
	assert.Contains(v.T(), output, "Executable path: /tmp/bin/secret.sh")
	assert.Contains(v.T(), output, "Executable permissions: OK, the executable has the correct permissions")
	assert.Contains(v.T(), output, "File mode: 100700")
	assert.Contains(v.T(), output, "Owner: dd-agent")
	assert.Contains(v.T(), output, "Group: dd-agent")
	assert.Contains(v.T(), output, "Number of secrets decrypted: 1")
	assert.Contains(v.T(), output, "- 'alias_secret':\n\tused in 'datadog.yaml' configuration in entry 'host_aliases'")
	// assert we don't output the decrypted secret
	assert.NotContains(v.T(), output, "a_super_secret_string")
}
