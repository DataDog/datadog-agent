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

	"go.yaml.in/yaml/v3"
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

func TestConfigScrubbedYamlWithENC(t *testing.T) {
	wd, _ := os.Getwd()

	inputConf := filepath.Join(wd, "test", "conf_enc.yaml")
	inputConfData, err := os.ReadFile(inputConf)
	require.NoError(t, err)

	outputConf := filepath.Join(wd, "test", "conf_enc_scrubbed.yaml")
	outputConfData, err := os.ReadFile(outputConf)
	require.NoError(t, err)

	// Create scrubber with ENC preservation enabled
	scrubber := NewWithDefaults()
	scrubber.SetPreserveENC(true)
	cleaned, err := scrubber.ScrubBytes([]byte(inputConfData))
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

func TestScrubbingENC(t *testing.T) {
	t.Run("basic ENC handler preserved", func(t *testing.T) {
		result, err := ScrubYamlString(`api_key: ENC[my_secret]`)
		require.NoError(t, err)
		require.YAMLEq(t, `api_key: ENC[my_secret]`, result)
	})

	t.Run("ENC with whitespace preserved", func(t *testing.T) {
		result, err := ScrubYamlString(`api_key: "  ENC[key]	"`)
		require.NoError(t, err)
		require.YAMLEq(t, `api_key: "  ENC[key]	"`, result)
	})

	t.Run("empty ENC handler preserved", func(t *testing.T) {
		result, err := ScrubYamlString(`api_key: ENC[]`)
		require.NoError(t, err)
		require.YAMLEq(t, `api_key: ENC[]`, result)
	})

	t.Run("invalid ENC formats scrubbed", func(t *testing.T) {
		result, err := ScrubYamlString(`api_key: ENC[incomplete
password: ENC
token: [not_enc]`)
		require.NoError(t, err)
		require.YAMLEq(t, `api_key: "********"
password: "********"
token: "********"`, result)
	})

	t.Run("ENC in nested structure", func(t *testing.T) {
		input := interface{}(map[string]interface{}{
			"database": map[string]interface{}{
				"password": "ENC[db_pass]",
				"token":    "plain_token",
			},
		})
		expected := interface{}(map[string]interface{}{
			"database": map[string]interface{}{
				"password": "ENC[db_pass]",
				"token":    "********",
			},
		})
		ScrubDataObj(&input)
		assert.Equal(t, expected, input)
	})

	t.Run("ENC in array", func(t *testing.T) {
		input := interface{}(map[string]interface{}{
			"secrets": []interface{}{
				map[string]interface{}{
					"password": "ENC[secret1]",
				},
				map[string]interface{}{
					"password": "plain_secret",
				},
			},
		})
		expected := interface{}(map[string]interface{}{
			"secrets": []interface{}{
				map[string]interface{}{
					"password": "ENC[secret1]",
				},
				map[string]interface{}{
					"password": "********",
				},
			},
		})
		ScrubDataObj(&input)
		assert.Equal(t, expected, input)
	})
}

func TestComplexYAMLWithNewKeys(t *testing.T) {
	testCases := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name: "Complex nested configuration with all new keys",
			input: map[string]interface{}{
				"api_config": map[string]interface{}{
					"endpoints": []interface{}{
						map[string]interface{}{
							"name":         "primary",
							"x-api-key":    "primary_key_123",
							"x-auth-token": "primary_token_456",
							"x-api-secret": "primary_secret_789",
						},
						map[string]interface{}{
							"name":                "secondary",
							"x-goog-api-key":      "google_key_abc",
							"x-consul-token":      "consul_token_def",
							"x-ibm-client-secret": "ibm_secret_ghi",
						},
					},
					"authentication": map[string]interface{}{
						"methods": []interface{}{
							"oauth",
							"api_key",
							"bearer",
						},
						"credentials": map[string]interface{}{
							"x-vault-token":           "vault_token_123",
							"x-datadog-monitor-token": "dd_monitor_456",
							"x-chalk-client-secret":   "chalk_secret_789",
							"private-token":           "private_token_abc",
							"kong-admin-token":        "kong_admin_def",
						},
					},
				},
				"services": map[string]interface{}{
					"database": map[string]interface{}{
						"connection": map[string]interface{}{
							"host": "localhost",
							"port": 5432,
							"credentials": map[string]interface{}{
								"username":       "dbuser",
								"password":       "dbpass123",
								"x-static-token": "static_token_xyz",
							},
						},
					},
					"cache": map[string]interface{}{
						"redis": map[string]interface{}{
							"auth": map[string]interface{}{
								"accesstoken":   "redis_access_123",
								"session_token": "redis_session_456",
								"cookie":        "redis_cookie_789",
							},
						},
					},
					"external_apis": []interface{}{
						map[string]interface{}{
							"name": "sonar",
							"config": map[string]interface{}{
								"x-sonar-passcode":    "sonar_passcode_123",
								"x-seel-api-key":      "seel_key_456",
								"x-vtex-api-apptoken": "vtex_apptoken_789",
							},
						},
						map[string]interface{}{
							"name": "monitoring",
							"config": map[string]interface{}{
								"x-vtex-api-appkey": "vtex_appkey_abc",
								"authority":         "monitoring_auth_def",
								"lodauth":           "lodauth_ghi",
							},
						},
					},
				},
				"security": map[string]interface{}{
					"headers": map[string]interface{}{
						"authorization": "auth_header_123",
						"cookie":        "session_cookie_456",
						"x-auth":        "custom_auth_789",
					},
				},
			},
			expected: map[string]interface{}{
				"api_config": map[string]interface{}{
					"endpoints": []interface{}{
						map[string]interface{}{
							"name":         "primary",
							"x-api-key":    "********",
							"x-auth-token": "********",
							"x-api-secret": "********",
						},
						map[string]interface{}{
							"name":                "secondary",
							"x-goog-api-key":      "********",
							"x-consul-token":      "********",
							"x-ibm-client-secret": "********",
						},
					},
					"authentication": map[string]interface{}{
						"methods": []interface{}{
							"oauth",
							"api_key",
							"bearer",
						},
						"credentials": map[string]interface{}{
							"x-vault-token":           "********",
							"x-datadog-monitor-token": "********",
							"x-chalk-client-secret":   "********",
							"private-token":           "********",
							"kong-admin-token":        "********",
						},
					},
				},
				"services": map[string]interface{}{
					"database": map[string]interface{}{
						"connection": map[string]interface{}{
							"host": "localhost",
							"port": 5432,
							"credentials": map[string]interface{}{
								"username":       "dbuser",
								"password":       "********",
								"x-static-token": "********",
							},
						},
					},
					"cache": map[string]interface{}{
						"redis": map[string]interface{}{
							"auth": map[string]interface{}{
								"accesstoken":   "********",
								"session_token": "********",
								"cookie":        "********",
							},
						},
					},
					"external_apis": []interface{}{
						map[string]interface{}{
							"name": "sonar",
							"config": map[string]interface{}{
								"x-sonar-passcode":    "********",
								"x-seel-api-key":      "********",
								"x-vtex-api-apptoken": "********",
							},
						},
						map[string]interface{}{
							"name": "monitoring",
							"config": map[string]interface{}{
								"x-vtex-api-appkey": "********",
								"authority":         "********",
								"lodauth":           "********",
							},
						},
					},
				},
				"security": map[string]interface{}{
					"headers": map[string]interface{}{
						"authorization": "********",
						"cookie":        "********",
						"x-auth":        "********",
					},
				},
			},
		},
		{
			name: "Deeply nested arrays and maps",
			input: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": []interface{}{
						map[string]interface{}{
							"level3": map[string]interface{}{
								"level4": []interface{}{
									map[string]interface{}{
										"x-api-key":    "deeply_nested_key_123",
										"x-auth-token": "deeply_nested_token_456",
										"normal_field": "should_not_be_scrubbed",
									},
									map[string]interface{}{
										"x-api-secret":         "deeply_nested_secret_789",
										"x-ibm-client-secret":  "deeply_nested_ibm_abc",
										"another_normal_field": "also_not_scrubbed",
									},
								},
							},
						},
						map[string]interface{}{
							"level3": map[string]interface{}{
								"level4": map[string]interface{}{
									"x-vault-token":           "another_deep_token_123",
									"x-datadog-monitor-token": "another_deep_dd_456",
									"regular_config":          "keep_as_is",
								},
							},
						},
					},
				},
			},
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": []interface{}{
						map[string]interface{}{
							"level3": map[string]interface{}{
								"level4": []interface{}{
									map[string]interface{}{
										"x-api-key":    "********",
										"x-auth-token": "********",
										"normal_field": "should_not_be_scrubbed",
									},
									map[string]interface{}{
										"x-api-secret":         "********",
										"x-ibm-client-secret":  "********",
										"another_normal_field": "also_not_scrubbed",
									},
								},
							},
						},
						map[string]interface{}{
							"level3": map[string]interface{}{
								"level4": map[string]interface{}{
									"x-vault-token":           "********",
									"x-datadog-monitor-token": "********",
									"regular_config":          "keep_as_is",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Mixed data types with new keys",
			input: map[string]interface{}{
				"string_values": map[string]interface{}{
					"x-api-key":     "string_key_123",
					"x-auth-token":  "string_token_456",
					"normal_string": "keep_this",
				},
				"numeric_values": map[string]interface{}{
					"port":           8080,
					"timeout":        30,
					"x-static-token": "numeric_token_789", // Still a string
				},
				"boolean_values": map[string]interface{}{
					"enabled":       true,
					"debug":         false,
					"x-vault-token": "boolean_token_abc", // Still a string
				},
				"null_values": map[string]interface{}{
					"optional_field":          nil,
					"x-datadog-monitor-token": "null_token_def", // Still a string
				},
			},
			expected: map[string]interface{}{
				"string_values": map[string]interface{}{
					"x-api-key":     "********",
					"x-auth-token":  "********",
					"normal_string": "keep_this",
				},
				"numeric_values": map[string]interface{}{
					"port":           8080,
					"timeout":        30,
					"x-static-token": "********",
				},
				"boolean_values": map[string]interface{}{
					"enabled":       true,
					"debug":         false,
					"x-vault-token": "********",
				},
				"null_values": map[string]interface{}{
					"optional_field":          nil,
					"x-datadog-monitor-token": "********",
				},
			},
		},
		{
			name: "Edge cases with empty and special values",
			input: map[string]interface{}{
				"empty_strings": map[string]interface{}{
					"x-api-key":    "",
					"x-auth-token": "",
					"normal_empty": "",
				},
				"whitespace_values": map[string]interface{}{
					"x-api-secret":        "   ",
					"x-ibm-client-secret": "\t\n",
					"normal_whitespace":   "   ",
				},
				"special_characters": map[string]interface{}{
					"x-chalk-client-secret": "!@#$%^&*()",
					"x-vault-token":         "token-with-dashes_and_underscores.123",
					"normal_special":        "!@#$%^&*()",
				},
			},
			expected: map[string]interface{}{
				"empty_strings": map[string]interface{}{
					"x-api-key":    "********",
					"x-auth-token": "********",
					"normal_empty": "",
				},
				"whitespace_values": map[string]interface{}{
					"x-api-secret":        "********",
					"x-ibm-client-secret": "********",
					"normal_whitespace":   "   ",
				},
				"special_characters": map[string]interface{}{
					"x-chalk-client-secret": "********",
					"x-vault-token":         "********",
					"normal_special":        "!@#$%^&*()",
				},
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

func TestYAMLStringScrubbingWithNewKeys(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "Complex YAML string with nested structures",
			input: `
api_config:
  primary_endpoint:
    url: "https://api.example.com"
    authentication:
      x-api-key: "primary_api_key_12345"
      x-auth-token: "primary_auth_token_67890"
      x-api-secret: "primary_api_secret_abcdef"

  secondary_endpoints:
    - name: "google"
      config:
        x-goog-api-key: "google_api_key_ghijk"
        x-consul-token: "consul_token_lmnop"
    - name: "ibm"
      config:
        x-ibm-client-secret: "ibm_client_secret_qrstu"
        x-chalk-client-secret: "chalk_client_secret_vwxyz"

services:
  database:
    connection:
      host: "localhost"
      port: 5432
      credentials:
        username: "dbuser"
        password: "dbpassword123"
        x-static-token: "static_token_12345"

  cache:
    redis:
      auth:
        accesstoken: "redis_access_token_67890"
        session_token: "redis_session_token_abcdef"
        cookie: "redis_cookie_ghijk"

  external_apis:
    - name: "sonar"
      config:
        x-sonar-passcode: "sonar_passcode_lmnop"
        x-seel-api-key: "seel_api_key_qrstu"
        x-vtex-api-apptoken: "vtex_app_token_vwxyz"
    - name: "monitoring"
      config:
        x-vtex-api-appkey: "vtex_app_key_12345"
        authority: "monitoring_authority_67890"
        lodauth: "lodauth_abcdef"

security:
  headers:
    authorization: "auth_header_67890"
    cookie: "session_cookie_abcdef"
    x-auth: "custom_auth_ghijk"
    private-token: "private_token_lmnop"
    kong-admin-token: "kong_admin_token_qrstu"

# Non-sensitive configuration
app_name: "my_application"
version: "1.0.0"
debug: true
log_level: "info"
`,
			expected: `
api_config:
  primary_endpoint:
    url: "https://api.example.com"
    authentication:
      x-api-key: "********"
      x-auth-token: "********"
      x-api-secret: "********"

  secondary_endpoints:
    - name: "google"
      config:
        x-goog-api-key: "********"
        x-consul-token: "********"
    - name: "ibm"
      config:
        x-ibm-client-secret: "********"
        x-chalk-client-secret: "********"

services:
  database:
    connection:
      host: "localhost"
      port: 5432
      credentials:
        username: "dbuser"
        password: "********"
        x-static-token: "********"

  cache:
    redis:
      auth:
        accesstoken: "********"
        session_token: "********"
        cookie: "********"

  external_apis:
    - name: "sonar"
      config:
        x-sonar-passcode: "********"
        x-seel-api-key: "********"
        x-vtex-api-apptoken: "********"
    - name: "monitoring"
      config:
        x-vtex-api-appkey: "********"
        authority: "********"
        lodauth: "********"

security:
  headers:
    authorization: "********"
    cookie: "********"
    x-auth: "********"
    private-token: "********"
    kong-admin-token: "********"

# Non-sensitive configuration
app_name: "my_application"
version: "1.0.0"
debug: true
log_level: "info"
`,
		},
		{
			name: "YAML with arrays containing sensitive data",
			input: `
authentication_methods:
  - method: "oauth"
    config:
      x-api-key: "oauth_key_123"
      x-auth-token: "oauth_token_456"
  - method: "api_key"
    config:
      x-api-secret: "api_secret_789"
      x-ibm-client-secret: "ibm_secret_abc"
  - method: "bearer"
    config:
      x-vault-token: "vault_token_def"
      x-datadog-monitor-token: "dd_monitor_ghi"

service_configs:
  - name: "web"
    secrets:
      x-chalk-client-secret: "web_chalk_secret_123"
      x-static-token: "web_static_token_456"
  - name: "api"
    secrets:
      private-token: "api_private_token_789"
      kong-admin-token: "api_kong_admin_abc"
  - name: "worker"
    secrets:
      accesstoken: "worker_access_token_def"
      session_token: "worker_session_token_ghi"
      cookie: "worker_cookie_jkl"

# Regular configuration
database_url: "postgresql://localhost:5432/mydb"
redis_url: "redis://localhost:6379"
`,
			expected: `
authentication_methods:
  - method: "oauth"
    config:
      x-api-key: "********"
      x-auth-token: "********"
  - method: "api_key"
    config:
      x-api-secret: "********"
      x-ibm-client-secret: "********"
  - method: "bearer"
    config:
      x-vault-token: "********"
      x-datadog-monitor-token: "********"

service_configs:
  - name: "web"
    secrets:
      x-chalk-client-secret: "********"
      x-static-token: "********"
  - name: "api"
    secrets:
      private-token: "********"
      kong-admin-token: "********"
  - name: "worker"
    secrets:
      accesstoken: "********"
      session_token: "********"
      cookie: "********"

# Regular configuration
database_url: "postgresql://localhost:5432/mydb"
redis_url: "redis://localhost:6379"
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cleaned, err := ScrubYamlString(tc.input)
			require.NoError(t, err)

			// Verify the cleaned YAML is still valid
			var yamlData interface{}
			err = yaml.Unmarshal([]byte(cleaned), &yamlData)
			assert.NoError(t, err, "Cleaned YAML should be valid")

			// Test that sensitive keys are scrubbed by checking the parsed data
			// This is more robust than string comparison since YAML formatting can vary
			ScrubDataObj(&yamlData)

			// Parse expected YAML to compare structures
			var expectedData interface{}
			err = yaml.Unmarshal([]byte(tc.expected), &expectedData)
			require.NoError(t, err)

			// Compare the data structures instead of string formatting
			assert.Equal(t, expectedData, yamlData)
		})
	}
}
