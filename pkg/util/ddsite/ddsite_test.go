// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddsite

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsGovSite(t *testing.T) {
	tests := []struct {
		site string
		want bool
	}{
		{"ddog-gov.com", true},
		{"ddog-gov.mil", true},
		{"xxxx99.ddog-gov.com", true},
		{"xxxx99.ddog-gov.mil", true},
		{"datadoghq.com", false},
		{"datad0g.com", false},
		{"example.com", false},
		{"", false},
		{"notddog-gov.com", false},
	}
	for _, tc := range tests {
		t.Run(tc.site, func(t *testing.T) {
			assert.Equal(t, tc.want, IsGovSite(tc.site))
		})
	}
}

func TestIsGovURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://app.ddog-gov.com", true},
		{"https://app.ddog-gov.mil", true},
		{"https://agent.xxxx99.ddog-gov.com", true},
		{"http://intake.ddog-gov.mil", true},
		{"https://app.datadoghq.com", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			assert.Equal(t, tc.want, IsGovURL(tc.url))
		})
	}
}

func TestIsKnownSite(t *testing.T) {
	tests := []struct {
		site string
		want bool
	}{
		{"datadoghq.com", true},
		{"datadoghq.eu", true},
		{"datad0g.com", true},
		{"datad0g.eu", true},
		{"ddog-gov.com", true},
		{"ddog-gov.mil", true},
		{"us3.datadoghq.com", true},
		{"xxxx99.ddog-gov.mil", true},
		{"example.com", false},
		{"datadoghq.internal", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.site, func(t *testing.T) {
			assert.Equal(t, tc.want, IsKnownSite(tc.site))
		})
	}
}

func TestIsKnownHost(t *testing.T) {
	tests := []struct {
		hostname string
		want     bool
	}{
		{"app.datadoghq.com", true},
		{"app.us3.datadoghq.com", true},
		{"agent.ddog-gov.mil", true},
		{"intake.profile.ddog-gov.com", true},
		{"app.myproxy.com", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.hostname, func(t *testing.T) {
			assert.Equal(t, tc.want, IsKnownHost(tc.hostname))
		})
	}
}

func TestExtractSiteFromHostname(t *testing.T) {
	tests := []struct {
		hostname string
		want     string
	}{
		{"app.us3.datadoghq.com", "us3.datadoghq.com"},
		{"app.datadoghq.com", "datadoghq.com"},
		{"app.datadoghq.eu", "datadoghq.eu"},
		{"intake.profile.datadoghq.com", "datadoghq.com"},
		{"app.ddog-gov.com", "ddog-gov.com"},
		{"app.ddog-gov.mil", "ddog-gov.mil"},
		{"app.xxxx99.ddog-gov.com", "xxxx99.ddog-gov.com"},
		{"custom.agent.us2.datadoghq.com", "us2.datadoghq.com"},
		// trailing dots are stripped
		{"app.datadoghq.com.", "datadoghq.com"},
		// not a known domain
		{"app.myproxy.com", ""},
		{"app.datadoghq.internal", ""},
		{"", ""},
		// label-boundary: "notdatadoghq.com" should NOT match
		{"notdatadoghq.com", ""},
	}
	for _, tc := range tests {
		t.Run(tc.hostname, func(t *testing.T) {
			assert.Equal(t, tc.want, ExtractSiteFromHostname(tc.hostname))
		})
	}
}

func TestExtractSiteFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://intake.profile.us3.datadoghq.com/v1/input", "us3.datadoghq.com"},
		{"https://intake.profile.datadoghq.com/v1/input", "datadoghq.com"},
		{"https://intake.profile.datadoghq.eu/v1/input", "datadoghq.eu"},
		{"https://intake.profile.ddog-gov.com/v1/input", "ddog-gov.com"},
		{"https://intake.profile.ddog-gov.mil/v1/input", "ddog-gov.mil"},
		{"https://app.xxxx99.ddog-gov.mil", "xxxx99.ddog-gov.mil"},
		{"https://custom.example.com/path", ""},
		{"", ""},
		{"://invalid", ""},
	}
	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			assert.Equal(t, tc.want, ExtractSiteFromURL(tc.url))
		})
	}
}

func TestGetAPIDomain(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		// Standard domains
		{"basic datadoghq.com", "https://agent.datadoghq.com", "https://api.datadoghq.com"},
		{"with dc prefix", "https://agent.us3.datadoghq.com", "https://api.us3.datadoghq.com"},
		{"datadoghq.eu", "https://agent.datadoghq.eu", "https://api.datadoghq.eu"},
		{"staging datad0g.com", "https://agent.datad0g.com", "https://api.datad0g.com"},
		{"staging with dc", "https://agent.us1.datad0g.com", "https://api.us1.datad0g.com"},
		// Gov cloud
		{"gov .com", "https://agent.ddog-gov.com", "https://api.ddog-gov.com"},
		{"gov .mil", "https://agent.ddog-gov.mil", "https://api.ddog-gov.mil"},
		{"gov long-named", "https://agent.xxxx99.ddog-gov.com", "https://api.xxxx99.ddog-gov.com"},
		// Trailing dot preserved
		{"trailing dot", "https://agent.ddog-gov.com.", "https://api.ddog-gov.com."},
		// Multiple subdomains
		{"multi subdomain", "https://custom.agent.datadoghq.com", "https://api.datadoghq.com"},
		{"multi subdomain with dc", "https://custom.agent.us2.datadoghq.com", "https://api.us2.datadoghq.com"},
		// No scheme
		{"no scheme", "agent.ddog-gov.com", "https://api.ddog-gov.com"},
		// Unknown domains returned unchanged
		{"unknown domain", "https://custom.example.com", "https://custom.example.com"},
		{"proxy", "https://app.myproxy.com", "https://app.myproxy.com"},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, GetAPIDomain(tc.endpoint))
		})
	}
}
