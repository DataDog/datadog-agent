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
	} {
		assert.Equal(t, tt.expected, KeysPerDomains(tt.input))
	}
}

func getEndpoint(t *testing.T, u string) *url.URL {
	e, err := url.Parse(u)
	assert.NoError(t, err)
	return e
}
