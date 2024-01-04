// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secret

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/assert"
)

type linuxSecretSuite struct {
	baseSecretSuite
}

func TestLinuxSecretSuite(t *testing.T) {
	e2e.Run(t, &linuxSecretSuite{}, e2e.AgentStackDef(e2e.WithVMParams(ec2params.WithOS(ec2os.UbuntuOS))))
}

func (v *linuxSecretSuite) TestAgentSecretExecDoesNotExist() {
	v.UpdateEnv(e2e.AgentStackDef(e2e.WithAgentParams(agentparams.WithAgentConfig("secret_backend_command: /does/not/exist"))))
	output := v.Env().Agent.Secret()
	assert.Contains(v.T(), output, "=== Checking executable permissions ===")
	assert.Contains(v.T(), output, "Executable path: /does/not/exist")
	assert.Contains(v.T(), output, "Executable permissions: error: invalid executable '/does/not/exist': can't stat it: no such file or directory")
	assert.Regexp(v.T(), "Number of secrets .+: 0", output)
}

func (v *linuxSecretSuite) TestAgentSecretChecksExecutablePermissions() {
	v.UpdateEnv(e2e.AgentStackDef(e2e.WithAgentParams(agentparams.WithAgentConfig("secret_backend_command: /usr/bin/echo"))))

	output := v.Env().Agent.Secret()

	assert.Contains(v.T(), output, "=== Checking executable permissions ===")
	assert.Contains(v.T(), output, "Executable path: /usr/bin/echo")
	assert.Contains(v.T(), output, "Executable permissions: error: invalid executable: '/usr/bin/echo' isn't owned by this user")
	assert.Regexp(v.T(), "Number of secrets .+: 0", output)
}

func (v *linuxSecretSuite) TestAgentSecretCorrectPermissions() {
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
	assert.Regexp(v.T(), "Number of secrets .+: 1", output)
	assert.Contains(v.T(), output, "- 'alias_secret':\n\tused in 'datadog.yaml' configuration in entry 'host_aliases'")
	// assert we don't output the resolved secret
	assert.NotContains(v.T(), output, "a_super_secret_string")
}
