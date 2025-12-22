// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package connfilter

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConnFilter(t *testing.T) {
	type expectedMatch struct {
		domain      string
		ip          string
		shouldMatch bool
	}
	tests := []struct {
		name                      string
		config                    string
		ddSite                    string
		monitorIPWithoutDomain    bool
		expectedMatches           []expectedMatch
		expectedErr               string
		expectedCustomFilterCount int
	}{
		{
			name: "type exclude",
			config: `
filters:
  - match_domain: '*.google.com'
    type: exclude
`,
			expectedMatches: []expectedMatch{
				{domain: "zoom.us", shouldMatch: true},
				{domain: "dns.google.com", shouldMatch: false},
				{domain: "abc.google.com", shouldMatch: false},
			},
			expectedCustomFilterCount: 1,
		},
		{
			name: "type include",
			config: `
filters:
  - match_domain: '*.google.com'
    type: exclude
  - match_domain: 'abc.google.com'  # precedence matter, for this case include should be after the exclude to take precedence
    type: include
`,
			expectedMatches: []expectedMatch{
				{domain: "zoom.us", shouldMatch: true},
				{domain: "dns.google.com", shouldMatch: false},
				{domain: "123.google.com", shouldMatch: false},
				{domain: "abc.google.com", shouldMatch: true},
			},
			expectedCustomFilterCount: 2,
		},
		{
			name: "domain strategy wildcard",
			config: `
filters:
  - match_domain: '*.google.com'
    match_domain_strategy: 'wildcard'
    type: exclude
`,
			expectedMatches: []expectedMatch{
				{domain: "zoom.us", shouldMatch: true},
				{domain: "abc.googlexcom", shouldMatch: true},
				{domain: "dns.google.com", shouldMatch: false},
				{domain: "abc.google.com", shouldMatch: false},
			},
			expectedCustomFilterCount: 1,
		},
		{
			name: "domain strategy regex",
			config: `
filters:
  - match_domain: '.*\.google\.com'
    match_domain_strategy: 'regex'
    type: exclude
`,
			expectedMatches: []expectedMatch{
				{domain: "zoom.us", shouldMatch: true},
				{domain: "dns.google.com", shouldMatch: false},
				{domain: "abc.google.com", shouldMatch: false},
			},
			expectedCustomFilterCount: 1,
		},
		{
			name: "invalid strategy",
			config: `
filters:
  - match_domain: '.*\.google\.com'
    match_domain_strategy: 'invalid'
    type: exclude
`,
			expectedErr:               "invalid match domain strategy: invalid",
			expectedCustomFilterCount: 0,
		},
		{
			name: "single ip exclude IPv4",
			config: `
filters:
  - match_ip: 10.10.10.10
    type: exclude
`,
			expectedMatches: []expectedMatch{
				{domain: "abc", ip: "10.10.10.10", shouldMatch: false},
				{domain: "abc", ip: "10.10.10.9", shouldMatch: true},
				{domain: "abc", ip: "10.10.10.11", shouldMatch: true},
			},
			expectedCustomFilterCount: 1,
		},
		{
			name: "single ip exclude IPv6",
			config: `
filters:
  - match_ip: 2001:4860:4860::8888
    type: exclude
`,
			expectedMatches: []expectedMatch{
				{domain: "abc", ip: "2001:4860:4860::8888", shouldMatch: false},
				{domain: "abc", ip: "2001:4860:4860::8844", shouldMatch: true},
			},
			expectedCustomFilterCount: 1,
		},
		{
			name: "single ip exclude, then include",
			config: `
filters:
  - match_ip: 10.10.10.10
    type: exclude
  - match_ip: 10.10.10.10
    type: include
`,
			expectedMatches: []expectedMatch{
				{domain: "abc", ip: "10.10.10.10", shouldMatch: true},
				{domain: "abc", ip: "10.10.10.9", shouldMatch: true},
				{domain: "abc", ip: "10.10.10.11", shouldMatch: true},
			},
			expectedCustomFilterCount: 2,
		},
		{
			name: "cidr exclude, then include ip and cidr for IPv4",
			config: `
filters:
  - match_ip: 10.10.10.0/24
    type: exclude
  - match_ip: 10.10.10.0/30
    type: include
  - match_ip: 10.10.10.100
    type: include
`,
			expectedMatches: []expectedMatch{
				{domain: "abc", ip: "10.10.10.0", shouldMatch: true},
				{domain: "abc", ip: "10.10.10.1", shouldMatch: true},
				{domain: "abc", ip: "10.10.10.2", shouldMatch: true},
				{domain: "abc", ip: "10.10.10.3", shouldMatch: true},
				{domain: "abc", ip: "10.10.10.4", shouldMatch: false},
				{domain: "abc", ip: "10.10.10.5", shouldMatch: false},
				{domain: "abc", ip: "10.10.10.100", shouldMatch: true},
				{domain: "abc", ip: "10.10.10.101", shouldMatch: false},
				{domain: "abc", ip: "10.10.10.254", shouldMatch: false},
				{domain: "abc", ip: "10.10.10.255", shouldMatch: false},
			},
			expectedCustomFilterCount: 3,
		},
		{
			name: "cidr exclude, then include ip and cidr for IPv6",
			config: `
filters:
  - match_ip: 2001:4860:0000:0000:0000:0000:0000:0000/32
    type: exclude
  - match_ip: 2001:4860:0000:0000:0000:0000:0000:0010
    type: include
`,
			expectedMatches: []expectedMatch{
				{ip: "2001:4860:0000:0000:0000:0000:0000:0000", shouldMatch: false},
				{ip: "2001:4860:ffff:ffff:ffff:ffff:ffff:ffff", shouldMatch: false},
				{ip: "2001:4860:0000:0000:0000:0000:0000:0010", shouldMatch: true},
			},
			expectedCustomFilterCount: 2,
		},
		{
			name: "cidr parsing error",
			config: `
filters:
  - match_ip: 2001:4860:0000:0000:0000:0000:0000:0000/999
    type: exclude
`,
			expectedErr:               "failed to parsing match_ip",
			expectedCustomFilterCount: 0,
		},
		{
			name: "exclude all domain, then include",
			config: `
filters:
  - match_domain: '*'
    type: exclude
  - match_domain: 'abc.google.com'
    type: include
  - match_domain: '*.datadoghq.com'
    type: include
`,
			expectedMatches: []expectedMatch{
				{domain: "zoom.us", shouldMatch: false},
				{domain: "dns.google.com", shouldMatch: false},
				{domain: "123.google.com", shouldMatch: false},
				{domain: "abc.google.com", shouldMatch: true},
				{domain: "abc.datadoghq.com", shouldMatch: true},
				{domain: "123.datadoghq.com", shouldMatch: true},
			},
			expectedCustomFilterCount: 3,
		},
		{
			name: "exclude all ip, then include",
			config: `
filters:
  - match_ip: 0.0.0.0/0
    type: exclude
  - match_ip: 10.10.10.0/30
    type: include
  - match_ip: 10.10.10.100
    type: include
`,
			expectedMatches: []expectedMatch{
				{ip: "10.10.20.0", shouldMatch: false},
				{ip: "10.10.10.0", shouldMatch: true},
				{ip: "10.10.10.1", shouldMatch: true},
				{ip: "10.10.10.2", shouldMatch: true},
				{ip: "10.10.10.3", shouldMatch: true},
				{ip: "10.10.10.4", shouldMatch: false},
				{ip: "10.10.10.100", shouldMatch: true},
			},
			expectedCustomFilterCount: 3,
		},
		{
			name: "invalid domain",
			config: `
filters:
  - match_domain: '*//$[.google.com'
    match_domain_strategy: 'regex'
    type: exclude
`,
			expectedErr:               "error building regex",
			expectedCustomFilterCount: 0,
		},
		{
			name: "invalid domain and valid domain",
			config: `
filters:
  - match_domain: '*//$[.google.com'
    match_domain_strategy: 'regex'
    type: exclude
  - match_domain: 'zoom.us'
    type: exclude
`,
			expectedMatches: []expectedMatch{
				{domain: "zoom.us", shouldMatch: false},
				{domain: "dns.google.com", shouldMatch: true},
				{domain: "abc.google.com", shouldMatch: true},
			},
			expectedErr:               "error building regex",
			expectedCustomFilterCount: 1,
		},
		{
			name:   "default datadog domain excluded with site",
			config: ``,
			ddSite: "datad0g.com",
			expectedMatches: []expectedMatch{
				{domain: "zoom.us", shouldMatch: true},
				{domain: "dns.datadoghq.com", shouldMatch: false},
				{domain: "dns.datadoghq.eu", shouldMatch: false},
				{domain: "abc.datad0g.com", shouldMatch: false},
				{domain: "1.datadog.pool.ntp.org", shouldMatch: false},
			},
			expectedErr:               "",
			expectedCustomFilterCount: 0,
		},
		{
			name:   "default datadog domain excluded without site",
			config: ``,
			ddSite: "",
			expectedMatches: []expectedMatch{
				{domain: "zoom.us", shouldMatch: true},
				{domain: "dns.datadoghq.com", shouldMatch: false},
				{domain: "dns.datadoghq.eu", shouldMatch: false},
				{domain: "abc.datad0g.com", shouldMatch: true},
				{domain: "1.datadog.pool.ntp.org", shouldMatch: false},
			},
			expectedErr: "",
		},
		{
			name: "include all domain",
			config: `
filters:
  - match_domain: '*'
    type: include
`,
			ddSite: "datad0g.com",
			expectedMatches: []expectedMatch{
				{domain: "", shouldMatch: true},
				{domain: "dns.datadoghq.com", shouldMatch: true},
				{domain: "dns.datadoghq.eu", shouldMatch: true},
				{domain: "abc.datad0g.com", shouldMatch: true},
				{domain: "1.datadog.pool.ntp.org", shouldMatch: true},
			},
			expectedErr:               "",
			expectedCustomFilterCount: 1,
		},
		{
			name: "invalid filter type",
			config: `
filters:
  - match_domain: 'zoom.us'
    type: invalid
`,
			expectedErr:               "invalid filter type: invalid",
			expectedCustomFilterCount: 0,
		},
		{
			name: "valid and invalid filter type",
			config: `
filters:
  - match_domain: 'google.com'
    type: exclude
  - match_domain: 'zoom.us'
    type: invalid
`,
			expectedMatches: []expectedMatch{
				{domain: "google.com", shouldMatch: false},
			},
			expectedErr:               "invalid filter type: invalid",
			expectedCustomFilterCount: 1,
		},
		{
			name: "monitor IP without domain enabled",
			config: `
filters:
`,
			ddSite:                 "datad0g.com",
			monitorIPWithoutDomain: true,
			expectedMatches: []expectedMatch{
				{domain: "", ip: "1.1.1.1", shouldMatch: true},
				{domain: "cloudflare", ip: "1.1.1.1", shouldMatch: true},
			},
			expectedErr:               "",
			expectedCustomFilterCount: 1,
		},
		{
			name: "monitor IP without domain disabled",
			config: `
filters:
`,
			ddSite:                 "datad0g.com",
			monitorIPWithoutDomain: false,
			expectedMatches: []expectedMatch{
				{domain: "", ip: "1.1.1.1", shouldMatch: false},
				{domain: "cloudflare", ip: "1.1.1.1", shouldMatch: true},
			},
			expectedErr:               "",
			expectedCustomFilterCount: 0,
		},
		{
			name: "monitor IP without domain disabled with filters",
			config: `
filters:
  - match_ip: 10.10.10.0/30
    type: include
  - match_ip: 10.10.10.100
    type: include
`,
			ddSite:                 "datad0g.com",
			monitorIPWithoutDomain: false,
			expectedMatches: []expectedMatch{
				{domain: "cloudflare", ip: "1.1.1.1", shouldMatch: true},
				{ip: "1.1.1.1", shouldMatch: false},
				{ip: "10.10.20.0", shouldMatch: false},
				{ip: "10.10.10.0", shouldMatch: true},
				{ip: "10.10.10.1", shouldMatch: true},
				{ip: "10.10.10.2", shouldMatch: true},
				{ip: "10.10.10.3", shouldMatch: true},
				{ip: "10.10.10.4", shouldMatch: false},
				{ip: "10.10.10.100", shouldMatch: true},
			},
			expectedErr:               "",
			expectedCustomFilterCount: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connFilter, err := getConnFilter(t, tt.config, tt.ddSite, tt.monitorIPWithoutDomain)
			if tt.expectedErr != "" {
				assert.ErrorContains(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, connFilter.filters, tt.expectedCustomFilterCount+len(getDefaultConnFilters(tt.ddSite, false)))
			for _, expMatch := range tt.expectedMatches {
				require.NotNil(t, connFilter)
				if expMatch.ip == "" {
					assert.Equal(t, connFilter.IsIncluded(expMatch.domain, netip.Addr{}), expMatch.shouldMatch, expMatch)
				} else {
					assert.Equal(t, connFilter.IsIncluded(expMatch.domain, netip.MustParseAddr(expMatch.ip)), expMatch.shouldMatch, expMatch)
				}
			}
		})
	}
}
