// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secret

import (
	_ "embed"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

type windowsSecretSuite struct {
	baseSecretSuite
}

func TestWindowsSecretSuite(t *testing.T) {
	e2e.Run(t, &windowsSecretSuite{}, e2e.WithProvisioner(awshost.Provisioner(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)))))
}

func (v *windowsSecretSuite) TestAgentSecretExecDoesNotExist() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)), awshost.WithAgentOptions(agentparams.WithAgentConfig("secret_backend_command: /does/not/exist"))))

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		output := v.Env().Agent.Client.Secret()
		assert.Contains(t, output, "=== Checking executable permissions ===")
		assert.Contains(t, output, "Executable path: /does/not/exist")
		assert.Contains(t, output, "Executable permissions: error: secretBackendCommand '/does/not/exist' does not exist")
		assert.Regexp(t, "Number of secrets .+: 0", output)
	}, 30*time.Second, 3*time.Second)
}

func (v *windowsSecretSuite) TestAgentSecretChecksExecutablePermissions() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)), awshost.WithAgentOptions(agentparams.WithAgentConfig("secret_backend_command: C:\\Windows\\system32\\cmd.exe"))))

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		output := v.Env().Agent.Client.Secret()
		assert.Contains(t, output, "=== Checking executable permissions ===")
		assert.Contains(t, output, "Executable path: C:\\Windows\\system32\\cmd.exe")
		assert.Regexp(t, "Executable permissions: error: invalid executable 'C:\\\\Windows\\\\system32\\\\cmd.exe': other users/groups than LOCAL_SYSTEM, .+ have rights on it", output)
		assert.Regexp(t, "Number of secrets .+: 0", output)
	}, 30*time.Second, 3*time.Second)
}

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

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		output := v.Env().Agent.Client.Secret()
		assert.Contains(t, output, "=== Checking executable permissions ===")
		assert.Contains(t, output, "Executable path: C:\\TestFolder\\secret.bat")
		assert.Contains(t, output, "Executable permissions: OK, the executable has the correct permissions")

		ddagentRegex := `Access : .+\\ddagentuser Allow  ReadAndExecute`
		assert.Regexp(t, ddagentRegex, output)
		assert.Regexp(t, "Number of secrets .+: 1", output)
		assert.Contains(t, output, "- 'alias_secret':\r\n\tused in 'datadog.yaml' configuration in entry 'host_aliases")
		// assert we don't output the resolved secret
		assert.NotContains(t, output, "a_super_secret_string")
	}, 30*time.Second, 3*time.Second)
}
