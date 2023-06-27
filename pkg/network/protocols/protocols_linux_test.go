// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package protocols

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
)

func TestProtocolValue(t *testing.T) {
	tests := []struct {
		name        string
		kernelValue http.ProtocolType
		expected    network.ProtocolType
	}{
		{
			name:        "ProtocolUnknown",
			kernelValue: http.ProtocolUnknown,
			expected:    network.ProtocolUnknown,
		},
		{
			name:        "ProtocolHTTP",
			kernelValue: http.ProtocolHTTP,
			expected:    network.ProtocolHTTP,
		},
		{
			name:        "ProtocolHTTP2",
			kernelValue: http.ProtocolHTTP2,
			expected:    network.ProtocolHTTP2,
		},
		{
			name:        "ProtocolTLS",
			kernelValue: http.ProtocolTLS,
			expected:    network.ProtocolTLS,
		},
		{
			name:        "ProtocolAMQP",
			kernelValue: http.ProtocolAMQP,
			expected:    network.ProtocolAMQP,
		},
		{
			name:        "ProtocolRedis",
			kernelValue: http.ProtocolRedis,
			expected:    network.ProtocolRedis,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, int(test.expected), int(test.kernelValue))
			require.True(t, network.IsValidProtocolValue(uint8(test.kernelValue)))
		})
	}
}
