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

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
)

func TestProtocolValue(t *testing.T) {
	tests := []struct {
		name        string
		kernelValue http.ProtocolType
		expected    ProtocolType
	}{
		{
			name:        "ProtocolUnknown",
			kernelValue: http.ProtocolUnknown,
			expected:    ProtocolUnknown,
		},
		{
			name:        "ProtocolHTTP",
			kernelValue: http.ProtocolHTTP,
			expected:    ProtocolHTTP,
		},
		{
			name:        "ProtocolHTTP2",
			kernelValue: http.ProtocolHTTP2,
			expected:    ProtocolHTTP2,
		},
		{
			name:        "ProtocolTLS",
			kernelValue: http.ProtocolTLS,
			expected:    ProtocolTLS,
		},
		{
			name:        "ProtocolAMQP",
			kernelValue: http.ProtocolAMQP,
			expected:    ProtocolAMQP,
		},
		{
			name:        "ProtocolRedis",
			kernelValue: http.ProtocolRedis,
			expected:    ProtocolRedis,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, int(test.expected), int(test.kernelValue))
			require.True(t, IsValidProtocolValue(uint8(test.kernelValue)))
		})
	}
}
