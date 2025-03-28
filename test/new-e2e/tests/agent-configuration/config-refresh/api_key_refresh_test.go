// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configrefresh

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	fakeIntakeURL := v.Env().FakeIntake.Client().URL()
	v.T().Logf("WACKTEST88 FAKEINTAKE URL IS : %s", fakeIntakeURL)
	assert.NoError(v.T(), err)
	assert.Equal(v.T(), lastAPIKey, secondAPIKey)
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
			awshost.WithAgentOptions(
				secrets.WithUnixSetupScript("/tmp/secret.py", false),
				agentparams.WithSkipAPIKeyInConfig(),
				agentparams.WithAgentConfig(config),
			),
		),
	)

	// Verify initial API keys in status
	status := v.Env().Agent.Client.Status()
	v.T().Logf("WACKTEST1 status.Content: %s", status.Content)

	// Update secrets in the backend
	for key, value := range updatedAPIKeys {
		secretClient.SetSecret(key, value)
	}

	// Refresh secrets in the agent
	secretRefreshOutput := v.Env().Agent.Client.Secret(agentclient.WithArgs([]string{"refresh"}))
	require.Contains(v.T(), secretRefreshOutput, "api_key")

	// Verify that the new API keys appear in status
	status = v.Env().Agent.Client.Status()
	v.T().Logf("WACKTEST1 STATUS AFTER REFRESH: %s", status.Content)
	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		// Check main API key
		assert.Contains(t, status.Content, "API key ending with 54321")

		// Check additional endpoints with specific API key counts
		assert.Contains(t, status.Content, `intake - API Keys ending with:
      - 54321`)
		assert.Contains(t, status.Content, `api/v2/series - API Key ending with:
      - 54321`)
	}, 1*time.Minute, 10*time.Second)

	// Wait for the agent to send data to each endpoint and verify the API keys
	endpoints := []string{
		"/intake",
		"/api/v2/series",
	}
	fakeIntakeURL := v.Env().FakeIntake.Client().URL()
	v.T().Logf("WACKTEST99 FAKEINTAKE URL IS : %s", fakeIntakeURL)
	for _, endpoint := range endpoints {
		url := fmt.Sprintf("%s/fakeintake/payloads/?endpoint=%s", fakeIntakeURL, endpoint)
		v.T().Logf("WACKTEST2 Checking FakeIntake payloads for endpoint: %s", endpoint)
		v.T().Logf("WACKTEST555 URL IS: %s", url)
		// Wait for payloads to appear in FakeIntake
		assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
			// First check if we have any payloads
			resp, err := http.Get(url)
			require.NoError(t, err, "Failed to fetch FakeIntake payloads for %s", endpoint)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "Unexpected response code from FakeIntake for %s", endpoint)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "Failed to read FakeIntake response body for %s", endpoint)
			v.T().Logf("WACKTEST3 Raw response body for %s: %s", endpoint, string(body))

			// Parse JSON response into a slice of payloads
			var payloads []map[string]interface{}
			err = json.Unmarshal(body, &payloads)
			require.NoError(t, err, "Failed to decode FakeIntake JSON response for %s", endpoint)
			v.T().Logf("WACKTEST4.1 Number of payloads found for %s: %d", endpoint, len(payloads))

			// Verify we have payloads
			require.NotEmpty(t, payloads, "No payloads found in FakeIntake for endpoint %s", endpoint)

			// Collect all API keys seen in payloads
			foundAPIKeys := map[string]bool{}
			for i, payload := range payloads {
				v.T().Logf("WACKTEST5 Payload %d for %s: %+v", i, endpoint, payload)
				if apiKey, exists := payload["api_key"].(string); exists {
					foundAPIKeys[strings.TrimSpace(apiKey)] = true
					v.T().Logf("WACKTEST6 Found API key in payload %d: %s", i, apiKey)
				} else {
					v.T().Logf("WACKTEST7 No API key found in payload %d", i)
				}
			}

			// Verify we found API keys in the payloads
			require.NotEmpty(t, foundAPIKeys, "No API keys found in FakeIntake payloads for endpoint %s", endpoint)
			v.T().Logf("Found API keys for %s: %v", endpoint, foundAPIKeys)

			// Check for expected API keys based on endpoint.
			if endpoint == "/intake" {
				// For /intake, we expect both apikey2 and apikey3
				assert.True(t, foundAPIKeys[updatedAPIKeys["apikey2"]], "Expected apikey2 not found for /intake")
				assert.True(t, foundAPIKeys[updatedAPIKeys["apikey3"]], "Expected apikey3 not found for /intake")
			} else {
				// For /api/v2/series, we expect apikey4
				assert.True(t, foundAPIKeys[updatedAPIKeys["apikey4"]], "Expected apikey4 not found for /api/v2/series")
			}
		}, 1*time.Minute, 10*time.Second)
	}
}
