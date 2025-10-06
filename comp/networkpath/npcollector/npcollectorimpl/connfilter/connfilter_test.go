package connfilter

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
			name: "exclude domain",
			config: `
filters:
  - match_domain: '.*\.google\.com'
    type: exclude
`,
			expectedMatches: []expectedMatch{
				{domain: "zoom.us", ip: "0.0.0.0", shouldMatch: true},
				{domain: "dns.google.com", ip: "0.0.0.0", shouldMatch: false},
				{domain: "abc.google.com", ip: "0.0.0.0", shouldMatch: false},
			},
		},

		// TODO: TEST FOR ALL CASES
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connFilter, err := getConnFilter(t, tt.config)
			if tt.expectedErr != "" {
				assert.EqualError(t, err, tt.expectedErr)
			}
			for _, expMatch := range tt.expectedMatches {
				assert.Equal(t, connFilter.Match(expMatch.domain, expMatch.ip), expMatch.shouldMatch)
			}
		})
	}
}
