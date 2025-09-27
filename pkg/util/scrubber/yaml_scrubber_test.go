// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/yaml.v3"
)

func TestScrubDataObj(t *testing.T) {
	testCases := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name: "Scrub sensitive info from map",
			input: map[string]interface{}{
				"password": "password123",
				"username": "user1",
			},
			expected: map[string]interface{}{
				"password": "********",
				"username": "user1",
			},
		},
		{
			name: "SNMPConfig",
			input: map[string]interface{}{
				"community_string": "password123",
				"authKey":          "password",
				"authkey":          "password",
				"privKey":          "password",
				"privkey":          "password",
			},
			expected: map[string]interface{}{
				"community_string": "********",
				"authKey":          "********",
				"authkey":          "********",
				"privKey":          "********",
				"privkey":          "********",
			},
		},
		{
			name: "Scrub sensitive info from nested map",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"password": "password123",
					"email":    "user@example.com",
				},
			},
			expected: map[string]interface{}{
				"user": map[string]interface{}{
					"password": "********",
					"email":    "user@example.com",
				},
			},
		},
		{
			name:     "No sensitive info to scrub",
			input:    "Just a regular string.",
			expected: "Just a regular string.",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ScrubDataObj(&tc.input)
			assert.Equal(t, tc.expected, tc.input)
		})
	}
}

func TestConfigScrubbedValidYaml(t *testing.T) {
	wd, _ := os.Getwd()

	inputConf := filepath.Join(wd, "test", "conf.yaml")
	inputConfData, err := os.ReadFile(inputConf)
	require.NoError(t, err)

	outputConf := filepath.Join(wd, "test", "conf_scrubbed.yaml")
	outputConfData, err := os.ReadFile(outputConf)
	require.NoError(t, err)

	cleaned, err := ScrubBytes([]byte(inputConfData))
	require.NoError(t, err)

	// First test that the a scrubbed yaml is still a valid yaml
	var out interface{}
	err = yaml.Unmarshal(cleaned, &out)
	assert.NoError(t, err, "Could not load YAML configuration after being scrubbed")

	// We replace windows line break by linux so the tests pass on every OS
	trimmedOutput := strings.TrimSpace(strings.ReplaceAll(string(outputConfData), "\r\n", "\n"))
	trimmedCleaned := strings.TrimSpace(strings.ReplaceAll(string(cleaned), "\r\n", "\n"))

	assert.Equal(t, trimmedOutput, trimmedCleaned)
}

func TestConfigScrubbedYaml(t *testing.T) {
	wd, _ := os.Getwd()

	inputConf := filepath.Join(wd, "test", "conf_multiline.yaml")
	inputConfData, err := os.ReadFile(inputConf)
	require.NoError(t, err)

	outputConf := filepath.Join(wd, "test", "conf_multiline_scrubbed.yaml")
	outputConfData, err := os.ReadFile(outputConf)
	require.NoError(t, err)

	cleaned, err := ScrubYaml([]byte(inputConfData))
	require.NoError(t, err)

	// First test that the a scrubbed yaml is still a valid yaml
	var out interface{}
	err = yaml.Unmarshal(cleaned, &out)
	assert.NoError(t, err, "Could not load YAML configuration after being scrubbed")

	// We replace windows line break by linux so the tests pass on every OS
	trimmedOutput := strings.TrimSpace(strings.ReplaceAll(string(outputConfData), "\r\n", "\n"))
	trimmedCleaned := strings.TrimSpace(strings.ReplaceAll(string(cleaned), "\r\n", "\n"))

	assert.Equal(t, trimmedOutput, trimmedCleaned)
}

func TestEmptyYaml(t *testing.T) {
	cleaned, err := ScrubYaml(nil)
	require.NoError(t, err)
	assert.Equal(t, "", string(cleaned))

	cleaned, err = ScrubYaml([]byte(""))
	require.NoError(t, err)
	assert.Equal(t, "", string(cleaned))
}

func TestEmptyYamlString(t *testing.T) {
	cleaned, err := ScrubYamlString("")
	require.NoError(t, err)
	assert.Equal(t, "", string(cleaned))
}

func TestAddStrippedKeysExceptions(t *testing.T) {
	t.Run("single key", func(t *testing.T) {
		contents := `api_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'`

		AddStrippedKeys([]string{"api_key"})

		scrubbed, err := ScrubYamlString(contents)
		require.Nil(t, err)
		require.YAMLEq(t, `api_key: '***************************aaaaa'`, scrubbed)
	})

	t.Run("multiple keys", func(t *testing.T) {
		contents := `api_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
some_other_key: 'bbbb'
app_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaacccc'
yet_another_key: 'dddd'`

		keys := []string{"api_key", "some_other_key", "app_key"}
		AddStrippedKeys(keys)

		// check that AddStrippedKeys didn't modify the parameter slice
		assert.Equal(t, []string{"api_key", "some_other_key", "app_key"}, keys)

		scrubbed, err := ScrubYamlString(contents)
		require.Nil(t, err)
		expected := `api_key: '***************************aaaaa'
some_other_key: '********'
app_key: '***********************************acccc'
yet_another_key: 'dddd'`
		require.YAMLEq(t, expected, scrubbed)
	})
}

func TestNewAPIKeyAndAuthPatterns(t *testing.T) {
	testCases := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name: "API Key variations",
			input: map[string]interface{}{
				"APIKEY":  "secret123",
				"API_KEY": "secret456",
				"ApiKey":  "secret789",
				"Api_key": "secret012",
				"api-key": "secret345",
				"apikey":  "secret678",
			},
			expected: map[string]interface{}{
				"APIKEY":  "********",
				"API_KEY": "********",
				"ApiKey":  "********",
				"Api_key": "********",
				"api-key": "********",
				"apikey":  "********",
			},
		},
		{
			name: "Authority and Authorization",
			input: map[string]interface{}{
				"Authority":     "auth123",
				"Authorization": "auth456",
				"authority":     "auth789",
				"authorization": "auth012",
			},
			expected: map[string]interface{}{
				"Authority":     "********",
				"Authorization": "********",
				"authority":     "********",
				"authorization": "********",
			},
		},
		{
			name: "HTTP Header API Keys - X- prefix",
			input: map[string]interface{}{
				"X-API-KEY":              "key123",
				"X-API-Key":              "key456",
				"X-Api-Key":              "key789",
				"X-Auth":                 "auth123",
				"X-Auth-Token":           "token456",
				"X-DreamFactory-Api-Key": "dreamkey789",
				"X-LZ-API-Key":           "lzkey012",
				"X-Rundeck-Auth-Token":   "rundeck345",
				"X-Stratum-Auth":         "stratum678",
				"X-SunGard-IdP-API-Key":  "sungard901",
				"X-VTEX-API-AppKey":      "vtex234",
				"X-Octopus-ApiKey":       "octopus567",
				"x-api-key":              "lowercase890",
				"x-pm-partner-key":       "partner123",
				"x-rapidapi-key":         "rapid456",
				"x-functions-key":        "func789",
			},
			expected: map[string]interface{}{
				"X-API-KEY":              "********",
				"X-API-Key":              "********",
				"X-Api-Key":              "********",
				"X-Auth":                 "********",
				"X-Auth-Token":           "********",
				"X-DreamFactory-Api-Key": "********",
				"X-LZ-API-Key":           "********",
				"X-Rundeck-Auth-Token":   "********",
				"X-Stratum-Auth":         "********",
				"X-SunGard-IdP-API-Key":  "********",
				"X-VTEX-API-AppKey":      "********",
				"X-Octopus-ApiKey":       "********",
				"x-api-key":              "********",
				"x-pm-partner-key":       "********",
				"x-rapidapi-key":         "********",
				"x-functions-key":        "********",
			},
		},
		{
			name: "Specific API Keys and Auth Tokens",
			input: map[string]interface{}{
				"CMS-SVC-API-Key":   "cms123",
				"Sec-WebSocket-Key": "websocket456",
				"auth-tenantId":     "tenant789",
				"cainzapp-api-key":  "cainz012",
				"key":               "key345",
				"key1":              "key678",
				"LODAuth":           "lodauth901",
				"statuskey":         "status234",
			},
			expected: map[string]interface{}{
				"CMS-SVC-API-Key":   "********",
				"Sec-WebSocket-Key": "********",
				"auth-tenantId":     "********",
				"cainzapp-api-key":  "********",
				"key":               "key345", // Too generic - not scrubbed
				"key1":              "key678", // Too generic - not scrubbed
				"LODAuth":           "********",
				"statuskey":         "********",
			},
		},
		{
			name: "Mixed case sensitivity test",
			input: map[string]interface{}{
				"x-api-key": "lowercase123",
				"X-API-KEY": "uppercase456",
				"X-Api-Key": "mixedcase789",
				"x-auth":    "lowerauth012",
				"X-AUTH":    "upperauth345",
				"X-Auth":    "mixedauth678",
			},
			expected: map[string]interface{}{
				"x-api-key": "********",
				"X-API-KEY": "********",
				"X-Api-Key": "********",
				"x-auth":    "********",
				"X-AUTH":    "********",
				"X-Auth":    "********",
			},
		},
		{
			name: "Non-matching keys should not be scrubbed",
			input: map[string]interface{}{
				"regular_config":   "should_not_be_scrubbed",
				"some_other_value": "also_not_scrubbed",
				"normal_setting":   "keep_as_is",
				"database_host":    "localhost",
				"port":             "8080",
				"api_endpoint":     "https://api.example.com",
				"username":         "testuser",
				"email":            "test@example.com",
			},
			expected: map[string]interface{}{
				"regular_config":   "should_not_be_scrubbed",
				"some_other_value": "also_not_scrubbed",
				"normal_setting":   "keep_as_is",
				"database_host":    "localhost",
				"port":             "8080",
				"api_endpoint":     "https://api.example.com",
				"username":         "testuser",
				"email":            "test@example.com",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ScrubDataObj(&tc.input)
			assert.Equal(t, tc.expected, tc.input)
		})
	}
}
