// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScrubYamlWithSecrets(t *testing.T) {
	input := []byte(`api_key: ENC[main_key]
logs_config:
  api_key: raw-logs-key
init_config:
  password: ENC[shared_password]
instances:
  - password: raw-password
  - password: ENC[shared_password]
  - password: 123456
  - password: !custom ENC[tagged]
  - password: |
      multiline-password
logs:
  - password: raw-log-password
jmx_metrics:
  - password: ENC[shared_password]
snmp_traps:
  community_strings: [raw-community, 'ENC[community]']
password: false
client_secret: ""
already_redacted:
  password: "********"
`)
	cleaned, secrets, err := NewWithDefaults().ScrubYamlWithSecrets(input)
	require.NoError(t, err)
	assert.ElementsMatch(t, []ConfigSecret{
		{Path: "api_key", Kind: "enc", Handle: "main_key"},
		{Path: "logs_config.api_key", Kind: "raw"},
		{Path: "init_config.password", Kind: "enc", Handle: "shared_password"},
		{Path: "instances.0.password", Kind: "raw"},
		{Path: "instances.1.password", Kind: "enc", Handle: "shared_password"},
		{Path: "instances.2.password", Kind: "raw"},
		{Path: "instances.3.password", Kind: "enc", Handle: "tagged"},
		{Path: "instances.4.password", Kind: "raw"},
		{Path: "logs.0.password", Kind: "raw"},
		{Path: "jmx_metrics.0.password", Kind: "enc", Handle: "shared_password"},
		{Path: "snmp_traps.community_strings", Kind: "raw"},
		{Path: "snmp_traps.community_strings.1", Kind: "enc", Handle: "community"},
	}, secrets)
	encoded, err := json.Marshal(secrets)
	require.NoError(t, err)
	for _, secret := range []string{"raw-logs-key", "raw-password", "raw-log-password", "raw-community", "123456", "multiline-password"} {
		assert.NotContains(t, string(cleaned), secret)
		assert.NotContains(t, string(encoded), secret)
	}
}

func TestScrubYamlWithSecretsRejectsUnsafeInput(t *testing.T) {
	for _, input := range []string{"password: [broken", "password: value\n---\npassword: other", "password: &p value\nother: *p", "password: a\npassword: b"} {
		cleaned, secrets, err := NewWithDefaults().ScrubYamlWithSecrets([]byte(input))
		require.Error(t, err)
		assert.Nil(t, cleaned)
		assert.Nil(t, secrets)
	}
}

func TestScrubYamlWithSecretsDoesNotLeakCredentialsInMapKeys(t *testing.T) {
	scrubber := NewWithDefaults()
	cleaned, secrets, err := scrubber.ScrubYamlWithSecrets([]byte(`additional_endpoints:
  https://user:map-password@endpoint.example: [raw-key, 'ENC[endpoint_key]']
`))
	require.NoError(t, err)
	encoded, err := json.Marshal([]any{secrets, scrubber.GetStrippedKeys()})
	require.NoError(t, err)
	for _, value := range []string{"map-password", "raw-key"} {
		assert.NotContains(t, string(cleaned), value)
		assert.NotContains(t, string(encoded), value)
	}
	assert.ElementsMatch(t, []ConfigSecret{
		{Path: "additional_endpoints", Kind: "raw"},
		{Path: "additional_endpoints.https://user:********@endpoint.example.1", Kind: "enc", Handle: "endpoint_key"},
	}, secrets)
}

func TestScrubYamlWithSecretsPreservesStructuredReplacementMetadata(t *testing.T) {
	input := []byte(`logs_config:
  additional_endpoints:
    - host: example.com
      port: 0x1f90
      api_key: raw-endpoint-key
    - host: other.example.com
      api_key: ENC[shared]
private_action_runner:
  enabled: true
  credentials:
    values:
      z_last: {value: raw-credential}
      a_first: {value: 'ENC[shared]'}
scanning_rules:
  - id: visible-id
    license: visible-license
    pattern: raw-pattern
    proximity_keywords:
      included_keywords: [raw-keyword, 'ENC[shared]']
snmp_traps:
  community_strings: [raw-community, 'ENC[shared]', false, null, '']
`)
	scrubber := NewWithDefaults()
	cleaned, secrets, err := scrubber.ScrubYamlWithSecrets(input)
	require.NoError(t, err)
	assert.ElementsMatch(t, []ConfigSecret{
		{Path: "logs_config.additional_endpoints", Kind: "raw"},
		{Path: "logs_config.additional_endpoints.1.api_key", Kind: "enc", Handle: "shared"},
		{Path: "private_action_runner", Kind: "raw"},
		{Path: "private_action_runner.credentials.values.a_first.value", Kind: "enc", Handle: "shared"},
		{Path: "scanning_rules", Kind: "raw"},
		{Path: "scanning_rules.0.proximity_keywords.included_keywords.1", Kind: "enc", Handle: "shared"},
		{Path: "snmp_traps.community_strings", Kind: "raw"},
		{Path: "snmp_traps.community_strings.1", Kind: "enc", Handle: "shared"},
	}, secrets)
	for _, value := range []string{"raw-endpoint-key", "raw-credential", "raw-pattern", "raw-keyword", "raw-community"} {
		assert.NotContains(t, string(cleaned), value)
	}
	for _, value := range []string{"example.com", "visible-id", "visible-license"} {
		assert.Contains(t, string(cleaned), value)
	}

	// Observing declarations must not change the scrubbed YAML or the legacy
	// path metadata collected by #59920, including re-entrant replacements.
	plainScrubber := NewWithDefaults()
	plain, err := plainScrubber.ScrubYamlPreserveStructure(input)
	require.NoError(t, err)
	assert.Equal(t, plain, cleaned)
	assert.Equal(t, plainScrubber.GetStrippedKeys(), scrubber.GetStrippedKeys())
	assert.Contains(t, scrubber.GetStrippedKeys(), "logs_config.additional_endpoints.0.api_key")
	assert.Contains(t, scrubber.GetStrippedKeys(), "scanning_rules")
}

func TestScrubYamlWithSecretsCallIsolation(t *testing.T) {
	scrubber := NewWithDefaults()
	_, err := scrubber.ScrubYaml([]byte("previous:\n  password: raw-previous\n"))
	require.NoError(t, err)
	for _, name := range []string{"first", "second", "third", "fourth"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := []byte(name + ":\n  password: raw-password\n  api_key: ENC[shared]\n")
			_, declarations, err := scrubber.ScrubYamlWithSecrets(input)
			require.NoError(t, err)
			expected := []ConfigSecret{
				{Path: name + ".password", Kind: "raw"},
				{Path: name + ".api_key", Kind: "enc", Handle: "shared"},
			}
			assert.ElementsMatch(t, expected, declarations)
			for _, empty := range []string{"", "# comment only", "hostname: safe-host"} {
				_, next, err := scrubber.ScrubYamlWithSecrets([]byte(empty))
				require.NoError(t, err)
				assert.Empty(t, next)
			}
			cleaned, rejected, err := scrubber.ScrubYamlWithSecrets([]byte("password: secret\n---\ninvalid: ["))
			require.Error(t, err)
			assert.Nil(t, cleaned)
			assert.Nil(t, rejected)
			assert.ElementsMatch(t, expected, declarations, "later calls must not mutate returned declarations")
			assert.Contains(t, scrubber.GetStrippedKeys(), "previous.password", "per-call collection must not clear legacy paths")
		})
	}
}

func TestScrubYamlWithSecretsRespectsReplacerFilter(t *testing.T) {
	scrubber := NewWithDefaults()
	scrubber.SetShouldApply(func(Replacer) bool { return false })
	cleaned, secrets, err := scrubber.ScrubYamlWithSecrets([]byte(`password: raw-password
community_strings: [raw-community, 'ENC[shared]']
`))
	require.NoError(t, err)
	assert.Contains(t, string(cleaned), "raw-password")
	assert.Contains(t, string(cleaned), "raw-community")
	assert.Equal(t, []ConfigSecret{{Path: "community_strings.1", Kind: "enc", Handle: "shared"}}, secrets)
	assert.Empty(t, scrubber.GetStrippedKeys())
}

func TestScrubYamlWithSecretsCollapsedMapping(t *testing.T) {
	input := []byte(`token:
  second: raw-value
  first: ENC[shared]
  nested:
    42:
      password: ENC[shared]
`)
	cleaned, secrets, err := NewWithDefaults().ScrubYamlWithSecrets(input)
	require.NoError(t, err)
	assert.YAMLEq(t, `token: '********'`, string(cleaned))
	assert.ElementsMatch(t, []ConfigSecret{
		{Path: "token", Kind: "raw"},
		{Path: "token.first", Kind: "enc", Handle: "shared"},
		{Path: "token.nested.42.password", Kind: "enc", Handle: "shared"},
	}, secrets)
}

func TestScrubYamlWithSecretsUnchangedStructuredValues(t *testing.T) {
	input := []byte(`logs_config:
  additional_endpoints:
    - host: example.com
      port: 0x1f90
      api_key: ENC[shared]
private_action_runner:
  enabled: true
  credentials:
    values:
      existing: {value: '********'}
      backend: {value: 'ENC[shared]'}
`)
	_, secrets, err := NewWithDefaults().ScrubYamlWithSecrets(input)
	require.NoError(t, err)
	assert.ElementsMatch(t, []ConfigSecret{
		{Path: "logs_config.additional_endpoints.0.api_key", Kind: "enc", Handle: "shared"},
		{Path: "private_action_runner.credentials.values.backend.value", Kind: "enc", Handle: "shared"},
	}, secrets)
}
