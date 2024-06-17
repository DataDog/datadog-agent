// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secret

import (
	_ "embed"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	secrets "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-shared-components/secretsutils"
)

type windowsSecretSuite struct {
	baseSecretSuite
}

func TestWindowsSecretSuite(t *testing.T) {
	e2e.Run(t, &windowsSecretSuite{}, e2e.WithProvisioner(awshost.Provisioner(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)))))
}

func (v *windowsSecretSuite) TestAgentSecretExecDoesNotExist() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)), awshost.WithAgentOptions(agentparams.WithAgentConfig("secret_backend_command: /does/not/exist"))))

	output := v.Env().Agent.Client.Secret()
	assert.Contains(v.T(), output, "=== Checking executable permissions ===")
	assert.Contains(v.T(), output, "Executable path: /does/not/exist")
	assert.Contains(v.T(), output, "Executable permissions: error: secretBackendCommand '/does/not/exist' does not exist")
	assert.Regexp(v.T(), "Number of secrets .+: 0", output)
}

func (v *windowsSecretSuite) TestAgentSecretChecksExecutablePermissions() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)), awshost.WithAgentOptions(agentparams.WithAgentConfig("secret_backend_command: C:\\Windows\\system32\\cmd.exe"))))

	output := v.Env().Agent.Client.Secret()
	assert.Contains(v.T(), output, "=== Checking executable permissions ===")
	assert.Contains(v.T(), output, "Executable path: C:\\Windows\\system32\\cmd.exe")
	assert.Regexp(v.T(), "Executable permissions: error: invalid executable 'C:\\\\Windows\\\\system32\\\\cmd.exe': other users/groups than LOCAL_SYSTEM, .+ have rights on it", output)
	assert.Regexp(v.T(), "Number of secrets .+: 0", output)
}

// TODO: use helpers here
//
//go:embed fixtures/setup_secret.ps1
var secretSetupScript []byte

func (v *windowsSecretSuite) TestAgentSecretCorrectPermissions() {
	config := `secret_backend_command: C:\TestFolder\secret.bat
host_aliases:
  - ENC[alias_secret]`

	// We embed a script that file create the secret binary (C:\secret.bat) with the correct permissions
	v.UpdateEnv(
		awshost.Provisioner(
			awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
			awshost.WithAgentOptions(
				agentparams.WithFile(`C:/TestFolder/setup_secret.ps1`, string(secretSetupScript), true),
			),
		),
	)

	v.Env().RemoteHost.MustExecute(`C:/TestFolder/setup_secret.ps1 -FilePath "C:/TestFolder/secret.bat" -FileContent '@echo {"alias_secret": {"value": "a_super_secret_string"}}'`)

	v.UpdateEnv(
		awshost.Provisioner(
			awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
			awshost.WithAgentOptions(agentparams.WithAgentConfig(config))),
	)

	output := v.Env().Agent.Client.Secret()
	assert.Contains(v.T(), output, "=== Checking executable permissions ===")
	assert.Contains(v.T(), output, "Executable path: C:\\TestFolder\\secret.bat")
	assert.Contains(v.T(), output, "Executable permissions: OK, the executable has the correct permissions")

	ddagentRegex := `Access : .+\\ddagentuser Allow  ReadAndExecute`
	assert.Regexp(v.T(), ddagentRegex, output)
	assert.Regexp(v.T(), "Number of secrets .+: 1", output)
	assert.Contains(v.T(), output, "- 'alias_secret':\r\n\tused in 'datadog.yaml' configuration in entry 'host_aliases")
	// assert we don't output the resolved secret
	assert.NotContains(v.T(), output, "a_super_secret_string")
}

func (v *windowsSecretSuite) TestAgentConfigRefresh() {
	config := `secret_backend_command: C:\TestFolder\wrapper.bat
secret_backend_arguments:
  - "C:\TestFolder"
api_key: ENC[api_key]
`

	agentParams := []func(*agentparams.Params) error{
		agentparams.WithSkipAPIKeyInConfig(),
		agentparams.WithAgentConfig(config),
	}
	agentParams = append(agentParams, secrets.WithWindowsSecretSetupScript("C:/TestFolder/wrapper.bat", false)...)

	// Create API Key secret before running the Agent
	secretClient := secrets.NewSecretClient(v.T(), v.Env().RemoteHost, `C:\TestFolder`)
	secretClient.SetSecret("api_key", "abcdefghijklmnopqrstuvwxyz123456")

	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		awshost.WithAgentOptions(agentParams...)),
	)

	status := v.Env().Agent.Client.Status()
	assert.Contains(v.T(), status.Content, "API key ending with 23456")

	secretClient.SetSecret("api_key", "123456abcdefghijklmnopqrstuvwxyz")

	secretRefreshOutput := v.Env().Agent.Client.Secret(agentclient.WithArgs([]string{"refresh"}))
	require.Contains(v.T(), secretRefreshOutput, "api_key")

	status = v.Env().Agent.Client.Status()
	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		assert.Contains(t, status.Content, "API key ending with vwxyz")
	}, 1*time.Minute, 10*time.Second)
}
