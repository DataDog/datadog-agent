// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package snmp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyConfigParsesTheBackendPayloadVerbatim(t *testing.T) {
	raw := []byte(`{
		"init_config": {"loader": "core", "ping": {"enabled": true}},
		"instances": [
			{"ip_address": "10.0.0.1", "cred_id": "cred-abc"},
			{"ip_address": "10.0.0.2", "cred_id": "cred-def"}
		]
	}`)

	var got keyConfig
	require.NoError(t, json.Unmarshal(raw, &got))

	assert.Equal(t, "core", got.InitConfig.Loader)
	assert.True(t, got.InitConfig.Ping.Enabled)
	assert.Equal(t, []documentInstance{
		{IPAddress: "10.0.0.1", CredID: "cred-abc"},
		{IPAddress: "10.0.0.2", CredID: "cred-def"},
	}, got.Instances)
}

func TestRenderInitConfig(t *testing.T) {
	got, err := renderInitConfig(initConfig{Loader: "core", Ping: pingConfig{Enabled: true}})
	require.NoError(t, err)
	assert.YAMLEq(t, "loader: core\nping:\n  enabled: true\n", string(got))
}

func TestRenderInitConfigOmitsAnAbsentLoader(t *testing.T) {
	got, err := renderInitConfig(initConfig{})
	require.NoError(t, err)
	assert.YAMLEq(t, "ping:\n  enabled: false\n", string(got))
}

func TestRenderInstanceForV2C(t *testing.T) {
	got, err := renderInstance("10.0.0.1", credential{
		ID:              "v2c-public",
		SNMPVersion:     "2c",
		CommunityString: "public",
	})
	require.NoError(t, err)

	assert.YAMLEq(t, "ip_address: 10.0.0.1\nsnmp_version: \"2c\"\ncommunity_string: public\n", string(got))
	assert.NotContains(t, string(got), "port")
	assert.NotContains(t, string(got), "timeout")
	assert.NotContains(t, string(got), "retries")
	assert.NotContains(t, string(got), "authProtocol")
	assert.NotContains(t, string(got), "user")
}

func TestRenderInstanceForV1(t *testing.T) {
	got, err := renderInstance("10.0.0.9", credential{
		ID:              "v1",
		SNMPVersion:     "1",
		CommunityString: "public",
	})
	require.NoError(t, err)
	assert.YAMLEq(t, "ip_address: 10.0.0.9\nsnmp_version: \"1\"\ncommunity_string: public\n", string(got))
}

func TestRenderInstanceForV3(t *testing.T) {
	got, err := renderInstance("10.0.0.2", credential{
		ID:           "v3-full",
		SNMPVersion:  "3",
		User:         "test-user",
		AuthProtocol: "SHA",
		AuthKey:      "test-auth-key",
		PrivProtocol: "AES",
		PrivKey:      "test-priv-key",
		ContextName:  "test-context",
	})
	require.NoError(t, err)

	assert.YAMLEq(t, `
ip_address: 10.0.0.2
snmp_version: "3"
user: test-user
authProtocol: SHA
authKey: test-auth-key
privProtocol: AES
privKey: test-priv-key
context_name: test-context
`, string(got))
	assert.NotContains(t, string(got), "community_string")
}

func TestRenderInstanceKeepsTheVersionAString(t *testing.T) {
	// The snmp check reads snmp_version as a string, so it must stay quoted.
	got, err := renderInstance("10.0.0.3", credential{ID: "v3", SNMPVersion: "3", User: "u"})
	require.NoError(t, err)
	assert.Contains(t, string(got), `snmp_version: "3"`)
}
