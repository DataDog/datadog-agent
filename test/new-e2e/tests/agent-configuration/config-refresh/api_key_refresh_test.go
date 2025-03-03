// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configrefresh

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	secrets "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/secretsutils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type linuxAPIKeyRefreshSuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor os.Descriptor
}

func TestLinuxAPIKeyFreshSuite(t *testing.T) {
	suite := &linuxAPIKeyRefreshSuite{descriptor: os.UbuntuDefault}
	e2e.Run(t, suite, e2e.WithProvisioner(awshost.Provisioner()))
}

func (v *linuxAPIKeyRefreshSuite) TestIntakeRefreshAPIKey() {
	const firstAPIKey = "abcdefghijklmnopqrstuvwxyz123456"
	const secondAPIKey = "123456abcdefghijklmnopqrstuvwxyz"

	// Create config that has an encoded (secret) api key
	config := `secret_backend_command: /tmp/secret.py
secret_backend_arguments:
  - /tmp
api_key: ENC[api_key]
`
	secretClient := secrets.NewClient(v.T(), v.Env().RemoteHost, "/tmp")
	// Set the real api key in the secret backend
	secretClient.SetSecret("api_key", firstAPIKey)

	v.UpdateEnv(
		awshost.Provisioner(
			awshost.WithAgentOptions(
				secrets.WithUnixSetupScript("/tmp/secret.py", false),
				agentparams.WithSkipAPIKeyInConfig(),
				agentparams.WithAgentConfig(config),
			),
		),
	)

	// Status command shows that original API Key is in use
	status := v.Env().Agent.Client.Status()
	assert.Contains(v.T(), status.Content, "API key ending with 23456")

	// Change the api key in the secret backend, and refresh it in the Agent
	secretClient.SetSecret("api_key", secondAPIKey)
	secretRefreshOutput := v.Env().Agent.Client.Secret(agentclient.WithArgs([]string{"refresh"}))
	require.Contains(v.T(), secretRefreshOutput, "api_key")

	// Assert that the status command shows the new API Key
	status = v.Env().Agent.Client.Status()
	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		assert.Contains(t, status.Content, "API key ending with vwxyz")
	}, 1*time.Minute, 10*time.Second)

	// Assert that the fakeIntake has received the API Key
	lastAPIKey, err := v.Env().FakeIntake.Client().GetLastAPIKey()
	assert.NoError(v.T(), err)
	assert.Equal(v.T(), lastAPIKey, secondAPIKey)
}
