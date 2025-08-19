// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package utils holds utils related files
package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsIPMatch(t *testing.T) {
	// DefaultPrivateIPCIDRs is a list of private IP CIDRs that are used to determine if an IP is private or not.
	var DefaultPrivateIPCIDRs = []string{
		// IETF RPC 1918
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		// IETF RFC 5735
		"0.0.0.0/8",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"192.88.99.0/24",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"240.0.0.0/4",
		// IETF RFC 6598
		"100.64.0.0/10",
		// // IETF RFC 4193
		"fc00::/7",
	}

	cset := CIDRSet{}
	for _, cidr := range DefaultPrivateIPCIDRs {
		if err := cset.AppendCIDR(cidr); err != nil {
			t.Fatalf("failed to append CIDR %s: %v", cidr, err)
		}
	}

	// cset.Debug()

	testCases := []struct {
		name     string
		ip       string
		expected bool
	}{
		{
			name:     "dont match 1",
			ip:       "11.1.1.1",
			expected: false,
		},

		{
			name:     "dont match 2",
			ip:       "172.48.1.1",
			expected: false,
		},

		{
			name:     "dont match 3",
			ip:       "192.167.1.1",
			expected: false,
		},

		{
			name:     "match in 24-bit block",
			ip:       "10.11.11.11",
			expected: true,
		},
		{
			name:     "match in 20-bit block",
			ip:       "172.24.11.11",
			expected: true,
		},
		{
			name:     "match in 16-bit block",
			ip:       "192.168.11.11",
			expected: true,
		},
		{
			name:     "IPv6 ULA",
			ip:       "fdf8:b35f:91b1::11",
			expected: true,
		},
		{
			name:     "IPv6 Global",
			ip:       "2001:0:0eab:dead::a0:abcd:4e",
			expected: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.expected, cset.MatchIP(testCase.ip))
		})
	}
}
