// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import (
	"math"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDirection(t *testing.T) {
	assert.Equal(t, "ingress", Direction(uint32(0)))
	assert.Equal(t, "egress", Direction(uint32(1)))
	assert.Equal(t, "ingress", Direction(uint32(99))) // invalid direction will default to ingress
}

func TestEtherType(t *testing.T) {
	assert.Equal(t, "", EtherType(0))
	assert.Equal(t, "", EtherType(0x8888))
	assert.Equal(t, "IPv4", EtherType(0x0800))
	assert.Equal(t, "IPv6", EtherType(0x86DD))
}

func TestMacAddress(t *testing.T) {
	assert.Equal(t, "82:a5:6e:a5:aa:99", MacAddress(uint64(143647037565593)))
	assert.Equal(t, "00:00:00:00:00:00", MacAddress(uint64(0)))
}

func TestTCPFlags(t *testing.T) {
	tests := []struct {
		name          string
		flags         uint32
		expectedFlags []string
	}{
		{
			name:          "no flag",
			flags:         uint32(0),
			expectedFlags: nil,
		},
		{
			name:          "FIN",
			flags:         uint32(1),
			expectedFlags: []string{"FIN"},
		},
		{
			name:          "SYN",
			flags:         uint32(2),
			expectedFlags: []string{"SYN"},
		},
		{
			name:          "RST",
			flags:         uint32(4),
			expectedFlags: []string{"RST"},
		},
		{
			name:          "PSH",
			flags:         uint32(8),
			expectedFlags: []string{"PSH"},
		},
		{
			name:          "ACK",
			flags:         uint32(16),
			expectedFlags: []string{"ACK"},
		},
		{
			name:          "URG",
			flags:         uint32(32),
			expectedFlags: []string{"URG"},
		},
		{
			name:          "FIN SYN ACK",
			flags:         uint32(19),
			expectedFlags: []string{"FIN", "SYN", "ACK"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualFlags := TCPFlags(tt.flags)
			assert.Equal(t, tt.expectedFlags, actualFlags)
		})
	}
}

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
			assert.Equalf(t, tt.expectedFormattedMask, CIDR(tt.ipAddr, tt.maskRawValue), "format.CIDR(%v, %v)", tt.ipAddr, tt.maskRawValue)
		})
	}
}

func TestIPAddr(t *testing.T) {
	assert.Equal(t, "0.0.0.0", IPAddr([]byte{0, 0, 0, 0}))
	assert.Equal(t, "1.2.3.4", IPAddr([]byte{1, 2, 3, 4}))
	assert.Equal(t, "127.0.0.1", IPAddr([]byte{127, 0, 0, 1}))
	assert.Equal(t, "255.255.255.255", IPAddr([]byte{255, 255, 255, 255}))
	assert.Equal(t, "255.255.255.255", IPAddr([]byte{255, 255, 255, 255}))
	assert.Equal(t, "7f00::505:505:505", IPAddr([]byte{127, 0, 0, 0, 0, 0, 0, 0, 0, 0, 5, 5, 5, 5, 5, 5}))
}

func TestPort(t *testing.T) {
	assert.Equal(t, "65535", Port(math.MaxUint16))
	assert.Equal(t, "10", Port(10))
	assert.Equal(t, "0", Port(0))
	assert.Equal(t, "*", Port(-1))
	assert.Equal(t, "invalid", Port(-10))
}
