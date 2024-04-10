// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secret

import (
	_ "embed"
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/test-infra-definitions/components/activedirectory"
	msiparams "github.com/DataDog/test-infra-definitions/components/datadog/agentparams/msi"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
)

const (
	TestDomain   = "datadogqalab.local"
	TestUser     = "TestUser"
	TestPassword = "Test1234#"
)

type windowsSecretSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
	name               string
	provisionerOptions []winawshost.ProvisionerOption
}

// updateEnvWithOption updates the environment with a new provisioner option
func (v *windowsSecretSuite) updateEnvWithOption(opt winawshost.ProvisionerOption) {
	v.UpdateEnv(winawshost.ProvisionerNoFakeIntake(append(v.provisionerOptions, opt)...))
}

func TestWindowsSecretSuite(t *testing.T) {
	suites := []windowsSecretSuite{
		{
			name:               "windows-secret-suite",
			provisionerOptions: []winawshost.ProvisionerOption{},
		},
		{
			name: "windows-domain-secret-suite",
			provisionerOptions: []winawshost.ProvisionerOption{
				winawshost.WithActiveDirectoryOptions(
					activedirectory.WithDomainController(TestDomain, TestPassword),
					activedirectory.WithDomainUser(TestUser, TestPassword),
				),
				winawshost.WithAgentOptions(agentparams.WithAdditionalInstallParameters(
					msiparams.NewInstallParams(
						msiparams.WithAgentUser(fmt.Sprintf("%s\\%s", TestDomain, TestUser)),
						msiparams.WithAgentUserPassword(TestPassword)))),
			},
		},
	}

	for _, suite := range suites {
		suite := suite
		t.Run(suite.name, func(t *testing.T) {
			e2e.Run(t, &suite, e2e.WithProvisioner(winawshost.ProvisionerNoFakeIntake(
				suite.provisionerOptions...,
			)))
		})
	}
}

func (v *windowsSecretSuite) TestAgentSecretExecDoesNotExist() {
	v.updateEnvWithOption(winawshost.WithAgentOptions(agentparams.WithAgentConfig("secret_backend_command: /does/not/exist")))

	output := v.Env().Agent.Client.Secret()
	assert.Contains(v.T(), output, "=== Checking executable permissions ===")
	assert.Contains(v.T(), output, "Executable path: /does/not/exist")
	assert.Contains(v.T(), output, "Executable permissions: error: secretBackendCommand '/does/not/exist' does not exist")
	assert.Regexp(v.T(), "Number of secrets .+: 0", output)
}

func (v *windowsSecretSuite) TestAgentSecretChecksExecutablePermissions() {
	v.updateEnvWithOption(winawshost.WithAgentOptions(agentparams.WithAgentConfig("secret_backend_command: C:\\Windows\\system32\\cmd.exe")))

	output := v.Env().Agent.Client.Secret()
	assert.Contains(v.T(), output, "=== Checking executable permissions ===")
	assert.Contains(v.T(), output, "Executable path: C:\\Windows\\system32\\cmd.exe")
	assert.Regexp(v.T(), "Executable permissions: error: invalid executable 'C:\\\\Windows\\\\system32\\\\cmd.exe': other users/groups than LOCAL_SYSTEM, .+ have rights on it", output)
	assert.Regexp(v.T(), "Number of secrets .+: 0", output)
}

//go:embed fixtures/setup_secret.ps1
var secretSetupScript []byte

func (v *windowsSecretSuite) TestAgentSecretCorrectPermissions() {
	config := `secret_backend_command: C:\TestFolder\secret.bat
host_aliases:
  - ENC[alias_secret]`

	// We embed a script that creates the secret binary (C:\secret.bat) with the correct permissions
	v.updateEnvWithOption(
		winawshost.WithAgentOptions(
			agentparams.WithFile(`C:/TestFolder/setup_secret.ps1`, string(secretSetupScript), true),
		),
	)

	v.Env().RemoteHost.MustExecute(`C:/TestFolder/setup_secret.ps1 -FilePath "C:/TestFolder/secret.bat" -FileContent '@echo {"alias_secret": {"value": "a_super_secret_string"}}'`)

	v.updateEnvWithOption(
		winawshost.WithAgentOptions(agentparams.WithAgentConfig(config)),
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
