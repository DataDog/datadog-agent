// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package flowaggregator

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
)

func TestNetflowProtocolToConnectionType(t *testing.T) {
	tests := []struct {
		name       string
		ipProtocol uint32
		expected   model.ConnectionType
		ok         bool
	}{
		{
			name:       "tcp",
			ipProtocol: 6,
			expected:   model.ConnectionType_tcp,
			ok:         true,
		},
		{
			name:       "udp",
			ipProtocol: 17,
			expected:   model.ConnectionType_udp,
			ok:         true,
		},
		{
			name:       "unsupported protocol",
			ipProtocol: 1,
			expected:   0,
			ok:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, ok := netflowProtocolToConnectionType(tt.ipProtocol)
			assert.Equal(t, tt.expected, actual)
			assert.Equal(t, tt.ok, ok)
		})
	}
}

func TestToUint16Port(t *testing.T) {
	tests := []struct {
		name     string
		port     int32
		expected uint16
		ok       bool
	}{
		{
			name:     "zero",
			port:     0,
			expected: 0,
			ok:       true,
		},
		{
			name:     "max uint16",
			port:     65535,
			expected: 65535,
			ok:       true,
		},
		{
			name:     "negative",
			port:     -1,
			expected: 0,
			ok:       false,
		},
		{
			name:     "overflow",
			port:     65536,
			expected: 0,
			ok:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, ok := toUint16Port(tt.port)
			assert.Equal(t, tt.expected, actual)
			assert.Equal(t, tt.ok, ok)
		})
	}
}
