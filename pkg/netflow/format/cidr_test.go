// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCIDR(t *testing.T) {
	tests := []struct {
		name                  string
		ipAddr                []byte
		maskRawValue          uint32
		expectedFormattedMask string
	}{
		{
			name:                  "ipv4 case1",
			ipAddr:                []byte{192, 1, 128, 108},
			maskRawValue:          26,
			expectedFormattedMask: "192.1.128.64/26",
		},
		{
			name:                  "ipv4 case2",
			ipAddr:                []byte{192, 1, 128, 54},
			maskRawValue:          25,
			expectedFormattedMask: "192.1.128.0/25",
		},
		{
			name:                  "ipv6 case1",
			ipAddr:                net.ParseIP("2001:0DB8:ABCD:0012:0000:0000:0000:0010"),
			maskRawValue:          112,
			expectedFormattedMask: "2001:db8:abcd:12::/112",
		},
		{
			name:                  "ipv6 localhost mask 128",
			ipAddr:                net.ParseIP("::1"),
			maskRawValue:          128,
			expectedFormattedMask: "::1/128",
		},
		{
			name:                  "ipv6 localhost mask 128 long form",
			ipAddr:                net.ParseIP("0:0:0:0:0:0:0:1"),
			maskRawValue:          128,
			expectedFormattedMask: "::1/128",
		},
		{
			name:                  "ipv6 localhost mask 127",
			ipAddr:                net.ParseIP("::1"),
			maskRawValue:          127,
			expectedFormattedMask: "::/127",
		},
		{
			name:                  "invalid ipv6 mask",
			ipAddr:                net.ParseIP("2001:0DB8:ABCD:0012:0000:0000:0000:0010"),
			maskRawValue:          300,
			expectedFormattedMask: "/300",
		},
		{
			name:                  "empty ip bytes",
			ipAddr:                []byte{},
			maskRawValue:          20,
			expectedFormattedMask: "/20",
		},
		{
			name:                  "invalid mask",
			ipAddr:                []byte{192, 1, 128, 108},
			maskRawValue:          50,
			expectedFormattedMask: "/50",
		},
		{
			name:                  "invalid ip",
			ipAddr:                []byte{0},
			maskRawValue:          20,
			expectedFormattedMask: "/20",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.expectedFormattedMask, CIDR(tt.ipAddr, tt.maskRawValue), "FormatMask(%v, %v)", tt.ipAddr, tt.maskRawValue)
		})
	}
}
