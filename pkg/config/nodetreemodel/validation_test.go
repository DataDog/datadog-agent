// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestUnknownEnvVars(t *testing.T) {
	cfg := NewNodeTreeConfig("datadog", "DD", nil)
	cfg.BindEnvAndSetDefault("unknown_env_test.api_key", "")
	cfg.BindEnvAndSetDefault("unknown_env_test.custom", "", "DD_UNKNOWN_ENV_TEST_CUSTOM_NAME")
	cfg.BuildSchema()

	// env vars bound by another config instance are known too
	other := NewNodeTreeConfig("system-probe", "DD", nil)
	other.BindEnvAndSetDefault("unknown_env_test.system_probe_setting", "")
	other.BuildSchema()

	registerKnownEnvVars("DD_UNKNOWN_ENV_TEST_EXTRA")

	tests := []struct {
		envVar  string
		unknown bool
	}{
		{"DD_UNKNOWN_ENV_TEST_API_KEY", false},
		{"DD_UNKNOWN_ENV_TEST_CUSTOM_NAME", false},
		{"DD_UNKNOWN_ENV_TEST_SYSTEM_PROBE_SETTING", false},
		{"DD_UNKNOWN_ENV_TEST_EXTRA", false},
		{"DD_PROXY_HTTP", false},
		{"DD_INSIDE_CI", false},
		{"DD_UNKNOWN_ENV_TEST_UNKNOWN", true},
		{"UNKNOWN_ENV_TEST", false}, // no DD_ prefix
	}
	for _, tc := range tests {
		t.Run(tc.envVar, func(t *testing.T) {
			var expected []string
			if tc.unknown {
				expected = []string{"Unknown environment variable: " + tc.envVar}
			}
			c := &ntmConfig{}
			c.findUnknownEnvVars([]string{tc.envVar + "=foo"})
			assert.Equal(t, expected, c.warnings)
		})
	}
}

func TestValidateReportsUnknownEnvVars(t *testing.T) {
	t.Setenv("DD_UNKNOWN_ENV_TEST_VALIDATE", "foo")

	cfg := NewNodeTreeConfig("datadog", "DD", nil)
	cfg.BuildSchema()
	cfg.(*ntmConfig).Validate()

	require.Contains(t, cfg.Warnings(), "Unknown environment variable: DD_UNKNOWN_ENV_TEST_VALIDATE")
}

func TestCleanStrings(t *testing.T) {
	cfg := &ntmConfig{}
	cfg.findUnexpectedCodepoints("key", "container_collect_some", false)
	assert.Empty(t, cfg.warnings)
}

func TestDirtyStrings(t *testing.T) {
	cfg := &ntmConfig{}
	cfg.findUnexpectedCodepoints("key", "\u202acontainer_collect_some", false)
	warnings := cfg.warnings
	require.Len(t, warnings, 1)
	assert.Equal(t, "For key 'key', configuration value string '\u202acontainer_collect_some' - Unexpected unicode Bidirectional control codepoint 'U+202A' detected at byte position 0", warnings[0])
}

func TestDirtySecretStrings(t *testing.T) {
	cfg := &ntmConfig{}
	cfg.findUnexpectedCodepoints("key", "\u202asecret_value", true)
	warnings := cfg.warnings
	require.Len(t, warnings, 1)
	assert.Equal(t, "For key 'key', configuration value string '<REDACTED SECRET>' - Unexpected unicode Bidirectional control codepoint 'U+202A' detected at byte position 0", warnings[0])
}

func TestVariousCodepoints(t *testing.T) {
	tests := []struct {
		input              string
		expectedCodepoints []string
	}{
		{
			input:              "hello world",
			expectedCodepoints: nil,
		},
		{
			input:              "hello \u202aworld",
			expectedCodepoints: []string{"U+202A"},
		},
		{
			input:              "hello \nworld",
			expectedCodepoints: nil,
		},
		{
			input:              "hello\u00b7world\U0001f63f",
			expectedCodepoints: nil,
		},
		{
			input:              "test\u200bing",
			expectedCodepoints: []string{"U+200B"},
		},
		{
			input:              "test\u202fte\u200asti\u2000ng",
			expectedCodepoints: []string{"U+202F", "U+200A", "U+2000"},
		},
		{
			input:              "invalid\xffutf8",
			expectedCodepoints: []string{"U+FFFD"},
		},
	}
	for _, tc := range tests {
		cfg := &ntmConfig{}
		cfg.findUnexpectedCodepoints("key", tc.input, false)
		warnings := cfg.warnings
		require.Len(t, warnings, len(tc.expectedCodepoints), "input: %q", tc.input)
		for i, expectedCodepoint := range tc.expectedCodepoints {
			assert.Contains(t, warnings[i], "'"+expectedCodepoint+"'")
		}
	}
}

func newUnicodeTestConfig(t *testing.T, yamlConfig string) *ntmConfig {
	cfg := NewNodeTreeConfig("datadog", "DD", nil)
	cfg.SetDefault("api_key", "")
	cfg.SetDefault("root_element.nestedKey", "")
	cfg.SetDefault("runtime_security_config.activity_dump.remote_storage.endpoints.logs_dd_url", "")
	cfg.BuildSchema()
	cfg.SetConfigType("yaml")
	require.NoError(t, cfg.ReadConfig(strings.NewReader(yamlConfig)))
	return cfg.(*ntmConfig)
}

// unexpectedUnicodeWarnings returns only the warnings added by findUnexpectedUnicode, ignoring the ones emitted
// while loading the config
func unexpectedUnicodeWarnings(cfg *ntmConfig) []string {
	cfg.warnings = nil
	cfg.findUnexpectedUnicode()
	return cfg.warnings
}

func TestUnexpectedUnicode(t *testing.T) {
	keyYaml := "api_\u202akey: fakeapikey\n"
	valueYaml := "api_key: fa\u202akeapikey\n"

	warnings := unexpectedUnicodeWarnings(newUnicodeTestConfig(t, keyYaml))
	require.Len(t, warnings, 1)

	assert.Contains(t, warnings[0], "Configuration key string")
	assert.Contains(t, warnings[0], "U+202A")

	warnings = unexpectedUnicodeWarnings(newUnicodeTestConfig(t, valueYaml))

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "For key 'api_key'")
	assert.Contains(t, warnings[0], "U+202A")
}

func TestUnexpectedUnicodeInSecretIsRedacted(t *testing.T) {
	cfg := newUnicodeTestConfig(t, "api_key: ENC[api_key]\n")
	cfg.Set("api_key", "fa\u202akeapikey", model.SourceSecret)

	warnings := unexpectedUnicodeWarnings(cfg)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "For key 'api_key', configuration value string '<REDACTED SECRET>'")
	assert.Contains(t, warnings[0], "U+202A")
	assert.NotContains(t, warnings[0], "keapikey")
}

func TestUnexpectedNestedUnicode(t *testing.T) {
	yaml := "runtime_security_config:\n  activity_dump:\n    remote_storage:\n      endpoints:\n        logs_dd_url: \"http://\u202adatadawg.com\""

	warnings := unexpectedUnicodeWarnings(newUnicodeTestConfig(t, yaml))
	require.Len(t, warnings, 1)

	assert.Contains(t, warnings[0], "U+202A")
	assert.Contains(t, warnings[0], "For key 'runtime_security_config.activity_dump.remote_storage.endpoints.logs_dd_url'")
}

func TestUnexpectedWhitespace(t *testing.T) {
	tests := []struct {
		yaml                string
		expectedWarningText string
		expectedPosition    string
	}{
		{
			yaml:                "root_element:\n  nestedKey: \"hiddenI\u200bnvalidWhitespaceEmbedded\n\"",
			expectedWarningText: "U+200B",
			expectedPosition:    fmt.Sprintf("position %d", 7),
		},
		{
			yaml:                "root_element:\n  nestedKey: \u202fhiddenInvalidWhitespaceToLeft\n",
			expectedWarningText: "U+202F",
			expectedPosition:    fmt.Sprintf("position %d", 0),
		},
		{
			yaml:                "root_element:\n  nestedKey: [validValue, \u202fhiddenInvalidWhitespaceToLeft]\n",
			expectedWarningText: "U+202F",
			expectedPosition:    fmt.Sprintf("position %d", 0),
		},
	}
	for _, tc := range tests {
		warnings := unexpectedUnicodeWarnings(newUnicodeTestConfig(t, tc.yaml))
		require.Len(t, warnings, 1)

		assert.Contains(t, warnings[0], tc.expectedWarningText)
		assert.Contains(t, warnings[0], tc.expectedPosition)
	}
}

func TestValidateReportsUnexpectedUnicode(t *testing.T) {
	cfg := newUnicodeTestConfig(t, "api_key: fa\u202akeapikey\n")
	cfg.Validate()

	warnings := cfg.Warnings()
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "For key 'api_key'")
	assert.Contains(t, warnings[0], "U+202A")
}
