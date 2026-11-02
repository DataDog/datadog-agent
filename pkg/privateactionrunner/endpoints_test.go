// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
		{"dd_url precedence", "datadoghq.com", "https://app.datadoghq.eu.:8443/path", "datadoghq.eu"},
		{"unknown domain", "datadoghq.eu", "https://proxy.example.com", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("site", tc.site)
			cfg.SetInTest("dd_url", tc.ddURL)
			assert.Equal(t, tc.wantSite, Site(cfg))
			endpoint, err := APIBaseURL(cfg, Site(cfg))
			assert.NoError(t, err)
			assert.Equal(t, "https://api."+tc.wantSite, endpoint)
		})
	}
}

func TestAPIBaseURLRCOverride(t *testing.T) {
	for _, fakeIntake := range []string{"false", "true"} {
		t.Run("fakeIntake="+fakeIntake, func(t *testing.T) {
			t.Setenv(constants.InternalUseDDURLForOPMSEnvVar, fakeIntake)
			cfg := configmock.New(t)
			cfg.SetInTest("site", "datadoghq.eu")
			cfg.SetInTest("remote_configuration.no_tls", true)
			cfg.SetInTest("dd_url", "https://app.us3.datadoghq.com")
			for _, endpoint := range []string{
				"https://config.datadoghq.eu.",
				"https://proxy.example.com:8443/prefix",
				"https://proxy.example.com:8443/prefix/",
				"http://localhost:8080",
			} {
				cfg.SetInTest("remote_configuration.rc_dd_url", endpoint)
				actual, err := APIBaseURL(cfg, Site(cfg))
				assert.NoError(t, err)
				assert.Equal(t, endpoint, actual)
				assert.Equal(t, "us3.datadoghq.com", Site(cfg), "routing must not change the runner's logical site")
			}
		})
	}
}

func TestAPIBaseURLRejectsInvalidOverride(t *testing.T) {
	for _, endpoint := range []string{
		"not-a-url", "ftp://proxy.example.com", "https://", "http://proxy.example.com",
		"https://user:secret@proxy.example.com", "https://proxy.example.com?secret=value", "https://proxy.example.com#fragment",
	} {
		cfg := configmock.New(t)
		cfg.SetInTest("remote_configuration.rc_dd_url", endpoint)
		actual, err := APIBaseURL(cfg, "datadoghq.com")
		assert.Error(t, err, endpoint)
		assert.Empty(t, actual)
		if err != nil {
			assert.NotContains(t, err.Error(), "secret")
		}
	}
	cfg := configmock.New(t)
	cfg.SetInTest("remote_configuration.rc_dd_url", "https://proxy.example.com")
	cfg.SetInTest("skip_ssl_validation", true)
	_, err := APIBaseURL(cfg, "datadoghq.com")
	assert.Error(t, err)
	cfg.SetInTest("remote_configuration.no_tls_validation", true)
	_, err = APIBaseURL(cfg, "datadoghq.com")
	assert.NoError(t, err)
}

func TestAPIBaseURLEmptyRCOverride(t *testing.T) {
	t.Setenv(constants.InternalUseDDURLForOPMSEnvVar, "false")
	cfg := configmock.New(t)
	cfg.SetInTest("site", "datadoghq.eu")
	cfg.SetInTest("remote_configuration.rc_dd_url", "")
	endpoint, err := APIBaseURL(cfg, Site(cfg))
	assert.NoError(t, err)
	assert.Equal(t, "https://api.datadoghq.eu", endpoint)
}

func TestAPIBaseURLFakeIntake(t *testing.T) {
	t.Setenv(constants.InternalUseDDURLForOPMSEnvVar, "true")
	for _, endpoint := range []string{"http://fakeintake.test:8080", "https://fakeintake.test:8443"} {
		cfg := configmock.New(t)
		cfg.SetInTest("dd_url", endpoint+"/")
		actual, err := APIBaseURL(cfg, "datadoghq.com")
		assert.NoError(t, err)
		assert.Equal(t, endpoint, actual)
	}
}
