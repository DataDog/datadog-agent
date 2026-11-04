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
		{"metrics URL does not select the site", "datadoghq.com", "https://app.datadoghq.eu.:8443/path", "datadoghq.com"},
		{"metrics proxy does not select the site", "datadoghq.eu", "https://proxy.example.com", "datadoghq.eu"},
		{"metrics URL with no site", "", "https://app.datadoghq.eu", "datadoghq.com"},
		{"metrics proxy with no site", "", "https://proxy.example.com", "datadoghq.com"},
		{"normalized site", " US3.DATADOGHQ.COM. ", "", "us3.datadoghq.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("site", tc.site)
			cfg.SetInTest("dd_url", tc.ddURL)
			assert.Equal(t, tc.wantSite, Site(cfg))
			assert.Equal(t, "https://api."+tc.wantSite, APIBaseURL(cfg, Site(cfg)))
		})
	}
}

func TestAPIBaseURLFakeIntake(t *testing.T) {
	t.Setenv(constants.InternalUseDDURLForOPMSEnvVar, "true")
	for _, endpoint := range []string{"http://fakeintake.test:8080", "https://fakeintake.test:8443"} {
		cfg := configmock.New(t)
		cfg.SetInTest("dd_url", endpoint+"/")
		assert.Equal(t, endpoint, APIBaseURL(cfg, "datadoghq.com"))
	}
}
