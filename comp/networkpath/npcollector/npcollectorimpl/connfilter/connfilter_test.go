// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package connfilter

import (
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
		name            string
		config          string
		ddSite          string
		expectedMatches []expectedMatch
		expectedErr     string
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
		},
		{
			name: "invalid strategy",
			config: `
filters:
  - match_domain: '.*\.google\.com'
    match_domain_strategy: 'invalid'
    type: exclude
`,
			expectedErr: "invalid match domain strategy: invalid",
		},
		{
			name: "single ip exclude IPv4",
			config: `
filters:
  - match_ip: 10.10.10.10
    type: exclude
`,
			expectedMatches: []expectedMatch{
				{ip: "10.10.10.10", shouldMatch: false},
				{ip: "10.10.10.9", shouldMatch: true},
				{ip: "10.10.10.11", shouldMatch: true},
			},
		},
		{
			name: "single ip exclude IPv6",
			config: `
filters:
  - match_ip: 2001:4860:4860::8888
    type: exclude
`,
			expectedMatches: []expectedMatch{
				{ip: "2001:4860:4860::8888", shouldMatch: false},
				{ip: "2001:4860:4860::8844", shouldMatch: true},
			},
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
				{ip: "10.10.10.10", shouldMatch: true},
				{ip: "10.10.10.9", shouldMatch: true},
				{ip: "10.10.10.11", shouldMatch: true},
			},
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
				{ip: "10.10.10.0", shouldMatch: true},
				{ip: "10.10.10.1", shouldMatch: true},
				{ip: "10.10.10.2", shouldMatch: true},
				{ip: "10.10.10.3", shouldMatch: true},
				{ip: "10.10.10.4", shouldMatch: false},
				{ip: "10.10.10.5", shouldMatch: false},
				{ip: "10.10.10.100", shouldMatch: true},
				{ip: "10.10.10.101", shouldMatch: false},
				{ip: "10.10.10.255", shouldMatch: false},
				{ip: "10.10.10.256", shouldMatch: true},
			},
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
		},
		{
			name: "cidr parsing error",
			config: `
filters:
  - match_ip: 2001:4860:0000:0000:0000:0000:0000:0000/999
    type: exclude
`,
			expectedErr: "failed to parsing match_ip",
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
		},
		{
			name: "invalid domain",
			config: `
filters:
  - match_domain: '*//$[.google.com'
    match_domain_strategy: 'regex'
    type: exclude
`,
			expectedErr: "error building regex",
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
			expectedErr: "error building regex",
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
			expectedErr: "",
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
			expectedErr: "",
		},
		{
			name: "invalid filter type",
			config: `
filters:
  - match_domain: 'zoom.us'
    type: invalid
`,
			expectedErr: "invalid filter type: invalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connFilter, err := getConnFilter(t, tt.config, tt.ddSite)
			if tt.expectedErr != "" {
				assert.ErrorContains(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
			}
			for _, expMatch := range tt.expectedMatches {
				require.NotNil(t, connFilter)
				assert.Equal(t, connFilter.IsIncluded(expMatch.domain, expMatch.ip), expMatch.shouldMatch, expMatch)
			}
		})
	}
}
