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
			name: "single ip exclude",
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
			name: "cidr exclude, then include ip and cidr",
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

		// TODO: TEST FOR ALL CASES
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connFilter, err := getConnFilter(t, tt.config)
			if tt.expectedErr != "" {
				assert.EqualError(t, err, tt.expectedErr)
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
