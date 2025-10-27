// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/stretchr/testify/assert"
)

func Test_convertProtocol(t *testing.T) {
	assert.Equal(t, convertProtocol(model.ConnectionType_udp), payload.ProtocolUDP)
	assert.Equal(t, convertProtocol(model.ConnectionType_tcp), payload.ProtocolTCP)
}

func Test_getDNSNameForIP(t *testing.T) {
	tests := []struct {
		name     string
		conns    *model.Connections
		ip       string
		expected string
	}{
		{
			name: "IP exists with single DNS name",
			conns: &model.Connections{
				Dns: map[string]*model.DNSEntry{
					"1.2.3.4": {
						Names: []string{"example.com"},
					},
				},
			},
			ip:       "1.2.3.4",
			expected: "example.com",
		},
		{
			name: "IP exists with multiple DNS names - returns first",
			conns: &model.Connections{
				Dns: map[string]*model.DNSEntry{
					"1.2.3.4": {
						Names: []string{"example.com", "example.org", "example.net"},
					},
				},
			},
			ip:       "1.2.3.4",
			expected: "example.com",
		},
		{
			name: "IP exists but DNSEntry has no names",
			conns: &model.Connections{
				Dns: map[string]*model.DNSEntry{
					"1.2.3.4": {
						Names: []string{},
					},
				},
			},
			ip:       "1.2.3.4",
			expected: "",
		},
		{
			name: "IP exists but DNSEntry is nil",
			conns: &model.Connections{
				Dns: map[string]*model.DNSEntry{
					"1.2.3.4": nil,
				},
			},
			ip:       "1.2.3.4",
			expected: "",
		},
		{
			name: "IP does not exist in DNS map",
			conns: &model.Connections{
				Dns: map[string]*model.DNSEntry{
					"1.2.3.4": {
						Names: []string{"example.com"},
					},
				},
			},
			ip:       "5.6.7.8",
			expected: "",
		},
		{
			name: "DNS map is nil",
			conns: &model.Connections{
				Dns: nil,
			},
			ip:       "1.2.3.4",
			expected: "",
		},
		{
			name: "DNS map is empty",
			conns: &model.Connections{
				Dns: map[string]*model.DNSEntry{},
			},
			ip:       "1.2.3.4",
			expected: "",
		},
		{
			name: "IPv6 address with DNS name",
			conns: &model.Connections{
				Dns: map[string]*model.DNSEntry{
					"2001:db8::1": {
						Names: []string{"ipv6.example.com"},
					},
				},
			},
			ip:       "2001:db8::1",
			expected: "ipv6.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getDNSNameForIP(tt.conns, tt.ip)
			assert.Equal(t, tt.expected, result)
		})
	}
}
