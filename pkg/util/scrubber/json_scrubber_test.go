// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScrubJSON(t *testing.T) {
	testCases := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "simple",
			input:    []byte(`{"password": "secret", "username": "user1"}`),
			expected: []byte(`{"password": "********", "username": "user1"}`),
		},
		{
			name:     "No sensitive info to scrub",
			input:    []byte(`{"message": "hello world", "count": 123}`),
			expected: []byte(`{"message": "hello world", "count": 123}`),
		},
		{
			name:     "nested",
			input:    []byte(`{"user": {"password": "secret", "email": "user@example.com"}}`),
			expected: []byte(`{"user": {"password": "********", "email": "user@example.com"}}`),
		},
		{
			name:     "array",
			input:    []byte(`[{"password": "secret1"}, {"password": "secret2"}]`),
			expected: []byte(`[{"password": "********"}, {"password": "********"}]`),
		},
		{
			name:     "empty object",
			input:    []byte(`{}`),
			expected: []byte(`{}`),
		},
		{
			name:     "complex config with password and tags",
			input:    []byte(`{"host": "localhost", "password": "secret123", "port": 9003, "tags": ["service: api", "partner: aa", "stage: prod", "source: tomcat-jmx"], "user": "admin"}`),
			expected: []byte(`{"host": "localhost", "password": "********", "port": 9003, "tags": ["service: api", "partner: aa", "stage: prod", "source: tomcat-jmx"], "user": "admin"}`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := ScrubJSON(tc.input)
			require.NoError(t, err)

			require.JSONEq(t, string(tc.expected), string(actual))
		})
	}

	t.Run("malformed", func(t *testing.T) {
		scrubber := New()
		scrubber.AddReplacer(SingleLine, Replacer{
			Regex: regexp.MustCompile("foo"),
			Repl:  []byte("bar"),
		})

		input := `{"foo": "bar", "baz"}`
		expected := `{"bar": "bar", "baz"}`
		actual, err := scrubber.ScrubJSON([]byte(input))
		require.NoError(t, err)
		require.Equal(t, expected, string(actual))
	})
}

func TestConfigScrubbedJson(t *testing.T) {
	wd, _ := os.Getwd()

	inputConf := filepath.Join(wd, "test", "config.json")
	inputConfData, err := os.ReadFile(inputConf)
	require.NoError(t, err)
	cleaned, err := ScrubJSON([]byte(inputConfData))
	require.NoError(t, err)
	// First test that the a scrubbed json is still valid
	var actualOutJSON map[string]interface{}
	err = json.Unmarshal(cleaned, &actualOutJSON)
	assert.NoError(t, err, "Could not load JSON configuration after being scrubbed")

	outputConf := filepath.Join(wd, "test", "config_scrubbed.json")
	outputConfData, err := os.ReadFile(outputConf)
	require.NoError(t, err)
	var expectedOutJSON map[string]interface{}
	err = json.Unmarshal(outputConfData, &expectedOutJSON)
	require.NoError(t, err)
	assert.Equal(t, reflect.DeepEqual(expectedOutJSON, actualOutJSON), true)
}

func TestComplexJSONWithNewKeys(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "Complex nested JSON with all new keys",
			input: `{
				"api_config": {
					"endpoints": [
						{
							"name": "primary",
							"x-api-key": "primary_key_123",
							"x-auth-token": "primary_token_456",
							"x-api-secret": "primary_secret_789"
						},
						{
							"name": "secondary",
							"x-goog-api-key": "google_key_abc",
							"x-consul-token": "consul_token_def",
							"x-ibm-client-secret": "ibm_secret_ghi"
						}
					],
					"authentication": {
						"methods": ["oauth", "api_key", "bearer"],
						"credentials": {
							"x-vault-token": "vault_token_123",
							"x-datadog-monitor-token": "dd_monitor_456",
							"x-chalk-client-secret": "chalk_secret_789",
							"private-token": "private_token_abc",
							"kong-admin-token": "kong_admin_def"
						}
					}
				},
				"services": {
					"database": {
						"connection": {
							"host": "localhost",
							"port": 5432,
							"credentials": {
								"username": "dbuser",
								"password": "dbpass123",
								"x-static-token": "static_token_xyz"
							}
						}
					},
					"cache": {
						"redis": {
							"auth": {
								"accesstoken": "redis_access_123",
								"session_token": "redis_session_456",
								"cookie": "redis_cookie_789"
							}
						}
					},
					"external_apis": [
						{
							"name": "sonar",
							"config": {
								"x-sonar-passcode": "sonar_passcode_123",
								"x-seel-api-key": "seel_key_456",
								"x-vtex-api-apptoken": "vtex_apptoken_789"
							}
						},
						{
							"name": "monitoring",
							"config": {
								"x-vtex-api-appkey": "vtex_appkey_abc",
								"authority": "monitoring_auth_def",
								"lodauth": "lodauth_ghi"
							}
						}
					]
				},
				"security": {
					"headers": {
						"authorization": "auth_header_123",
						"cookie": "session_cookie_456",
						"x-auth": "custom_auth_789"
					}
				}
			}`,
			expected: `{
				"api_config": {
					"endpoints": [
						{
							"name": "primary",
							"x-api-key": "********",
							"x-auth-token": "********",
							"x-api-secret": "********"
						},
						{
							"name": "secondary",
							"x-goog-api-key": "********",
							"x-consul-token": "********",
							"x-ibm-client-secret": "********"
						}
					],
					"authentication": {
						"methods": ["oauth", "api_key", "bearer"],
						"credentials": {
							"x-vault-token": "********",
							"x-datadog-monitor-token": "********",
							"x-chalk-client-secret": "********",
							"private-token": "********",
							"kong-admin-token": "********"
						}
					}
				},
				"services": {
					"database": {
						"connection": {
							"host": "localhost",
							"port": 5432,
							"credentials": {
								"username": "dbuser",
								"password": "********",
								"x-static-token": "********"
							}
						}
					},
					"cache": {
						"redis": {
							"auth": {
								"accesstoken": "********",
								"session_token": "********",
								"cookie": "********"
							}
						}
					},
					"external_apis": [
						{
							"name": "sonar",
							"config": {
								"x-sonar-passcode": "********",
								"x-seel-api-key": "********",
								"x-vtex-api-apptoken": "********"
							}
						},
						{
							"name": "monitoring",
							"config": {
								"x-vtex-api-appkey": "********",
								"authority": "********",
								"lodauth": "********"
							}
						}
					]
				},
				"security": {
					"headers": {
						"authorization": "********",
						"cookie": "********",
						"x-auth": "********"
					}
				}
			}`,
		},
		{
			name: "Deeply nested arrays and objects",
			input: `{
				"level1": {
					"level2": [
						{
							"level3": {
								"level4": [
									{
										"x-api-key": "deeply_nested_key_123",
										"x-auth-token": "deeply_nested_token_456",
										"normal_field": "should_not_be_scrubbed"
									},
									{
										"x-api-secret": "deeply_nested_secret_789",
										"x-ibm-client-secret": "deeply_nested_ibm_abc",
										"another_normal_field": "also_not_scrubbed"
									}
								]
							}
						},
						{
							"level3": {
								"level4": {
									"x-vault-token": "another_deep_token_123",
									"x-datadog-monitor-token": "another_deep_dd_456",
									"regular_config": "keep_as_is"
								}
							}
						}
					]
				}
			}`,
			expected: `{
				"level1": {
					"level2": [
						{
							"level3": {
								"level4": [
									{
										"x-api-key": "********",
										"x-auth-token": "********",
										"normal_field": "should_not_be_scrubbed"
									},
									{
										"x-api-secret": "********",
										"x-ibm-client-secret": "********",
										"another_normal_field": "also_not_scrubbed"
									}
								]
							}
						},
						{
							"level3": {
								"level4": {
									"x-vault-token": "********",
									"x-datadog-monitor-token": "********",
									"regular_config": "keep_as_is"
								}
							}
						}
					]
				}
			}`,
		},
		{
			name: "Mixed data types with new keys",
			input: `{
				"string_values": {
					"x-api-key": "string_key_123",
					"x-auth-token": "string_token_456",
					"normal_string": "keep_this"
				},
				"numeric_values": {
					"port": 8080,
					"timeout": 30,
					"x-static-token": "numeric_token_789"
				},
				"boolean_values": {
					"enabled": true,
					"debug": false,
					"x-vault-token": "boolean_token_abc"
				},
				"null_values": {
					"optional_field": null,
					"x-datadog-monitor-token": "null_token_def"
				}
			}`,
			expected: `{
				"string_values": {
					"x-api-key": "********",
					"x-auth-token": "********",
					"normal_string": "keep_this"
				},
				"numeric_values": {
					"port": 8080,
					"timeout": 30,
					"x-static-token": "********"
				},
				"boolean_values": {
					"enabled": true,
					"debug": false,
					"x-vault-token": "********"
				},
				"null_values": {
					"optional_field": null,
					"x-datadog-monitor-token": "********"
				}
			}`,
		},
		{
			name: "Edge cases with empty and special values",
			input: `{
				"empty_strings": {
					"x-api-key": "",
					"x-auth-token": "",
					"normal_empty": ""
				},
				"whitespace_values": {
					"x-api-secret": "   ",
					"x-ibm-client-secret": "\t\n",
					"normal_whitespace": "   "
				},
				"special_characters": {
					"x-chalk-client-secret": "!@#$%^&*()",
					"x-vault-token": "token-with-dashes_and_underscores.123",
					"normal_special": "!@#$%^&*()"
				}
			}`,
			expected: `{
				"empty_strings": {
					"x-api-key": "********",
					"x-auth-token": "********",
					"normal_empty": ""
				},
				"whitespace_values": {
					"x-api-secret": "********",
					"x-ibm-client-secret": "********",
					"normal_whitespace": "   "
				},
				"special_characters": {
					"x-chalk-client-secret": "********",
					"x-vault-token": "********",
					"normal_special": "!@#$%^&*()"
				}
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cleaned, err := ScrubJSON([]byte(tc.input))
			require.NoError(t, err)

			// Verify the cleaned JSON is still valid
			var jsonData interface{}
			err = json.Unmarshal(cleaned, &jsonData)
			assert.NoError(t, err, "Cleaned JSON should be valid")

			// Compare JSON structures using JSONEq for semantic equality
			require.JSONEq(t, tc.expected, string(cleaned))
		})
	}
}

func TestJSONStringScrubbingWithNewKeys(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "Complex JSON string with nested structures",
			input: `{
				"api_config": {
					"primary_endpoint": {
						"url": "https://api.example.com",
						"authentication": {
							"x-api-key": "primary_api_key_12345",
							"x-auth-token": "primary_auth_token_67890",
							"x-api-secret": "primary_api_secret_abcdef"
						}
					},
					"secondary_endpoints": [
						{
							"name": "google",
							"config": {
								"x-goog-api-key": "google_api_key_ghijk",
								"x-consul-token": "consul_token_lmnop"
							}
						},
						{
							"name": "ibm",
							"config": {
								"x-ibm-client-secret": "ibm_client_secret_qrstu",
								"x-chalk-client-secret": "chalk_client_secret_vwxyz"
							}
						}
					]
				},
				"services": {
					"database": {
						"connection": {
							"host": "localhost",
							"port": 5432,
							"credentials": {
								"username": "dbuser",
								"password": "dbpassword123",
								"x-static-token": "static_token_12345"
							}
						}
					},
					"cache": {
						"redis": {
							"auth": {
								"accesstoken": "redis_access_token_67890",
								"session_token": "redis_session_token_abcdef",
								"cookie": "redis_cookie_ghijk"
							}
						}
					},
					"external_apis": [
						{
							"name": "sonar",
							"config": {
								"x-sonar-passcode": "sonar_passcode_lmnop",
								"x-seel-api-key": "seel_api_key_qrstu",
								"x-vtex-api-apptoken": "vtex_app_token_vwxyz"
							}
						},
						{
							"name": "monitoring",
							"config": {
								"x-vtex-api-appkey": "vtex_app_key_12345",
								"authority": "monitoring_authority_67890",
								"lodauth": "lodauth_abcdef"
							}
						}
					]
				},
				"security": {
					"headers": {
						"authorization": "auth_header_67890",
						"cookie": "session_cookie_abcdef",
						"x-auth": "custom_auth_ghijk",
						"private-token": "private_token_lmnop",
						"kong-admin-token": "kong_admin_token_qrstu"
					}
				},
				"app_name": "my_application",
				"version": "1.0.0",
				"debug": true,
				"log_level": "info"
			}`,
			expected: `{
				"api_config": {
					"primary_endpoint": {
						"url": "https://api.example.com",
						"authentication": {
							"x-api-key": "********",
							"x-auth-token": "********",
							"x-api-secret": "********"
						}
					},
					"secondary_endpoints": [
						{
							"name": "google",
							"config": {
								"x-goog-api-key": "********",
								"x-consul-token": "********"
							}
						},
						{
							"name": "ibm",
							"config": {
								"x-ibm-client-secret": "********",
								"x-chalk-client-secret": "********"
							}
						}
					]
				},
				"services": {
					"database": {
						"connection": {
							"host": "localhost",
							"port": 5432,
							"credentials": {
								"username": "dbuser",
								"password": "********",
								"x-static-token": "********"
							}
						}
					},
					"cache": {
						"redis": {
							"auth": {
								"accesstoken": "********",
								"session_token": "********",
								"cookie": "********"
							}
						}
					},
					"external_apis": [
						{
							"name": "sonar",
							"config": {
								"x-sonar-passcode": "********",
								"x-seel-api-key": "********",
								"x-vtex-api-apptoken": "********"
							}
						},
						{
							"name": "monitoring",
							"config": {
								"x-vtex-api-appkey": "********",
								"authority": "********",
								"lodauth": "********"
							}
						}
					]
				},
				"security": {
					"headers": {
						"authorization": "********",
						"cookie": "********",
						"x-auth": "********",
						"private-token": "********",
						"kong-admin-token": "********"
					}
				},
				"app_name": "my_application",
				"version": "1.0.0",
				"debug": true,
				"log_level": "info"
			}`,
		},
		{
			name: "JSON with arrays containing sensitive data",
			input: `{
				"authentication_methods": [
					{
						"method": "oauth",
						"config": {
							"x-api-key": "oauth_key_123",
							"x-auth-token": "oauth_token_456"
						}
					},
					{
						"method": "api_key",
						"config": {
							"x-api-secret": "api_secret_789",
							"x-ibm-client-secret": "ibm_secret_abc"
						}
					},
					{
						"method": "bearer",
						"config": {
							"x-vault-token": "vault_token_def",
							"x-datadog-monitor-token": "dd_monitor_ghi"
						}
					}
				],
				"service_configs": [
					{
						"name": "web",
						"secrets": {
							"x-chalk-client-secret": "web_chalk_secret_123",
							"x-static-token": "web_static_token_456"
						}
					},
					{
						"name": "api",
						"secrets": {
							"private-token": "api_private_token_789",
							"kong-admin-token": "api_kong_admin_abc"
						}
					},
					{
						"name": "worker",
						"secrets": {
							"accesstoken": "worker_access_token_def",
							"session_token": "worker_session_token_ghi",
							"cookie": "worker_cookie_jkl"
						}
					}
				],
				"database_url": "postgresql://localhost:5432/mydb",
				"redis_url": "redis://localhost:6379"
			}`,
			expected: `{
				"authentication_methods": [
					{
						"method": "oauth",
						"config": {
							"x-api-key": "********",
							"x-auth-token": "********"
						}
					},
					{
						"method": "api_key",
						"config": {
							"x-api-secret": "********",
							"x-ibm-client-secret": "********"
						}
					},
					{
						"method": "bearer",
						"config": {
							"x-vault-token": "********",
							"x-datadog-monitor-token": "********"
						}
					}
				],
				"service_configs": [
					{
						"name": "web",
						"secrets": {
							"x-chalk-client-secret": "********",
							"x-static-token": "********"
						}
					},
					{
						"name": "api",
						"secrets": {
							"private-token": "********",
							"kong-admin-token": "********"
						}
					},
					{
						"name": "worker",
						"secrets": {
							"accesstoken": "********",
							"session_token": "********",
							"cookie": "********"
						}
					}
				],
				"database_url": "postgresql://localhost:5432/mydb",
				"redis_url": "redis://localhost:6379"
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cleaned, err := ScrubJSON([]byte(tc.input))
			require.NoError(t, err)

			// Verify the cleaned JSON is still valid
			var jsonData interface{}
			err = json.Unmarshal(cleaned, &jsonData)
			assert.NoError(t, err, "Cleaned JSON should be valid")

			// Compare JSON structures using JSONEq for semantic equality
			require.JSONEq(t, tc.expected, string(cleaned))
		})
	}
}
