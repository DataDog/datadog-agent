// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configrefresh

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	secrets "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/secretsutils"
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
	fmt.Println(lastAPIKey)
	assert.Equal(v.T(), 1, 2)
}

func (v *linuxAPIKeyRefreshSuite) TestIntakeRefreshAPIKeysAdditionalEndpoints() {
	// Define the API keys before and after refresh
	initialAPIKeys := map[string]string{
		"api_key": "apikey1_initial",
		"apikey2": "apikey2_initial",
		"apikey3": "apikey3_initial",
		"apikey4": "apikey4_initial",
	}
	updatedAPIKeys := map[string]string{
		"api_key": "apikey1_updated",
		"apikey2": "apikey2_updated",
		"apikey3": "apikey3_updated",
		"apikey4": "apikey4_updated",
	}

	// Define the agent config with additional endpoints
	config := `secret_backend_command: /tmp/secret.py
secret_backend_arguments:
  - /tmp
api_key: ENC[api_key]
additional_endpoints:
  "https://app.datadoghq.com":
    - ENC[apikey2]
    - ENC[apikey3]
  "https://app.datadoghq.eu":
    - ENC[apikey4]
`

	// Create a secret client to manage secrets
	secretClient := secrets.NewClient(v.T(), v.Env().RemoteHost, "/tmp")

	// Set initial secrets in the backend
	for key, value := range initialAPIKeys {
		secretClient.SetSecret(key, value)
	}

	// Deploy the agent with the initial secrets
	v.UpdateEnv(
		awshost.Provisioner(
			awshost.WithAgentOptions(
				secrets.WithUnixSetupScript("/tmp/secret.py", false),
				agentparams.WithSkipAPIKeyInConfig(),
				agentparams.WithAgentConfig(config),
			),
		),
	)

	// Verify initial API keys in status
	status := v.Env().Agent.Client.Status()
	assert.Contains(v.T(), status.Content, "API key ending with _initial")
	assert.Contains(v.T(), status.Content, "API key ending with _initial")

	// Update secrets in the backend
	for key, value := range updatedAPIKeys {
		secretClient.SetSecret(key, value)
	}

	// Refresh secrets in the agent
	secretRefreshOutput := v.Env().Agent.Client.Secret(agentclient.WithArgs([]string{"refresh"}))
	require.Contains(v.T(), secretRefreshOutput, "api_key")

	// Verify that the new API keys appear in status
	status = v.Env().Agent.Client.Status()
	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		assert.Contains(t, status.Content, "API key ending with _updated")
	}, 1*time.Minute, 10*time.Second)

	// Verify each updated API key in FakeIntake
	for _, updatedKey := range updatedAPIKeys {
		lastAPIKey, err := v.Env().FakeIntake.Client().GetLastAPIKey()
		assert.NoError(v.T(), err)
		fmt.Println("WACKTEST1", updatedKey)
		fmt.Println("WACKTEST2", lastAPIKey)
		assert.Equal(v.T(), lastAPIKey, updatedAPIKeys["api_key"])
	}
}
