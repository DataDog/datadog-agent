// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestIsIPPublic(t *testing.T) {
	bfh := BaseFieldHandlers{}
	for _, cidr := range setup.DefaultPrivateIPCIDRs {
		if err := bfh.privateCIDRs.AppendCIDR(cidr); err != nil {
			t.Fatalf("failed to append CIDR %s: %v", cidr, err)
		}
	}

	testCases := []struct {
		name     string
		ip       string
		expected bool
	}{
		{
			name:     "public 1",
			ip:       "11.1.1.1",
			expected: true,
		},
		{
			name:     "public 2",
			ip:       "172.48.1.1",
			expected: true,
		},
		{
			name:     "public 3",
			ip:       "192.167.1.1",
			expected: true,
		},
		{
			name:     "private in 24-bit block",
			ip:       "10.11.11.11",
			expected: false,
		},
		{
			name:     "private in 20-bit block",
			ip:       "172.24.11.11",
			expected: false,
		},
		{
			name:     "private in 16-bit block",
			ip:       "192.168.11.11",
			expected: false,
		},
		{
			name:     "IPv6 ULA",
			ip:       "fdf8:b35f:91b1::11",
			expected: false,
		},
		{
			name:     "IPv6 Global",
			ip:       "2001:0:0eab:dead::a0:abcd:4e",
			expected: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.expected, bfh.ResolveIsIPPublic(model.NewFakeEvent(), &model.IPPortContext{
				IPNet: *eval.IPNetFromIP(net.ParseIP(testCase.ip)),
			}))
		})
	}
}
