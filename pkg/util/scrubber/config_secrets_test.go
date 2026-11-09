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
proxy:
  http: http://user:proxy-value@localhost:1234
init_config:
  password: ENC[shared_password]
instances:
  - password: raw-password
    headers:
      Authorization: Bearer raw-header
      x.custom.header: ENC[custom_header]
  - password: ENC[second_password]
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
		{Path: []any{"api_key"}, Kind: "enc", Handle: "main_key"},
		{Path: []any{"logs_config", "api_key"}, Kind: "raw"},
		{Path: []any{"proxy", "http"}, Kind: "raw"},
		{Path: []any{"init_config", "password"}, Kind: "enc", Handle: "shared_password"},
		{Path: []any{"instances", 0, "password"}, Kind: "raw"},
		{Path: []any{"instances", 0, "headers", "Authorization"}, Kind: "raw"},
		{Path: []any{"instances", 0, "headers", "x.custom.header"}, Kind: "enc", Handle: "custom_header"},
		{Path: []any{"instances", 1, "password"}, Kind: "enc", Handle: "second_password"},
		{Path: []any{"snmp_traps", "community_strings", 0}, Kind: "raw"},
		{Path: []any{"snmp_traps", "community_strings", 1}, Kind: "enc", Handle: "community"},
	}, secrets)
	encoded, err := json.Marshal(secrets)
	require.NoError(t, err)
	for _, secret := range []string{"raw-logs-key", "proxy-value", "raw-password", "raw-header", "raw-community"} {
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

func TestScrubYamlWithSecretsIndependentSnapshots(t *testing.T) {
	scrubber := NewWithDefaults()
	_, secrets, err := scrubber.ScrubYamlWithSecrets([]byte("password: first\napi_key: ENC[key]"))
	require.NoError(t, err)
	require.Len(t, secrets, 2)
	_, secrets, err = scrubber.ScrubYamlWithSecrets([]byte("enabled: true"))
	require.NoError(t, err)
	assert.Empty(t, secrets, "removed secrets must not survive in the next file/snapshot")
}

func TestScrubYamlWithSecretsStructuredReplacers(t *testing.T) {
	_, secrets, err := NewWithDefaults().ScrubYamlWithSecrets([]byte(`additional_endpoints:
  https://endpoint.example: [raw-key, 'ENC[endpoint_key]']
`))
	require.NoError(t, err)
	assert.ElementsMatch(t, []ConfigSecret{
		{Path: []any{"additional_endpoints", "https://endpoint.example", 0}, Kind: "raw"},
		{Path: []any{"additional_endpoints", "https://endpoint.example", 1}, Kind: "enc", Handle: "endpoint_key"},
	}, secrets)
}

func TestScrubYamlWithSecretsDoesNotLeakCredentialsInMapKeys(t *testing.T) {
	cleaned, secrets, err := NewWithDefaults().ScrubYamlWithSecrets([]byte(`additional_endpoints:
  https://user:map-password@endpoint.example: [raw-key, 'ENC[endpoint_key]']
`))
	require.NoError(t, err)
	encoded, err := json.Marshal(secrets)
	require.NoError(t, err)
	for _, value := range []string{"map-password", "raw-key"} {
		assert.NotContains(t, string(cleaned), value)
		assert.NotContains(t, string(encoded), value)
	}
	assert.ElementsMatch(t, []ConfigSecret{
		{Path: []any{"additional_endpoints", "https://user:********@endpoint.example", 0}, Kind: "raw"},
		{Path: []any{"additional_endpoints", "https://user:********@endpoint.example", 1}, Kind: "enc", Handle: "endpoint_key"},
	}, secrets)
}
