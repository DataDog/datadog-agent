// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"net/url"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemovePathIfPresent(t *testing.T) {
	for _, tt := range []struct {
		input    string
		expected string
	}{
		{input: "http://foo.com", expected: "http://foo.com"},
		{input: "http://foo.com/", expected: "http://foo.com"},
		{input: "http://foo.com/api/v1", expected: "http://foo.com"},
		{input: "http://foo.com?foo", expected: "http://foo.com"},
		{input: "http://foo.com/api/v1/?foo", expected: "http://foo.com"},
		{input: "http://foo.com/api/v1?foo", expected: "http://foo.com"},
		{input: "http://foo.com:8080", expected: "http://foo.com:8080"},
		{input: "http://foo.com:8080/", expected: "http://foo.com:8080"},
		{input: "http://foo.com:8080/api/v1", expected: "http://foo.com:8080"},
	} {
		u, err := url.Parse(tt.input)
		require.NoError(t, err)

		assert.Equal(t, tt.expected, removePathIfPresent(u))
	}
}

func TestKeysPerDomain(t *testing.T) {
	for _, tt := range []struct {
		input    []Endpoint
		expected map[string][]utils.APIKeys
	}{
		{
			input: []Endpoint{
				{APIKey: "key1", Endpoint: getEndpoint(t, "http://foo.com"), ConfigSettingPath: "path"},
			},
			expected: map[string][]utils.APIKeys{
				"http://foo.com": {utils.NewAPIKeys("path", "key1")},
			},
		},
		{
			input: []Endpoint{
				{APIKey: "key1", Endpoint: getEndpoint(t, "http://foo.com"), ConfigSettingPath: "path1"},
				{APIKey: "key2", Endpoint: getEndpoint(t, "http://foo.com"), ConfigSettingPath: "path2"},
			},
			expected: map[string][]utils.APIKeys{
				"http://foo.com": {
					utils.NewAPIKeys("path1", "key1"),
					utils.NewAPIKeys("path2", "key2"),
				},
			},
		},
		{
			input: []Endpoint{
				{APIKey: "key1", Endpoint: getEndpoint(t, "http://foo.com"), ConfigSettingPath: "path1"},
				{APIKey: "key2", Endpoint: getEndpoint(t, "http://bar.com"), ConfigSettingPath: "path2"},
			},
			expected: map[string][]utils.APIKeys{
				"http://foo.com": {utils.NewAPIKeys("path1", "key1")},
				"http://bar.com": {utils.NewAPIKeys("path2", "key2")},
			},
		},
		{
			input: []Endpoint{
				{APIKey: "key1", Endpoint: getEndpoint(t, "http://foo.com"), ConfigSettingPath: "path1"},
				{APIKey: "key2", Endpoint: getEndpoint(t, "http://bar.com"), ConfigSettingPath: "path2"},
				{APIKey: "key3", Endpoint: getEndpoint(t, "http://foo.com"), ConfigSettingPath: "path3"},
			},
			expected: map[string][]utils.APIKeys{
				"http://foo.com": {
					utils.NewAPIKeys("path1", "key1"),
					utils.NewAPIKeys("path3", "key3"),
				},
				"http://bar.com": {utils.NewAPIKeys("path2", "key2")},
			},
		},
		{
			// A pending delegated-auth marker carries no key: the domain stays alive
			// in the resolver until the resolved key is written back to the config.
			input: []Endpoint{
				{Endpoint: getEndpoint(t, "http://foo.com"), ConfigSettingPath: "path1", HasPendingDelegatedAuth: true},
			},
			expected: map[string][]utils.APIKeys{
				"http://foo.com": {{ConfigSettingPath: "path1", HasPendingDelegatedAuth: true}},
			},
		},
		{
			input: []Endpoint{
				{APIKey: "key1", Endpoint: getEndpoint(t, "http://foo.com"), ConfigSettingPath: "path1"},
				{Endpoint: getEndpoint(t, "http://foo.com"), ConfigSettingPath: "path2", HasPendingDelegatedAuth: true},
			},
			expected: map[string][]utils.APIKeys{
				"http://foo.com": {
					utils.NewAPIKeys("path1", "key1"),
					{ConfigSettingPath: "path2", HasPendingDelegatedAuth: true},
				},
			},
		},
		{
			// A plain empty key (e.g. the main api_key during async delegated-auth
			// startup) must keep its resolver alive: only the pending marker is keyless.
			input: []Endpoint{
				{APIKey: "", Endpoint: getEndpoint(t, "http://foo.com"), ConfigSettingPath: "path1"},
			},
			expected: map[string][]utils.APIKeys{
				"http://foo.com": {utils.NewAPIKeys("path1", "")},
			},
		},
	} {
		assert.Equal(t, tt.expected, KeysPerDomains(tt.input))
	}
}

func TestEndpointDescriptorsConfigName(t *testing.T) {
	eds := EndpointDescriptors([]Endpoint{
		{APIKey: "main", Endpoint: getEndpoint(t, "https://main.com"), ConfigSettingPath: "api_key"},
		{APIKey: "k0", Endpoint: getEndpoint(t, "https://main.com/"), ConfigSettingPath: "path", ConfigKey: "https://main.com/"},
		{APIKey: "k1", Endpoint: getEndpoint(t, "https://slash.com/"), ConfigSettingPath: "path", ConfigKey: "https://slash.com/"},
		{Endpoint: getEndpoint(t, "https://slash.com/"), ConfigSettingPath: "path", ConfigKey: "https://slash.com/", HasPendingDelegatedAuth: true},
		{APIKey: "k3", Endpoint: getEndpoint(t, "https://bare.com"), ConfigSettingPath: "path", ConfigKey: "https://bare.com"},
		{APIKey: "k4", Endpoint: getEndpoint(t, "https://relative.com"), ConfigSettingPath: "path", ConfigKey: "//relative.com"},
		{Endpoint: getEndpoint(t, "https://pending.com/v1"), ConfigSettingPath: "path", ConfigKey: "https://pending.com/v1", HasPendingDelegatedAuth: true},
		{APIKey: "k5", Endpoint: getEndpoint(t, "https://two.com"), ConfigSettingPath: "path", ConfigKey: "https://two.com"},
		{APIKey: "k6", Endpoint: getEndpoint(t, "https://two.com/"), ConfigSettingPath: "path", ConfigKey: "https://two.com/"},
	})

	require.Len(t, eds, 6)
	assert.Equal(t, "https://main.com/", eds["https://main.com"].ConfigName, "the main key doesn't count as a config key")
	assert.Len(t, eds["https://main.com"].APIKeySet, 2)
	assert.Equal(t, "https://slash.com/", eds["https://slash.com"].ConfigName)
	assert.True(t, eds["https://slash.com"].HasPendingDelegatedAuth)
	assert.Empty(t, eds["https://bare.com"].ConfigName, "the domain already is the config key")
	assert.Equal(t, "//relative.com", eds["https://relative.com"].ConfigName)
	assert.Equal(t, "https://pending.com/v1", eds["https://pending.com"].ConfigName)
	assert.True(t, eds["https://pending.com"].HasPendingDelegatedAuth)
	assert.Empty(t, eds["https://two.com"].ConfigName, "two config keys can't share one config name")
	assert.Len(t, eds["https://two.com"].APIKeySet, 2)
}

func getEndpoint(t *testing.T, u string) *url.URL {
	e, err := url.Parse(u)
	assert.NoError(t, err)
	return e
}
