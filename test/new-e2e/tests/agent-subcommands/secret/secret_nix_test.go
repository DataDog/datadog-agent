// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secret

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	secrets "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-shared-components/secretsutils"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type linuxSecretSuite struct {
	baseSecretSuite
}

func TestLinuxSecretSuite(t *testing.T) {
	e2e.Run(t, &linuxSecretSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

func (v *linuxSecretSuite) TestAgentSecretExecDoesNotExist() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithAgentConfig("secret_backend_command: /does/not/exist"))))
	output := v.Env().Agent.Client.Secret()
	assert.Contains(v.T(), output, "=== Checking executable permissions ===")
	assert.Contains(v.T(), output, "Executable path: /does/not/exist")
	assert.Contains(v.T(), output, "Executable permissions: error: invalid executable '/does/not/exist': can't stat it: no such file or directory")
	assert.Regexp(v.T(), "Number of secrets .+: 0", output)
}

func (v *linuxSecretSuite) TestAgentSecretChecksExecutablePermissions() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithAgentConfig("secret_backend_command: /usr/bin/echo"))))

	output := v.Env().Agent.Client.Secret()

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

	v.UpdateEnv(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithAgentOptions(
				agentparams.WithFileWithPermissions("/tmp/bin/secret.sh", secretScript, false, secrets.WithUnixSecretPermissions(false)),
				agentparams.WithAgentConfig(config),
			),
		),
	)

	output := v.Env().Agent.Client.Secret()

	assert.Contains(v.T(), output, "=== Checking executable permissions ===")
	assert.Contains(v.T(), output, "Executable path: /tmp/bin/secret.sh")
	assert.Contains(v.T(), output, "Executable permissions: OK, the executable has the correct permissions")
	assert.Contains(v.T(), output, "File mode: 100700")
	assert.Contains(v.T(), output, "Owner: dd-agent")
	assert.Contains(v.T(), output, "Group: dd-agent")
	assert.Regexp(v.T(), "Number of secrets .+: 1", output)
	assert.Contains(v.T(), output, "- 'alias_secret':\n\tused in 'datadog.yaml' configuration in entry 'host_aliases/0'")
	// assert we don't output the resolved secret
	assert.NotContains(v.T(), output, "a_super_secret_string")
}

func (v *linuxSecretSuite) TestAgentConfigRefresh() {
	config := `secret_backend_command: /tmp/secret.py
secret_backend_arguments:
  - /tmp
api_key: ENC[api_key]
`

	secretClient := secrets.NewSecretClient(v.T(), v.Env().RemoteHost, "/tmp")
	secretClient.SetSecret("api_key", "abcdefghijklmnopqrstuvwxyz123456")

	v.UpdateEnv(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithAgentOptions(
				secrets.WithUnixSecretSetupScript("/tmp/secret.py", false),
				agentparams.WithSkipAPIKeyInConfig(),
				agentparams.WithAgentConfig(config),
			),
		),
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
