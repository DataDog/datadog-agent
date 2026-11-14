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

func TestScrubYamlPreserveStructureSecrets(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		expected    []ConfigSecret
	}{
		{
			"scalars", `api_key: !custom ENC[shared]
binary_reference: !!binary RU5DW2VuY29kZWRd
instances: [{password: raw-secret}, {password: 'ENC[shared]'}, {password: 123456}]
auth_token: |
  raw-secret
password: false
client_secret: ""
app_key: null
already_redacted: {password: '********'}
`, []ConfigSecret{
				{Path: "api_key", Kind: "enc", Handle: "shared"},
				{Path: "binary_reference", Kind: "enc", Handle: "encoded"},
				{Path: "instances.0.password", Kind: "raw"},
				{Path: "instances.1.password", Kind: "enc", Handle: "shared"},
				{Path: "instances.2.password", Kind: "raw"},
				{Path: "auth_token", Kind: "raw"},
			},
		},
		{
			"collapsed containers", `community_strings: [raw-secret, 'ENC[shared]', false, null, '']
token: {second: raw-secret, first: 'ENC[shared]', nested: {42: {password: 'ENC[shared]'}}}
`, []ConfigSecret{
				{Path: "community_strings", Kind: "raw"},
				{Path: "community_strings.1", Kind: "enc", Handle: "shared"},
				{Path: "token", Kind: "raw"},
				{Path: "token.first", Kind: "enc", Handle: "shared"},
				{Path: "token.nested.42.password", Kind: "enc", Handle: "shared"},
			},
		},
		{
			"sanitized keys", `additional_endpoints:
  https://user:url-secret@endpoint.example: [raw-secret, 'ENC[shared]']
`, []ConfigSecret{
				{Path: "additional_endpoints", Kind: "raw"},
				{Path: "additional_endpoints.https://user:********@endpoint.example.1", Kind: "enc", Handle: "shared"},
			},
		},
		{
			"structured replacements", `logs_config:
  additional_endpoints: [{host: example.com, api_key: raw-secret}, {api_key: 'ENC[shared]'}]
private_action_runner:
  credentials:
    values: {z_last: {value: raw-secret}, a_first: {value: 'ENC[shared]'}}
scanning_rules:
  - id: visible-id
    license: visible-license
    pattern: raw-secret
    proximity_keywords: {included_keywords: [raw-secret, 'ENC[shared]']}
`, []ConfigSecret{
				{Path: "logs_config.additional_endpoints", Kind: "raw"},
				{Path: "logs_config.additional_endpoints.1.api_key", Kind: "enc", Handle: "shared"},
				{Path: "private_action_runner", Kind: "raw"},
				{Path: "private_action_runner.credentials.values.a_first.value", Kind: "enc", Handle: "shared"},
				{Path: "scanning_rules", Kind: "raw"},
				{Path: "scanning_rules.0.proximity_keywords.included_keywords.1", Kind: "enc", Handle: "shared"},
			},
		},
		{
			"unchanged structured values", `logs_config:
  additional_endpoints: [{host: example.com, port: 0x1f90, api_key: 'ENC[shared]'}]
private_action_runner:
  credentials:
    values: {existing: {value: '********'}, backend: {value: 'ENC[shared]'}}
`, []ConfigSecret{
				{Path: "logs_config.additional_endpoints.0.api_key", Kind: "enc", Handle: "shared"},
				{Path: "private_action_runner.credentials.values.backend.value", Kind: "enc", Handle: "shared"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scrubber := NewWithDefaults()
			cleaned, secrets, err := scrubber.ScrubYamlPreserveStructure([]byte(tc.input))
			require.NoError(t, err)
			assert.ElementsMatch(t, tc.expected, secrets)
			encoded, err := json.Marshal([]any{secrets, scrubber.GetStrippedKeys()})
			require.NoError(t, err)
			for _, value := range []string{"raw-secret", "url-secret", "123456"} {
				assert.NotContains(t, string(cleaned), value)
				assert.NotContains(t, string(encoded), value)
			}
		})
	}
}

func TestScrubYamlPreserveStructureCallIsolation(t *testing.T) {
	scrubber := NewWithDefaults()
	_, err := scrubber.ScrubYaml([]byte("previous:\n  password: raw-previous\n"))
	require.NoError(t, err)
	for _, name := range []string{"first", "second", "third", "fourth"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := []byte(name + ":\n  password: raw-password\n  api_key: ENC[shared]\n")
			_, declarations, err := scrubber.ScrubYamlPreserveStructure(input)
			require.NoError(t, err)
			expected := []ConfigSecret{
				{Path: name + ".password", Kind: "raw"},
				{Path: name + ".api_key", Kind: "enc", Handle: "shared"},
			}
			assert.ElementsMatch(t, expected, declarations)
			for _, empty := range []string{"", "# comment only", "hostname: safe-host"} {
				_, next, err := scrubber.ScrubYamlPreserveStructure([]byte(empty))
				require.NoError(t, err)
				assert.Empty(t, next)
			}
			assert.ElementsMatch(t, expected, declarations, "later calls must not mutate returned declarations")
			assert.Contains(t, scrubber.GetStrippedKeys(), "previous.password", "per-call collection must not clear legacy paths")
		})
	}
}

func TestScrubYamlPreserveStructureRespectsReplacerFilter(t *testing.T) {
	scrubber := NewWithDefaults()
	scrubber.SetShouldApply(func(Replacer) bool { return false })
	cleaned, secrets, err := scrubber.ScrubYamlPreserveStructure([]byte(`password: raw-password
community_strings: [raw-community, 'ENC[shared]']
`))
	require.NoError(t, err)
	assert.Contains(t, string(cleaned), "raw-password")
	assert.Contains(t, string(cleaned), "raw-community")
	assert.Equal(t, []ConfigSecret{{Path: "community_strings.1", Kind: "enc", Handle: "shared"}}, secrets)
	assert.Empty(t, scrubber.GetStrippedKeys())
}
