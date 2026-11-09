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
		{Path: "logs.0.password", Kind: "raw"},
		{Path: "jmx_metrics.0.password", Kind: "enc", Handle: "shared_password"},
		{Path: "snmp_traps.community_strings.0", Kind: "raw"},
		{Path: "snmp_traps.community_strings.1", Kind: "enc", Handle: "community"},
	}, secrets)
	encoded, err := json.Marshal(secrets)
	require.NoError(t, err)
	for _, secret := range []string{"raw-logs-key", "raw-password", "raw-log-password", "raw-community"} {
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

func TestScrubYamlWithSecretsStructuredReplacers(t *testing.T) {
	_, secrets, err := NewWithDefaults().ScrubYamlWithSecrets([]byte(`additional_endpoints:
  https://endpoint.example: [raw-key, 'ENC[endpoint_key]']
`))
	require.NoError(t, err)
	assert.ElementsMatch(t, []ConfigSecret{
		{Path: "additional_endpoints.https://endpoint.example.0", Kind: "raw"},
		{Path: "additional_endpoints.https://endpoint.example.1", Kind: "enc", Handle: "endpoint_key"},
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
		{Path: "additional_endpoints.https://user:********@endpoint.example.0", Kind: "raw"},
		{Path: "additional_endpoints.https://user:********@endpoint.example.1", Kind: "enc", Handle: "endpoint_key"},
	}, secrets)
}
