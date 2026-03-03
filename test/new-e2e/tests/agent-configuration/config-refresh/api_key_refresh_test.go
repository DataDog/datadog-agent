// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configrefresh

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
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
			awshost.WithRunOptions(scenec2.WithAgentOptions(
				secrets.WithUnixSetupScript("/tmp/secret.py", false),
				agentparams.WithSkipAPIKeyInConfig(),
				agentparams.WithAgentConfig(config),
			),
			)),
	)

	// Status command shows that original API Key is in use
	status := v.Env().Agent.Client.Status()
	assert.Contains(v.T(), status.Content, "API key ending with 23456")

	// Change the api key in the secret backend, and refresh it in the Agent
	secretClient.SetSecret("api_key", secondAPIKey)
	secretRefreshOutput := v.Env().Agent.Client.Secret(agentclient.WithArgs([]string{"refresh"}))
	require.Contains(v.T(), secretRefreshOutput, "api_key")

	// Assert that the status command shows the new API Key
	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		status = v.Env().Agent.Client.Status()
		assert.Contains(t, status.Content, "API key ending with vwxyz")
	}, 1*time.Minute, 10*time.Second)

	// Assert that the fakeIntake has received the new API Key
	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		lastAPIKey, err := v.Env().FakeIntake.Client().GetLastAPIKey()
		assert.NoError(t, err)
		assert.Equal(t, secondAPIKey, lastAPIKey)
	}, 1*time.Minute, 10*time.Second)
}

func (v *linuxAPIKeyRefreshSuite) TestIntakeRefreshAPIKeysAdditionalEndpoints() {
	// Define the API keys before and after refresh
	oldEnding := "12345"
	initialAPIKeys := map[string]string{
		"api_key": "key1old" + oldEnding,
		"apikey2": "key2old" + oldEnding,
		"apikey3": "key3old" + oldEnding,
		"apikey4": "key4old" + oldEnding,
	}

	updatedEnding := "54321"
	updatedAPIKeys := map[string]string{
		"api_key": "key1new" + updatedEnding,
		"apikey2": "key2new" + updatedEnding,
		"apikey3": "key3new" + updatedEnding,
		"apikey4": "key4new" + updatedEnding,
	}

	// Define the agent config with additional endpoints
	config := `secret_backend_command: /tmp/secret.py
secret_backend_arguments:
  - /tmp
api_key: ENC[api_key]
additional_endpoints:
  "intake":
    - ENC[apikey2]
    - ENC[apikey3]
  "api/v2/series":
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
			awshost.WithRunOptions(scenec2.WithAgentOptions(
				secrets.WithUnixSetupScript("/tmp/secret.py", false),
				agentparams.WithSkipAPIKeyInConfig(),
				agentparams.WithAgentConfig(config),
			),
			)),
	)

	// Verify initial API keys in status
	status := v.Env().Agent.Client.Status()

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		assert.Contains(t, status.Content, "API key ending with 12345")
		assert.Contains(t, status.Content, `intake - API Keys ending with:
      - 12345`)
		assert.Contains(t, status.Content, `api/v2/series - API Key ending with:
      - 12345`)
	}, 1*time.Minute, 10*time.Second)

	// Update secrets in the backend
	for key, value := range updatedAPIKeys {
		secretClient.SetSecret(key, value)
	}

	// Refresh secrets in the agent
	secretRefreshOutput := v.Env().Agent.Client.Secret(agentclient.WithArgs([]string{"refresh"}))

	require.Contains(v.T(), secretRefreshOutput, "api_key")
	assert.Contains(v.T(), secretRefreshOutput, "Number of secrets reloaded: 4")
	assert.Contains(v.T(), secretRefreshOutput, "Secrets handle reloaded:")
	assert.Contains(v.T(), secretRefreshOutput, "- 'api_key':\n\tused in 'datadog.yaml' configuration in entry 'api_key'")
	assert.Contains(v.T(), secretRefreshOutput, "- 'apikey2':\n\tused in 'datadog.yaml' configuration in entry 'additional_endpoints/intake/0'")
	assert.Contains(v.T(), secretRefreshOutput, "- 'apikey3':\n\tused in 'datadog.yaml' configuration in entry 'additional_endpoints/intake/1'")
	assert.Contains(v.T(), secretRefreshOutput, "- 'apikey4':\n\tused in 'datadog.yaml' configuration in entry 'additional_endpoints/api/v2/series/0'")

	// Verify that the new API keys appear in status
	status = v.Env().Agent.Client.Status()
	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		// Check main API key
		assert.Contains(t, status.Content, "API key ending with 54321")

		assert.Contains(t, status.Content, `intake - API Keys ending with:
      - 54321`)
		assert.Contains(t, status.Content, `api/v2/series - API Key ending with:
      - 54321`)
	}, 1*time.Minute, 10*time.Second)
}
