// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secret

import (
	_ "embed"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/assert"
)

type windowsSecretSuite struct {
	baseSecretSuite
}

func TestWindowsSecretSuite(t *testing.T) {
	e2e.Run(t, &windowsSecretSuite{}, e2e.AgentStackDef(e2e.WithVMParams(ec2params.WithOS(ec2os.WindowsOS))))
}

func (v *windowsSecretSuite) TestAgentSecretExecDoesNotExist() {
	v.UpdateEnv(e2e.AgentStackDef(e2e.WithVMParams(ec2params.WithOS(ec2os.WindowsOS)), e2e.WithAgentParams(agentparams.WithAgentConfig("secret_backend_command: /does/not/exist"))))
	output := v.Env().Agent.Secret()
	assert.Contains(v.T(), output, "=== Checking executable permissions ===")
	assert.Contains(v.T(), output, "Executable path: /does/not/exist")
	assert.Contains(v.T(), output, "Executable permissions: error: secretBackendCommand '/does/not/exist' does not exist")
	assert.Regexp(v.T(), "Number of secrets .+: 0", output)
}

func (v *windowsSecretSuite) TestAgentSecretChecksExecutablePermissions() {
	v.UpdateEnv(e2e.AgentStackDef(e2e.WithVMParams(ec2params.WithOS(ec2os.WindowsOS)), e2e.WithAgentParams(agentparams.WithAgentConfig("secret_backend_command: C:\\Windows\\system32\\cmd.exe"))))

	output := v.Env().Agent.Secret()

	assert.Contains(v.T(), output, "=== Checking executable permissions ===")
	assert.Contains(v.T(), output, "Executable path: C:\\Windows\\system32\\cmd.exe")
	assert.Regexp(v.T(), "Executable permissions: error: invalid executable 'C:\\\\Windows\\\\system32\\\\cmd.exe': other users/groups than LOCAL_SYSTEM, .+ have rights on it", output)
	assert.Regexp(v.T(), "Number of secrets .+: 0", output)
}

//go:embed fixtures/setup_secret.ps1
var secretSetupScript []byte

func (v *windowsSecretSuite) TestAgentSecretCorrectPermissions() {
	config := `secret_backend_command: C:\secret.bat
host_aliases:
  - ENC[alias_secret]`

	// We embed a script that file create the secret binary (C:\secret.bat) with the correct permissions
	v.UpdateEnv(e2e.AgentStackDef(e2e.WithVMParams(ec2params.WithOS(ec2os.WindowsOS)),
		e2e.WithAgentParams(agentparams.WithFile(`C:/secret.bat`, string(secretSetupScript), true)),
		e2e.WithAgentParams(agentparams.WithFile(`C:/Users/Administator/scripts/setup_secret.ps1`, string(secretSetupScript), true))))
	v.Env().VM.Execute(`C:/Users/Administator/scripts/setup_secret.ps1 -FilePath "C:/secret.bat" -FileContent '@echo {"alias_secret": {"value": "a_super_secret_string"}}'`)
	v.UpdateEnv(e2e.AgentStackDef(e2e.WithVMParams(ec2params.WithOS(ec2os.WindowsOS)), e2e.WithAgentParams(agentparams.WithAgentConfig(config))))

	output := v.Env().Agent.Secret()

	assert.Contains(v.T(), output, "=== Checking executable permissions ===")
	assert.Contains(v.T(), output, "Executable path: C:\\secret.bat")
	assert.Contains(v.T(), output, "Executable permissions: OK, the executable has the correct permissions")

	ddagentRegex := `Access : .+\\ddagentuser Allow  ReadAndExecute`
	assert.Regexp(v.T(), ddagentRegex, output)
	assert.Regexp(v.T(), "Number of secrets .+: 1", output)
	assert.Contains(v.T(), output, "- 'alias_secret':\r\n\tused in 'datadog.yaml' configuration in entry 'host_aliases'")
	// assert we don't output the resolved secret
	assert.NotContains(v.T(), output, "a_super_secret_string")
}
