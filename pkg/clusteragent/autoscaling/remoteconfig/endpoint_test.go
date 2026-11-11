// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/meta"
)

const testAPIKey = "0123456789abcdef0123456789abcdef"

func newTestConfig(t *testing.T, url, apiKey string) model.Config {
	t.Helper()
	cfg := configmock.New(t)
	if url != "" {
		cfg.SetInTest(URLSetting, url)
	}
	if apiKey != "" {
		cfg.SetInTest(APIKeySetting, apiKey)
	}
	return cfg
}

// TestNotConfiguredByDefault is the case for every customer deployment and every
// cluster we do not explicitly enable: nothing about this package applies and
// autoscaling keeps using the Cluster Agent's default remote config client.
func TestNotConfiguredByDefault(t *testing.T) {
	cfg := configmock.New(t)

	assert.False(t, IsConfigured(cfg))
	for _, setting := range []string{URLSetting, APIKeySetting, SiteSetting, ConfigRootSetting, DirectorRootSetting, KeySetting} {
		assert.Equal(t, "", cfg.GetString(setting), setting)
	}

	// And the status section contributes nothing at all, so the Cluster Agent
	// status output is unchanged.
	provider := NewProvider(cfg, NewEndpointStatus())
	assert.Nil(t, provider.statusInfo())
	stats := map[string]interface{}{}
	require.NoError(t, provider.JSON(false, stats))
	assert.Empty(t, stats)
}

// TestIsConfiguredIgnoresWhitespace guards against a Helm value rendering to a
// blank string, which must read as "not configured" rather than as an invalid
// endpoint that fails startup.
func TestIsConfiguredIgnoresWhitespace(t *testing.T) {
	assert.False(t, IsConfigured(newTestConfig(t, "   ", testAPIKey)))
	assert.True(t, IsConfigured(newTestConfig(t, "https://config.datad0g.com", testAPIKey)))
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		apiKey   string
		wantSite string
	}{
		{
			name:     "staging",
			url:      "https://config.datad0g.com",
			apiKey:   testAPIKey,
			wantSite: "datad0g.com",
		},
		{
			// The datacenter label is dropped: the site is only used to pick the
			// trusted roots, and meta recognises staging only as "datad0g.com".
			// Keeping "us5." here would silently pin the production roots.
			name:     "staging datacenter subdomain",
			url:      "https://config.us5.datad0g.com",
			apiKey:   testAPIKey,
			wantSite: "us5.datad0g.com",
		},
		{
			name:     "production datacenter subdomain",
			url:      "https://config.us3.datadoghq.com",
			apiKey:   testAPIKey,
			wantSite: "us3.datadoghq.com",
		},
		{
			name:     "eu",
			url:      "https://config.datadoghq.eu",
			apiKey:   testAPIKey,
			wantSite: "datadoghq.eu",
		},
		{
			name:     "surrounding whitespace is trimmed",
			url:      "  https://config.datad0g.com  ",
			apiKey:   "  " + testAPIKey + "  ",
			wantSite: "datad0g.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := Load(newTestConfig(t, tt.url, tt.apiKey))
			require.NoError(t, err)
			// Taken verbatim from the URL: the site drives which TUF roots the
			// client and service pin, and TestLoadResolvesTheTrustedRootsThe...
			// covers what each one resolves to.
			assert.Equal(t, tt.wantSite, settings.Site)
			assert.Equal(t, testAPIKey, settings.APIKey)
			assert.NotEmpty(t, settings.URL)
		})
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		apiKey    string
		wantError string
	}{
		{
			name:      "url not set",
			apiKey:    testAPIKey,
			wantError: URLSetting + " is not set",
		},
		{
			name:      "not a datadog site",
			url:       "https://config.evil.example.com",
			apiKey:    testAPIKey,
			wantError: "does not point at a known Datadog site",
		},
		{
			// "notdatadoghq.com" must not be mistaken for "datadoghq.com".
			name:      "lookalike datadog domain",
			url:       "https://config.notdatadoghq.com",
			apiKey:    testAPIKey,
			wantError: "does not point at a known Datadog site",
		},
		{
			// No scheme means no hostname to extract a site from.
			name:      "no scheme",
			url:       "config.datad0g.com",
			apiKey:    testAPIKey,
			wantError: "does not point at a known Datadog site",
		},
		{
			// ExtractSiteFromURL parses, so a malformed URL yields no site.
			name:      "unparseable",
			url:       "https://config.datad0g.com/%zz",
			apiKey:    testAPIKey,
			wantError: "does not point at a known Datadog site",
		},
		{
			name:      "api key missing",
			url:       "https://config.datad0g.com",
			wantError: APIKeySetting + " must be set",
		},
		{
			name:      "api key blank",
			url:       "https://config.datad0g.com",
			apiKey:    "   ",
			wantError: APIKeySetting + " must be set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(newTestConfig(t, tt.url, tt.apiKey))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

// TestLoadTrustSettings covers a target backend that signs with its own keys, so
// the roots embedded for its site do not apply.
func TestLoadTrustSettings(t *testing.T) {
	cfg := newTestConfig(t, "https://config.datad0g.com", testAPIKey)
	cfg.SetInTest(SiteSetting, "datadoghq.com")
	cfg.SetInTest(ConfigRootSetting, "  config-root-metadata  ")
	cfg.SetInTest(DirectorRootSetting, "director-root-metadata")
	cfg.SetInTest(KeySetting, "DDRCM_TESTKEY")

	settings, err := Load(cfg)
	require.NoError(t, err)

	// The explicit site wins over the one derived from the URL.
	assert.Equal(t, "datadoghq.com", settings.Site)
	assert.Equal(t, "config-root-metadata", settings.ConfigRoot)
	assert.Equal(t, "director-root-metadata", settings.DirectorRoot)
	assert.Equal(t, "DDRCM_TESTKEY", settings.Key)
}

// TestLoadSiteOverrideStillRequiresKnownSiteURL keeps the guardrail intact: an
// explicit site must not become a way to point the Cluster Agent at a host that
// is not Datadog at all.
func TestLoadSiteOverrideStillRequiresKnownSiteURL(t *testing.T) {
	cfg := newTestConfig(t, "https://config.evil.example.com", testAPIKey)
	cfg.SetInTest(SiteSetting, "datad0g.com")

	_, err := Load(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not point at a known Datadog site")
}

// TestLoadResolvesTheTrustedRootsTheBackendActuallySigns asserts the invariant
// the site exists for, not the string implementing it: staging endpoints must
// resolve to the staging roots and production ones to the production roots.
func TestLoadResolvesTheTrustedRootsTheBackendActuallySigns(t *testing.T) {
	rootsFor := func(t *testing.T, rawURL string) []byte {
		t.Helper()
		settings, err := Load(newTestConfig(t, rawURL, testAPIKey))
		require.NoError(t, err)
		return meta.RootsDirector(settings.Site, "").Root()
	}

	staging := meta.RootsDirector("datad0g.com", "").Root()
	production := meta.RootsDirector("datadoghq.com", "").Root()
	gov := meta.RootsDirector("ddog-gov.com", "").Root()
	require.NotEqual(t, staging, production, "staging and production roots must differ for this test to mean anything")
	require.NotEqual(t, gov, production, "gov and production roots must differ for this test to mean anything")

	assert.Equal(t, staging, rootsFor(t, "https://config.datad0g.com"), "bare staging host")
	assert.Equal(t, production, rootsFor(t, "https://config.datadoghq.com"), "bare production host")
	assert.Equal(t, production, rootsFor(t, "https://config.us3.datadoghq.com"), "datacenter-qualified production host")
	assert.Equal(t, gov, rootsFor(t, "https://config.ddog-gov.com"), "bare gov host")
	assert.Equal(t, gov, rootsFor(t, "https://config.us1.ddog-gov.com"), "datacenter-qualified gov host")

	// meta matches gov on a ".ddog-gov.com" suffix but staging only on the exact
	// string, so a datacenter-qualified staging host resolves to the production
	// roots and needs DirectorRootSetting. Asserted rather than worked around, so
	// that fixing meta flips this deliberately.
	assert.Equal(t, production, rootsFor(t, "https://config.us5.datad0g.com"), "datacenter-qualified staging host")
}
