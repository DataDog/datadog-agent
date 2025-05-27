// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package client

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetDirectorIPAddress(t *testing.T) {
	tts := []struct {
		description string
		input       *DirectorStatus
		expected    string
		errMsg      string
	}{
		{
			description: "MyAddress and MyVnfManagementIPs exist, prefer MyAddress",
			input: &DirectorStatus{
				HAConfig: DirectorHAConfig{
					MyVnfManagementIPs: []string{
						"192.168.1.1",
						"192.168.1.2",
					},
					MyAddress: "10.0.0.1",
				},
			},
			expected: "10.0.0.1",
		},
		{
			description: "MyVnfManagementIPs exist, return first IP",
			input: &DirectorStatus{
				HAConfig: DirectorHAConfig{
					MyVnfManagementIPs: []string{
						"192.168.1.1",
						"192.168.1.2",
					},
				},
			},
			expected: "192.168.1.1",
		},
		{
			description: "MyVnfManagementIPs exist, single management address",
			input: &DirectorStatus{
				HAConfig: DirectorHAConfig{
					MyVnfManagementIPs: []string{
						"10.0.0.2",
					},
				},
			},
			expected: "10.0.0.2",
		},
		{
			description: "No list of IPs, use MyAddress",
			input: &DirectorStatus{
				HAConfig: DirectorHAConfig{
					MyAddress: "10.0.0.3",
				},
			},
			expected: "10.0.0.3",
		},
		{
			description: "No list of IPs or MyAddress, returns error",
			input:       &DirectorStatus{},
			errMsg:      "no management IPs found for director",
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			ip, err := test.input.IPAddress()
			if test.errMsg != "" {
				require.Error(t, err)
				require.True(t, strings.Contains(err.Error(), test.errMsg))
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, ip)
			}
		})
	}
}
