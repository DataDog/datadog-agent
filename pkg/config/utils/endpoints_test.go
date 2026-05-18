// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecretBackendWithMultipleEndpoints tests an edge case of `viper.AllSettings()` when a config
// key includes the key delimiter. Affects the config package when both secrets and multiple
// endpoints are configured.
// Refer to https://github.com/DataDog/viper/pull/2 for more details.
func TestSecretBackendWithMultipleEndpoints(t *testing.T) {
	conf := mock.NewFromFile(t, "./tests/datadog_secrets.yaml")

	expectedKeysPerDomain := EndpointDescriptorSet{
		"https://app.datadoghq.com.": EndpointDescriptor{
			BaseURL: "https://app.datadoghq.com.",
			APIKeySet: []APIKeys{
				NewAPIKeys("api_key", "https://app.datadoghq.com.", "someapikey"),
				NewAPIKeys("additional_endpoints", "https://app.datadoghq.com.", "someotherapikey"),
			},
		},
	}
	keysPerDomain, err := GetMultipleEndpoints(conf)
	assert.NoError(t, err)
	assert.Equal(t, expectedKeysPerDomain, keysPerDomain)
}

func TestGetMultipleEndpointsDefault(t *testing.T) {
	datadogYaml := `
api_key: fakeapikey

additional_endpoints:
  "https://app.datadoghq.com.":
  - fakeapikey2
  - fakeapikey3
  "https://foo.datadoghq.com.":
  - someapikey
`

	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := EndpointDescriptorSet{
		"https://foo.datadoghq.com.": EndpointDescriptor{BaseURL: "https://foo.datadoghq.com.", APIKeySet: []APIKeys{NewAPIKeys("additional_endpoints", "https://foo.datadoghq.com.", "someapikey")}},
		"https://app.datadoghq.com.": EndpointDescriptor{
			BaseURL: "https://app.datadoghq.com.",
			APIKeySet: []APIKeys{
				NewAPIKeys("api_key", "https://app.datadoghq.com.", "fakeapikey"),
				NewAPIKeys("additional_endpoints", "https://app.datadoghq.com.", "fakeapikey2", "fakeapikey3"),
			},
		},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestGetMultipleEndpointsDDURL(t *testing.T) {
	datadogYaml := `
dd_url: "https://app.datadoghq.com"
api_key: fakeapikey

additional_endpoints:
  "https://app.datadoghq.com":
  - fakeapikey2
  - fakeapikey3
  "https://foo.datadoghq.com":
  - someapikey
`

	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := EndpointDescriptorSet{
		"https://foo.datadoghq.com": EndpointDescriptor{BaseURL: "https://foo.datadoghq.com", APIKeySet: []APIKeys{NewAPIKeys("additional_endpoints", "https://foo.datadoghq.com", "someapikey")}},
		"https://app.datadoghq.com": EndpointDescriptor{
			BaseURL: "https://app.datadoghq.com",
			APIKeySet: []APIKeys{
				NewAPIKeys("api_key", "https://app.datadoghq.com", "fakeapikey"),
				NewAPIKeys("additional_endpoints", "https://app.datadoghq.com", "fakeapikey2", "fakeapikey3"),
			},
		},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestGetMultipleEndpointsEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")
	t.Setenv("DD_ADDITIONAL_ENDPOINTS", "{\"https://foo.datadoghq.com.\": [\"someapikey\"]}")

	testConfig := mock.New(t)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := EndpointDescriptorSet{
		"https://foo.datadoghq.com.": EndpointDescriptor{BaseURL: "https://foo.datadoghq.com.", APIKeySet: []APIKeys{NewAPIKeys("additional_endpoints", "https://foo.datadoghq.com.", "someapikey")}},
		"https://app.datadoghq.com.": EndpointDescriptor{BaseURL: "https://app.datadoghq.com.", APIKeySet: []APIKeys{NewAPIKeys("api_key", "https://app.datadoghq.com.", "fakeapikey")}},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestGetMultipleEndpointsSite(t *testing.T) {
	datadogYaml := `
site: datadoghq.eu
api_key: fakeapikey

additional_endpoints:
  "https://app.datadoghq.com.":
  - fakeapikey2
  - fakeapikey3
  "https://foo.datadoghq.com.":
  - someapikey
`

	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := EndpointDescriptorSet{
		"https://app.datadoghq.eu.":  EndpointDescriptor{BaseURL: "https://app.datadoghq.eu.", APIKeySet: []APIKeys{NewAPIKeys("api_key", "https://app.datadoghq.eu.", "fakeapikey")}},
		"https://foo.datadoghq.com.": EndpointDescriptor{BaseURL: "https://foo.datadoghq.com.", APIKeySet: []APIKeys{NewAPIKeys("additional_endpoints", "https://foo.datadoghq.com.", "someapikey")}},
		"https://app.datadoghq.com.": EndpointDescriptor{BaseURL: "https://app.datadoghq.com.", APIKeySet: []APIKeys{NewAPIKeys("additional_endpoints", "https://app.datadoghq.com.", "fakeapikey2", "fakeapikey3")}},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestGetMultipleEndpointsWithNoAdditionalEndpoints(t *testing.T) {
	datadogYaml := `
dd_url: "https://app.datadoghq.com"
api_key: fakeapikey
`

	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := EndpointDescriptorSet{
		"https://app.datadoghq.com": EndpointDescriptor{BaseURL: "https://app.datadoghq.com", APIKeySet: []APIKeys{NewAPIKeys("api_key", "https://app.datadoghq.com", "fakeapikey")}},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestGetMultipleEndpointseIgnoresDomainWithoutApiKey(t *testing.T) {
	datadogYaml := `
dd_url: "https://app.datadoghq.com"
api_key: fakeapikey

additional_endpoints:
  "https://app.datadoghq.com":
  - fakeapikey2
  "https://foo.datadoghq.com":
  - someapikey
  "https://bar.datadoghq.com":
  - ""
`

	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := EndpointDescriptorSet{
		"https://app.datadoghq.com": EndpointDescriptor{
			BaseURL: "https://app.datadoghq.com",
			APIKeySet: []APIKeys{
				NewAPIKeys("api_key", "https://app.datadoghq.com", "fakeapikey"),
				NewAPIKeys("additional_endpoints", "https://app.datadoghq.com", "fakeapikey2"),
			},
		},
		"https://foo.datadoghq.com": EndpointDescriptor{BaseURL: "https://foo.datadoghq.com", APIKeySet: []APIKeys{NewAPIKeys("additional_endpoints", "https://foo.datadoghq.com", "someapikey")}},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

func TestGetMultipleEndpointsApiKeyDeduping(t *testing.T) {
	datadogYaml := `
dd_url: "https://app.datadoghq.com"
api_key: fakeapikey

additional_endpoints:
  "https://app.datadoghq.com":
  - fakeapikey2
  - fakeapikey
  "https://foo.datadoghq.com":
  - someapikey
  - someotherapikey
  - someapikey
`

	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)

	expectedMultipleEndpoints := EndpointDescriptorSet{
		// "fakeapikey" appears in both api_key and additional_endpoints; both sets
		// keep their copy so each config path can track independent key rotations.
		// Cross-set dedup happens later in the resolver via DedupAPIKeys.
		"https://app.datadoghq.com": EndpointDescriptor{
			BaseURL: "https://app.datadoghq.com",
			APIKeySet: []APIKeys{
				NewAPIKeys("api_key", "https://app.datadoghq.com", "fakeapikey"),
				NewAPIKeys("additional_endpoints", "https://app.datadoghq.com", "fakeapikey2", "fakeapikey"),
			},
		},
		// "someapikey" appears twice in config — collapsed to one entry at construction.
		"https://foo.datadoghq.com": EndpointDescriptor{BaseURL: "https://foo.datadoghq.com", APIKeySet: []APIKeys{
			NewAPIKeys("additional_endpoints", "https://foo.datadoghq.com", "someapikey", "someotherapikey"),
		}},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
}

// TestAdditionalEndpointMixedPlainAndSecretKeys verifies that a single
// additional endpoint configured with one plain key and one ENC[handle] key
// produces both entries side by side: each carries the resolved key material
// in Key, the plain entry gets an idx_-prefixed name keyed on its list
// position, and the ENC entry gets an enc_-prefixed name keyed on its handle.
func TestAdditionalEndpointMixedPlainAndSecretKeys(t *testing.T) {
	const endpoint = "https://app.datadoghq.com"

	testConfig := mock.NewFromYAML(t, `
api_key: main_key
additional_endpoints:
  "https://app.datadoghq.com":
  - plain_extra_key
  - ENC[secret_handle]
`)
	// Secret resolution writes the resolved value for the ENC entry back to the
	// SourceSecret layer; the plain entry is unchanged.
	testConfig.Set("additional_endpoints", map[string]interface{}{
		endpoint: []interface{}{"plain_extra_key", "resolved_secret_value"},
	}, pkgconfigmodel.SourceSecret)

	endpoints, err := GetMultipleEndpoints(testConfig)
	require.NoError(t, err)

	descriptor, ok := endpoints[endpoint]
	require.True(t, ok, "endpoint %q must be present in the result", endpoint)

	// One APIKeys entry for additional_endpoints (the api_key set lives under
	// the same endpoint and is a separate APIKeys entry).
	var additional APIKeys
	for _, set := range descriptor.APIKeySet {
		if set.ConfigSettingPath == "additional_endpoints" {
			additional = set
			break
		}
	}
	require.Len(t, additional.Keys, 2, "additional endpoint must keep both keys")

	assert.Equal(t, "plain_extra_key", additional.Keys[0].Key)
	assert.Equal(t, "idx_https://app.datadoghq.com_additional_endpoints_0", additional.Keys[0].Name)

	assert.Equal(t, "resolved_secret_value", additional.Keys[1].Key)
	assert.Equal(t, "enc_https://app.datadoghq.com_additional_endpoints_secret_handle", additional.Keys[1].Name)
}

// TestMainAPIKeyEncBackedName verifies that an ENC-backed api_key gets an
// enc_-prefixed stable name from GetMultipleEndpoints, matching the behaviour
// already in place for additional_endpoints. Without this, a user who swaps
// which ENC handle is api_key vs. an additional endpoint between agent restarts
// would produce transactions that resolve to idx_-based names on both sides,
// making the two keys indistinguishable in the retry queue.
func TestMainAPIKeyEncBackedName(t *testing.T) {
	ddURL := "https://app.datadoghq.com"

	testConfig := mock.NewFromYAML(t, `
api_key: ENC[main_key_handle]
dd_url: https://app.datadoghq.com
`)
	testConfig.Set("api_key", "resolved_main_key", pkgconfigmodel.SourceSecret)

	endpoints, err := GetMultipleEndpoints(testConfig)
	require.NoError(t, err)

	descriptor, ok := endpoints[ddURL]
	require.True(t, ok)

	var primary APIKeys
	for _, set := range descriptor.APIKeySet {
		if set.ConfigSettingPath == "api_key" {
			primary = set
			break
		}
	}
	require.Len(t, primary.Keys, 1)
	assert.Equal(t, "resolved_main_key", primary.Keys[0].Key)
	assert.Equal(t, "enc_https://app.datadoghq.com_api_key_main_key_handle", primary.Keys[0].Name)
}

// TestAdditionalEndpointAPIKeyNamesTrimAndDedup verifies that empty keys are
// dropped and duplicate key values within one endpoint are collapsed at
// construction. The surviving entry keeps the first occurrence's name, and
// later entries' positions are reflected in their names (positions 0 and 4
// here, with the repeat at position 3 dropped).
func TestAdditionalEndpointAPIKeyNamesTrimAndDedup(t *testing.T) {
	endpoints := map[string][]string{
		"https://app.datadoghq.com": {"key1", "", "  ", "key1", "key2"},
	}
	result := MakeNamedEndpoints(endpoints, nil, "additional_endpoints")

	require.Len(t, result["https://app.datadoghq.com"], 1)
	keys := result["https://app.datadoghq.com"][0].Keys
	require.Len(t, keys, 2, "empty/whitespace keys must be trimmed and duplicate values collapsed")

	assert.Equal(t, "key1", keys[0].Key)
	assert.Equal(t, "idx_https://app.datadoghq.com_additional_endpoints_0", keys[0].Name)

	assert.Equal(t, "key2", keys[1].Key)
	assert.Equal(t, "idx_https://app.datadoghq.com_additional_endpoints_4", keys[1].Name)
}

// TestCanonicalEndpoint verifies superficial spelling differences collapse to
// the same identifier.
func TestCanonicalEndpoint(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://app.datadoghq.com", "https://app.datadoghq.com"},
		{"app.datadoghq.com", "https://app.datadoghq.com"},
		{"https://app.datadoghq.com/", "https://app.datadoghq.com"},
		{"HTTPS://APP.DATADOGHQ.COM/", "https://app.datadoghq.com"},
		{"  https://app.datadoghq.com  ", "https://app.datadoghq.com"},
		{"https://app.datadoghq.com/path/", "https://app.datadoghq.com/path"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, canonicalEndpoint(tc.in), "input=%q", tc.in)
	}

	// Distinct endpoints stay distinct.
	assert.NotEqual(t, canonicalEndpoint("https://app.datadoghq.com"), canonicalEndpoint("https://app.datadoghq.eu"))
	assert.NotEqual(t, canonicalEndpoint("https://app.datadoghq.com"), canonicalEndpoint("https://app.datadoghq.com/path"))

	// Edge cases: empty input, whitespace-only, and unparseable strings fall
	// back gracefully.
	assert.Equal(t, "", canonicalEndpoint(""))
	assert.Equal(t, "", canonicalEndpoint("   "))
	// A bare non-URL string parses without a host; we return it verbatim
	// (after trimming) rather than mangling it.
	assert.Equal(t, "https://not a url", canonicalEndpoint("https://not a url"))
}

// TestAdditionalEndpointsWithNamesFallsBackToConfig verifies that an
// unrecognized value (e.g. nil from an unset OnUpdate path) makes the helper
// fetch the current setting from the config Reader.
func TestAdditionalEndpointsWithNamesFallsBackToConfig(t *testing.T) {
	testConfig := mock.NewFromYAML(t, `
additional_endpoints:
  "https://app.datadoghq.com":
  - the_key
`)

	got := AdditionalEndpointsWithNames(testConfig, "additional_endpoints", nil)

	require.Len(t, got["https://app.datadoghq.com"], 1)
	assert.Equal(t, "the_key", got["https://app.datadoghq.com"][0]["key"])
}

// TestGetStringMapStringSliceRejectsBadShape verifies that values with a
// non-[]interface{} inner type and inner non-string elements are dropped
// instead of producing partial results.
func TestGetStringMapStringSliceRejectsBadShape(t *testing.T) {
	// Inner slice contains a non-string element: that endpoint is dropped.
	mixed := map[string]interface{}{
		"https://bad.example.com":  []interface{}{"k1", 42},
		"https://good.example.com": []interface{}{"k2"},
	}
	assert.Equal(t, map[string][]string{
		"https://good.example.com": {"k2"},
	}, getStringMapStringSlice(mixed))

	// Value is not a map at all: returns nil.
	assert.Nil(t, getStringMapStringSlice("not a map"))
	assert.Nil(t, getStringMapStringSlice(nil))
}

func TestAdditionalEndpointsWithNamesShape(t *testing.T) {
	datadogYaml := `
additional_endpoints:
  "https://app.datadoghq.com":
  - ENC[api_key_handle]
`
	testConfig := mock.NewFromYAML(t, datadogYaml)
	testConfig.Set("additional_endpoints", map[string]interface{}{
		"https://app.datadoghq.com": []interface{}{"resolved_api_key"},
	}, pkgconfigmodel.SourceSecret)

	got := AdditionalEndpointsWithNames(
		testConfig,
		"additional_endpoints",
		testConfig.Get("additional_endpoints"),
	)

	assert.Equal(t, map[string][]map[string]string{
		"https://app.datadoghq.com": {
			{
				"name": "enc_https://app.datadoghq.com_additional_endpoints_api_key_handle",
				"key":  "resolved_api_key",
			},
		},
	}, got)
}

func TestSiteEnvVar(t *testing.T) {
	testCases := []struct {
		convertSiteFQDNEnabled bool
		siteURL                string
		expectedSiteURL        string
		prefix                 string
		expectedURLWithPrefix  string
	}{
		{true, "datadoghq.eu", "https://app.datadoghq.eu.", "https://external-agent.", "https://external-agent.datadoghq.eu."},
		{false, "datadoghq.eu", "https://app.datadoghq.eu", "https://external-agent.", "https://external-agent.datadoghq.eu"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("convertSiteFQDNEnabled=%t", tc.convertSiteFQDNEnabled), func(t *testing.T) {
			t.Setenv("DD_API_KEY", "fakeapikey")
			t.Setenv("DD_SITE", tc.siteURL)

			testConfig := mock.New(t)
			testConfig.Set("convert_dd_site_fqdn.enabled", tc.convertSiteFQDNEnabled, pkgconfigmodel.SourceAgentRuntime)

			multipleEndpoints, err := GetMultipleEndpoints(testConfig)
			externalAgentURL := GetMainEndpoint(testConfig, tc.prefix, "external_config.external_agent_dd_url")

			expectedMultipleEndpoints := EndpointDescriptorSet{
				tc.expectedSiteURL: EndpointDescriptor{BaseURL: tc.expectedSiteURL, APIKeySet: []APIKeys{NewAPIKeys("api_key", tc.expectedSiteURL, "fakeapikey")}},
			}

			assert.NoError(t, err)
			assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
			assert.Equal(t, tc.expectedURLWithPrefix, externalAgentURL)
		})
	}

}

func TestDefaultSite(t *testing.T) {
	datadogYaml := `
api_key: fakeapikey
`
	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := EndpointDescriptorSet{
		"https://app.datadoghq.com.": EndpointDescriptor{BaseURL: "https://app.datadoghq.com.", APIKeySet: []APIKeys{NewAPIKeys("api_key", "https://app.datadoghq.com.", "fakeapikey")}},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://external-agent.datadoghq.com.", externalAgentURL)
}

func TestSite(t *testing.T) {
	testCases := []struct {
		yamlConfig     string
		externalPrefix string
		externalConfig string
		expectedSite   string
		expectedURL    string
	}{
		{
			yamlConfig: `
site: datadoghq.eu
api_key: fakeapikey
convert_dd_site_fqdn.enabled: true
`,
			externalPrefix: "https://external-agent.",
			externalConfig: "external_config.external_agent_dd_url",
			expectedSite:   "https://app.datadoghq.eu.",
			expectedURL:    "https://external-agent.datadoghq.eu.",
		},
		{
			yamlConfig: `
site: datadoghq.eu
api_key: fakeapikey
convert_dd_site_fqdn.enabled: false
`,
			externalPrefix: "https://external-agent.",
			externalConfig: "external_config.external_agent_dd_url",
			expectedSite:   "https://app.datadoghq.eu",
			expectedURL:    "https://external-agent.datadoghq.eu",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.expectedSite, func(t *testing.T) {
			testConfig := mock.NewFromYAML(t, tc.yamlConfig)

			multipleEndpoints, err := GetMultipleEndpoints(testConfig)
			externalAgentURL := GetMainEndpoint(testConfig, tc.externalPrefix, tc.externalConfig)

			expectedMultipleEndpoints := EndpointDescriptorSet{
				tc.expectedSite: EndpointDescriptor{BaseURL: tc.expectedSite, APIKeySet: []APIKeys{NewAPIKeys("api_key", tc.expectedSite, "fakeapikey")}},
			}

			assert.NoError(t, err)
			assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
			assert.Equal(t, tc.expectedURL, externalAgentURL)
		})
	}
}

func TestDDURLEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")
	t.Setenv("DD_URL", "https://app.datadoghq.eu")
	t.Setenv("DD_EXTERNAL_CONFIG_EXTERNAL_AGENT_DD_URL", "https://custom.external-agent.datadoghq.com")
	testConfig := mock.New(t)
	testConfig.BindEnv("external_config.external_agent_dd_url") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	testConfig.BuildSchema()

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := EndpointDescriptorSet{
		"https://app.datadoghq.eu": EndpointDescriptor{BaseURL: "https://app.datadoghq.eu", APIKeySet: []APIKeys{NewAPIKeys("api_key", "https://app.datadoghq.eu", "fakeapikey")}},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://custom.external-agent.datadoghq.com", externalAgentURL)
}

func TestDDDDURLEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")
	t.Setenv("DD_DD_URL", "https://app.datadoghq.eu")
	t.Setenv("DD_EXTERNAL_CONFIG_EXTERNAL_AGENT_DD_URL", "https://custom.external-agent.datadoghq.com")
	testConfig := mock.New(t)
	testConfig.BindEnv("external_config.external_agent_dd_url") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	testConfig.BuildSchema()

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := EndpointDescriptorSet{
		"https://app.datadoghq.eu": EndpointDescriptor{BaseURL: "https://app.datadoghq.eu", APIKeySet: []APIKeys{NewAPIKeys("api_key", "https://app.datadoghq.eu", "fakeapikey")}},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://custom.external-agent.datadoghq.com", externalAgentURL)
}

func TestDDURLAndDDDDURLEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "fakeapikey")

	// If DD_DD_URL and DD_URL are set, the value of DD_DD_URL is used
	t.Setenv("DD_DD_URL", "https://app.datadoghq.dd_dd_url.eu")
	t.Setenv("DD_URL", "https://app.datadoghq.dd_url.eu")

	t.Setenv("DD_EXTERNAL_CONFIG_EXTERNAL_AGENT_DD_URL", "https://custom.external-agent.datadoghq.com")
	testConfig := mock.New(t)
	testConfig.BindEnv("external_config.external_agent_dd_url") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	testConfig.BuildSchema()

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := EndpointDescriptorSet{
		"https://app.datadoghq.dd_dd_url.eu": EndpointDescriptor{BaseURL: "https://app.datadoghq.dd_dd_url.eu", APIKeySet: []APIKeys{NewAPIKeys("api_key", "https://app.datadoghq.dd_dd_url.eu", "fakeapikey")}},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://custom.external-agent.datadoghq.com", externalAgentURL)
}

func TestDDURLOverridesSite(t *testing.T) {
	datadogYaml := `
site: datadoghq.eu
dd_url: "https://app.datadoghq.com"
api_key: fakeapikey

external_config:
  external_agent_dd_url: "https://external-agent.datadoghq.com"
`
	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := EndpointDescriptorSet{
		"https://app.datadoghq.com": EndpointDescriptor{BaseURL: "https://app.datadoghq.com", APIKeySet: []APIKeys{NewAPIKeys("api_key", "https://app.datadoghq.com", "fakeapikey")}},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://external-agent.datadoghq.com", externalAgentURL)
}

func TestDDURLNoSite(t *testing.T) {
	datadogYaml := `
dd_url: "https://app.datadoghq.eu"
api_key: fakeapikey

external_config:
  external_agent_dd_url: "https://custom.external-agent.datadoghq.eu"
`
	testConfig := mock.NewFromYAML(t, datadogYaml)

	multipleEndpoints, err := GetMultipleEndpoints(testConfig)
	externalAgentURL := GetMainEndpoint(testConfig, "https://external-agent.", "external_config.external_agent_dd_url")

	expectedMultipleEndpoints := EndpointDescriptorSet{
		"https://app.datadoghq.eu": EndpointDescriptor{BaseURL: "https://app.datadoghq.eu", APIKeySet: []APIKeys{NewAPIKeys("api_key", "https://app.datadoghq.eu", "fakeapikey")}},
	}

	assert.NoError(t, err)
	assert.EqualValues(t, expectedMultipleEndpoints, multipleEndpoints)
	assert.Equal(t, "https://custom.external-agent.datadoghq.eu", externalAgentURL)
}

func TestExtractSiteFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		// Standard sites
		{"https://intake.profile.datadoghq.com/v1/input", "datadoghq.com"},
		{"https://intake.profile.datadoghq.eu/v1/input", "datadoghq.eu"},
		{"https://intake.profile.ddog-gov.com/v1/input", "ddog-gov.com"},

		// Datacenter subdomains
		{"https://intake.profile.us3.datadoghq.com/v1/input", "us3.datadoghq.com"},
		{"https://intake.profile.us5.datadoghq.com/v1/input", "us5.datadoghq.com"},
		{"https://intake.profile.ap1.datadoghq.com/v1/input", "ap1.datadoghq.com"},
		{"https://intake.profile.eu1.datadoghq.eu/v1/input", "eu1.datadoghq.eu"},

		// Staging/alternative domains
		{"https://intake.profile.datad0g.com/v1/input", "datad0g.com"},
		{"https://intake.profile.us3.datad0g.com/v1/input", "us3.datad0g.com"},

		// Trailing dots (FQDN)
		{"https://intake.profile.datadoghq.com./v1/input", "datadoghq.com"},
		{"https://intake.profile.us3.datadoghq.com./v1/input", "us3.datadoghq.com"},

		// Custom service prefixes
		{"https://ophzngaa-intake.profile.datadoghq.com/api/v2/profile", "datadoghq.com"},
		{"https://sourcemap-intake.us3.datadoghq.com/v1/input", "us3.datadoghq.com"},

		// Bare site as URL
		{"https://datadoghq.com", "datadoghq.com"},
		{"https://us3.datadoghq.com", "us3.datadoghq.com"},

		// Non-Datadog domains
		{"https://example.com/foo", ""},
		{"https://myproxy.internal/intake", ""},
		{"https://notdatadoghq.com/v1/input", ""},
		{"https://notdatad0g.eu/v1/input", ""},

		// Case-insensitive hostnames
		{"https://INTAKE.PROFILE.US3.DATADOGHQ.COM/v1/input", "us3.datadoghq.com"},
		{"https://intake.profile.Datadoghq.COM/v1/input", "datadoghq.com"},

		// Invalid/empty
		{"", ""},
		{"not-a-url", ""},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			assert.Equal(t, tc.expected, ExtractSiteFromURL(tc.url))
		})
	}
}

func TestAddAgentVersionToDomain(t *testing.T) {
	appVersionPrefix := getDomainPrefix("app")
	flareVersionPrefix := getDomainPrefix("flare")

	versionURLTests := []struct {
		url                 string
		expectedURL         string
		shouldAppendVersion bool
	}{
		{ // US
			"https://app.datadoghq.com",
			".datadoghq.com",
			true,
		},
		{ // EU
			"https://app.datadoghq.eu",
			".datadoghq.eu",
			true,
		},
		{ // Gov
			"https://app.ddog-gov.com",
			".ddog-gov.com",
			true,
		},
		{ // Gov long-named
			"https://app.xxxx99.ddog-gov.com",
			".xxxx99.ddog-gov.com",
			true,
		},
		{ // Additional site
			"https://app.us2.datadoghq.com",
			".us2.datadoghq.com",
			true,
		},
		{ // Arbitrary site
			"https://app.xx9.datadoghq.com",
			".xx9.datadoghq.com",
			true,
		},
		{ // Arbitrary long-named site
			"https://app.xxxx99.datadoghq.com",
			".xxxx99.datadoghq.com",
			true,
		},
		{ // Custom DD URL: leave unchanged
			"https://custom.datadoghq.com",
			"custom.datadoghq.com",
			false,
		},
		{ // Custom DD URL with 'agent' subdomain: leave unchanged
			"https://custom.agent.datadoghq.com",
			"custom.agent.datadoghq.com",
			false,
		},
		{ // Custom DD URL: unclear if anyone is actually using such a URL, but for now leave unchanged
			"https://app.custom.datadoghq.com",
			"app.custom.datadoghq.com",
			false,
		},
		{ // Custom top-level domain: unclear if anyone is actually using this, but for now leave unchanged
			"https://app.datadoghq.internal",
			"app.datadoghq.internal",
			false,
		},
		{ // DD URL set to proxy, leave unchanged
			"https://app.myproxy.com",
			"app.myproxy.com",
			false,
		},
		{ // MRF
			"https://app.mrf.datadoghq.com",
			".mrf.datadoghq.com",
			true,
		},
		{ // Trailing dot
			"https://app.datadoghq.com.",
			".datadoghq.com.",
			true,
		},
	}

	for _, testCase := range versionURLTests {
		appURL, err := AddAgentVersionToDomain(testCase.url, "app")
		require.NoError(t, err)
		flareURL, err := AddAgentVersionToDomain(testCase.url, "flare")
		require.NoError(t, err)

		if testCase.shouldAppendVersion {
			assert.Equal(t, "https://"+appVersionPrefix+testCase.expectedURL, appURL)
			assert.Equal(t, "https://"+flareVersionPrefix+testCase.expectedURL, flareURL)
		} else {
			assert.Equal(t, "https://"+testCase.expectedURL, appURL)
			assert.Equal(t, "https://"+testCase.expectedURL, flareURL)
		}
	}
}
