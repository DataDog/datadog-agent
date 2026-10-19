// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eudm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatHostname(t *testing.T) {
	for _, tc := range []struct{ name, serial, want string }{
		{"MacBook Pro", "C02ABC123", "macbook-pro-c02abc123"},
		{"MacBook Pro", "C02ABC456", "macbook-pro-c02abc456"},
		{"  Alice’s MacBook  ", " abc-123 ", "alice-s-macbook-abc-123"},
		{"EC2AMAZ-TEST", "VMware-56 4d ab", "ec2amaz-test-vmware-56-4d-ab"},
		{"ip-10-0-0-1", "ABC123", "ip-10-0-0-1-abc123"},
		{strings.Repeat("a", 300), "ABC123", strings.Repeat("a", 246) + "-abc123"},
	} {
		t.Run(tc.want[:min(len(tc.want), 40)], func(t *testing.T) {
			got, err := formatHostname(tc.name, tc.serial)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestInvalidIdentity(t *testing.T) {
	for _, serial := range []string{"", "  ", "Unknown", "None", "N/A", "Not Available", "Default string", "System Serial Number", "To Be Filled By O.E.M.", "00000000", "FF-FF-FF", strings.Repeat("a", 252)} {
		t.Run(serial, func(t *testing.T) {
			_, err := formatHostname("device", serial)
			require.Error(t, err)
		})
	}
	for _, name := range []string{"", "---", "你好"} {
		_, err := formatHostname(name, "ABC123")
		require.Error(t, err)
	}
}
