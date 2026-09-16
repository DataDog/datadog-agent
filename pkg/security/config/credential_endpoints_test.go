// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config holds config related files
package config

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestCredentialEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name     string
		config   RuntimeSecurityConfig
		expected []CredentialEndpoint
	}{
		{
			name: "defaults",
			config: RuntimeSecurityConfig{
				IMDSIPv4:           "169.254.169.254",
				EKSPodIdentityIPv4: "169.254.170.23",
				EKSPodIdentityIPv6: "fd00:ec2::23",
			},
			expected: []CredentialEndpoint{
				{netip.MustParseAddr("169.254.169.254"), model.CredentialSourceIMDS},
				{netip.MustParseAddr("169.254.170.23"), model.CredentialSourceEKSPodIdentity},
				{netip.MustParseAddr("fd00:ec2::23"), model.CredentialSourceEKSPodIdentity},
			},
		},
		{
			name: "an empty address disables that endpoint",
			config: RuntimeSecurityConfig{
				IMDSIPv4:           "169.254.169.254",
				EKSPodIdentityIPv4: "",
				EKSPodIdentityIPv6: "",
			},
			expected: []CredentialEndpoint{
				{netip.MustParseAddr("169.254.169.254"), model.CredentialSourceIMDS},
			},
		},
		{
			name:     "everything disabled",
			config:   RuntimeSecurityConfig{},
			expected: nil,
		},
		{
			name: "custom addresses",
			config: RuntimeSecurityConfig{
				IMDSIPv4:           "10.0.0.1",
				EKSPodIdentityIPv4: "10.0.0.2",
			},
			expected: []CredentialEndpoint{
				{netip.MustParseAddr("10.0.0.1"), model.CredentialSourceIMDS},
				{netip.MustParseAddr("10.0.0.2"), model.CredentialSourceEKSPodIdentity},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoints, err := tc.config.CredentialEndpoints()
			require.NoError(t, err)
			assert.Equal(t, tc.expected, endpoints)
		})
	}
}

func TestCredentialEndpointsInvalid(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config RuntimeSecurityConfig
	}{
		{
			name:   "malformed ipv4",
			config: RuntimeSecurityConfig{IMDSIPv4: "not-an-ip"},
		},
		{
			name:   "ipv6 address in an ipv4 setting",
			config: RuntimeSecurityConfig{IMDSIPv4: "fd00:ec2::23"},
		},
		{
			name:   "ipv4 address in an ipv6 setting",
			config: RuntimeSecurityConfig{EKSPodIdentityIPv6: "169.254.170.23"},
		},
		{
			name:   "cidr instead of an address",
			config: RuntimeSecurityConfig{EKSPodIdentityIPv4: "169.254.170.23/32"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.config.CredentialEndpoints()
			assert.Error(t, err)
		})
	}
}
