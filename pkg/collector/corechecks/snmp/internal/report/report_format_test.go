// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package report

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

func Test_formatValue(t *testing.T) {
	tests := []struct {
		name          string
		value         valuestore.ResultValue
		format        string
		expectedValue valuestore.ResultValue
		expectedError string
	}{
		{
			name: "mac_address: format mac_address",
			value: valuestore.ResultValue{
				Value: []byte{0x82, 0xa5, 0x6e, 0xa5, 0xc8, 0x01},
			},
			format: "mac_address",
			expectedValue: valuestore.ResultValue{
				Value: "82:a5:6e:a5:c8:01",
			},
		},
		{
			name: "ip_address: format IPv4 address",
			value: valuestore.ResultValue{
				Value: []byte{0x0a, 0x43, 0x00, 0x07},
			},
			format: "ip_address",
			expectedValue: valuestore.ResultValue{
				Value: "10.67.0.7",
			},
		},
		{
			name: "ip_address: format IPv6 address",
			value: valuestore.ResultValue{
				Value: []byte{0x20, 0x01, 0x48, 0x60, 0, 0, 0x20, 0x01, 0, 0, 0, 0, 0, 0, 0x00, 0x68},
			},
			format: "ip_address",
			expectedValue: valuestore.ResultValue{
				Value: "2001:4860:0:2001::68",
			},
		},
		{
			name: "ip_address: invalid ip_address",
			value: valuestore.ResultValue{
				Value: []byte{0x64, 0x43}, // only 2 bytes instead of 4 bytes (IPv4)
			},
			format: "ip_address",
			expectedValue: valuestore.ResultValue{
				Value: "?6443", // net.IP(...).String() will return `?` followed by hex when input is not IPv4 or IPv6
			},
		},
		{
			name: "ip_address: empty raw bytes",
			value: valuestore.ResultValue{
				Value: []byte{},
			},
			format: "ip_address",
			expectedValue: valuestore.ResultValue{
				Value: "",
			},
		},
		{
			name: "error unknown value type",
			value: valuestore.ResultValue{
				Value: valuestore.ResultValue{},
			},
			format:        "mac_address",
			expectedError: "value type `valuestore.ResultValue` not supported (format `mac_address`)",
		},
		{
			name: "error unknown format type",
			value: valuestore.ResultValue{
				Value: []byte{0x82, 0xa5, 0x6e, 0xa5, 0xc8, 0x01},
			},
			format:        "unknown_format",
			expectedError: "unknown format `unknown_format` (value type `[]uint8`)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := formatValue(tt.value, tt.format)
			assert.Equal(t, tt.expectedValue, value)
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			}
		})
	}
}
