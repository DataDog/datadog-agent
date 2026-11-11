// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
)

func TestEndpoints(t *testing.T) {
	t.Setenv(constants.InternalUseDDURLForOPMSEnvVar, "false")
	for _, tc := range []struct {
		name, site, ddURL, wantSite string
	}{
		{"default", "", "", "datadoghq.com"},
		{"EU", "datadoghq.eu", "", "datadoghq.eu"},
		{"US3", "us3.datadoghq.com", "", "us3.datadoghq.com"},
		{"staging", "datad0g.com", "", "datad0g.com"},
		{"government", "ddog-gov.com", "", "ddog-gov.com"},
		{"metrics URL does not select the site", "datadoghq.com", "https://app.datadoghq.eu.:8443/path", "datadoghq.com"},
		{"metrics proxy does not select the site", "datadoghq.eu", "https://proxy.example.com", "datadoghq.eu"},
		{"metrics URL with no site", "", "https://app.datadoghq.eu", "datadoghq.com"},
		{"metrics proxy with no site", "", "https://proxy.example.com", "datadoghq.com"},
		{"normalized site", " US3.DATADOGHQ.COM. ", "", "us3.datadoghq.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest(Enabled, true)
			cfg.SetInTest("site", tc.site)
			cfg.SetInTest("dd_url", tc.ddURL)
			assert.Equal(t, tc.wantSite, Site(cfg))
			endpoint, err := APIBaseURL(cfg, Site(cfg))
			require.NoError(t, err)
			assert.Equal(t, "https://api."+tc.wantSite, endpoint)
		})
	}
}

func TestAPIBaseURLPAROverride(t *testing.T) {
	for _, fakeIntake := range []string{"false", "true"} {
		t.Run("fakeIntake="+fakeIntake, func(t *testing.T) {
			t.Setenv(constants.InternalUseDDURLForOPMSEnvVar, fakeIntake)
			cfg := configmock.New(t)
			cfg.SetInTest(Enabled, true)
			cfg.SetInTest("site", "datadoghq.eu")
			cfg.SetInTest("dd_url", "https://app.us3.datadoghq.com")
			cfg.SetInTest("remote_configuration.rc_dd_url", "https://rc-proxy.example.com")
			for _, endpoint := range []string{
				"https://api.datadoghq.eu.",
				"https://par-proxy.example.com:8443/prefix",
				"https://par-proxy.example.com:8443/prefix/",
				"http://localhost:8080",
				"https://proxy.example.com:0",
				"https://proxy.example.com:65535",
				"https://[::1]:65535/prefix",
			} {
				cfg.SetInTest(RunnerURL, endpoint)
				actual, err := APIBaseURL(cfg, Site(cfg))
				require.NoError(t, err)
				assert.Equal(t, endpoint, actual)
				assert.False(t, ShouldWarnAboutRCOverride(cfg))
				assert.Equal(t, "datadoghq.eu", Site(cfg), "routing must not change the runner's logical site")
			}
		})
	}
}

func TestCanonicalRCURLDoesNotOverridePAR(t *testing.T) {
	t.Setenv(constants.InternalUseDDURLForOPMSEnvVar, "false")
	for _, site := range []string{"datadoghq.com", "datadoghq.eu", "us3.datadoghq.com", "us5.datadoghq.com", "ap1.datadoghq.com", "ap2.datadoghq.com", "uk1.datadoghq.com", "datad0g.com", "ddog-gov.com"} {
		t.Run(site, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest(Enabled, true)
			cfg.SetInTest("site", site)
			for _, rcURL := range []string{"", "https://config." + site, "https://config." + site + ".", "https://CONFIG." + site + ".:443/"} {
				cfg.SetInTest("remote_configuration.rc_dd_url", rcURL)
				endpoint, err := APIBaseURL(cfg, Site(cfg))
				require.NoError(t, err, rcURL)
				assert.Equal(t, "https://api."+site, endpoint)
				assert.Equal(t, rcURL != "", ShouldWarnAboutRCOverride(cfg), rcURL)
			}
		})
	}
}

func TestRCURLOverrideWarnsWithoutChangingPAR(t *testing.T) {
	t.Setenv(constants.InternalUseDDURLForOPMSEnvVar, "false")
	for _, rcURL := range []string{
		"https://rc-proxy.example.com", "https://config.us3.datadoghq.com", "https://api.datadoghq.eu",
		"http://config.datadoghq.eu", "https://config.datadoghq.eu:8443", "https://config.datadoghq.eu/prefix",
		"https://config.datadoghq.eu..", "https://config.datadoghq.eu?", "https://config.datadoghq.eu#",
		"https://user:secret@config.datadoghq.eu", "not-a-url",
	} {
		t.Run(rcURL, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest(Enabled, true)
			cfg.SetInTest("site", "datadoghq.eu")
			cfg.SetInTest("remote_configuration.rc_dd_url", rcURL)
			cfg.SetInTest(RunnerURL, "")
			endpoint, err := APIBaseURL(cfg, Site(cfg))
			require.NoError(t, err)
			assert.Equal(t, "https://api.datadoghq.eu", endpoint)
			assert.True(t, ShouldWarnAboutRCOverride(cfg))

			cfg.SetInTest(Enabled, false)
			assert.False(t, ShouldWarnAboutRCOverride(cfg))
			endpoint, err = APIBaseURL(cfg, Site(cfg))
			require.NoError(t, err, "disabled PAR must not reject RC configuration")
			assert.Equal(t, "https://api.datadoghq.eu", endpoint)
		})
	}
}

func TestAPIBaseURLRejectsInvalidOverride(t *testing.T) {
	for _, endpoint := range []string{
		"not-a-url", "ftp://proxy.example.com", "https://",
		"https://proxy.example.com:65536", "https://[::1]:99999", "https://proxy.example.com:99999999999999999999",
		"https://user:secret@proxy.example.com", "https://proxy.example.com?secret=value", "https://proxy.example.com#fragment",
	} {
		cfg := configmock.New(t)
		cfg.SetInTest(RunnerURL, endpoint)
		actual, err := APIBaseURL(cfg, "datadoghq.com")
		require.Error(t, err, endpoint)
		assert.Empty(t, actual)
		assert.NotContains(t, err.Error(), "secret")
	}
}

func TestAPIBaseURLFromEnvironment(t *testing.T) {
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_PAR_DD_URL", "https://par-proxy.example.com/prefix")
	cfg := configmock.New(t)
	assert.Equal(t, "https://par-proxy.example.com/prefix", cfg.GetString(RunnerURL))
	endpoint, err := APIBaseURL(cfg, Site(cfg))
	require.NoError(t, err)
	assert.Equal(t, "https://par-proxy.example.com/prefix", endpoint)
}

func TestAPIBaseURLDoesNotAliasOldSetting(t *testing.T) {
	t.Setenv(constants.InternalUseDDURLForOPMSEnvVar, "false")
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_PAR_DD_URL", "")
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_DD_URL", "https://old-env.example.com")
	cfg := configmock.New(t)
	cfg.SetInTest("site", "datadoghq.eu")
	cfg.SetInTest("private_action_runner.dd_url", "https://old-setting.example.com")
	endpoint, err := APIBaseURL(cfg, Site(cfg))
	require.NoError(t, err)
	assert.Equal(t, "https://api.datadoghq.eu", endpoint)
}

func TestAPIBaseURLFakeIntake(t *testing.T) {
	t.Setenv(constants.InternalUseDDURLForOPMSEnvVar, "true")
	for _, endpoint := range []string{"http://fakeintake.test:8080", "https://fakeintake.test:8443"} {
		cfg := configmock.New(t)
		cfg.SetInTest(Enabled, true)
		cfg.SetInTest("dd_url", endpoint+"/")
		cfg.SetInTest("remote_configuration.rc_dd_url", "http://localhost:1234")
		actual, err := APIBaseURL(cfg, "datadoghq.com")
		require.NoError(t, err)
		assert.Equal(t, endpoint, actual)
		assert.True(t, ShouldWarnAboutRCOverride(cfg))
	}
}
