// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestDNSKey(t *testing.T) {
	tests := []struct {
		name         string
		c            *ConnectionStats
		allowedPorts []int
		expectedKey  dns.Key
		expectedOk   bool
	}{
		{
			name: "DNS port 53",
			c: &ConnectionStats{
				ConnectionTuple: ConnectionTuple{
					DPort:  53,
					Source: util.AddressFromString("1.2.3.4"),
					Dest:   util.AddressFromString("5.6.7.8"),
					SPort:  12345,
					Type:   UDP,
				},
			},
			allowedPorts: []int{53},
			expectedOk:   true,
		},
		{
			name: "Non-DNS port 80",
			c: &ConnectionStats{
				ConnectionTuple: ConnectionTuple{
					DPort:  80,
					Source: util.AddressFromString("1.2.3.4"),
					Dest:   util.AddressFromString("5.6.7.8"),
					SPort:  12345,
					Type:   TCP,
				},
			},
			allowedPorts: []int{53},
			expectedOk:   false,
		},
		{
			name: "Custom DNS port 5353",
			c: &ConnectionStats{
				ConnectionTuple: ConnectionTuple{
					DPort:  5353,
					Source: util.AddressFromString("1.2.3.4"),
					Dest:   util.AddressFromString("5.6.7.8"),
					SPort:  12345,
					Type:   UDP,
				},
			},
			allowedPorts: []int{53, 5353},
			expectedOk:   true,
		},
		{
			name: "Custom DNS port 5353 not in allowed list",
			c: &ConnectionStats{
				ConnectionTuple: ConnectionTuple{
					DPort:  5353,
					Source: util.AddressFromString("1.2.3.4"),
					Dest:   util.AddressFromString("5.6.7.8"),
					SPort:  12345,
					Type:   UDP,
				},
			},
			allowedPorts: []int{53},
			expectedOk:   false,
		},
		{
			name:         "Nil connection",
			c:            nil,
			allowedPorts: []int{53},
			expectedOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := DNSKey(tt.c, tt.allowedPorts)
			assert.Equal(t, tt.expectedOk, ok)
		})
	}
}
