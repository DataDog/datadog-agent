// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discovery

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_incrementIP(t *testing.T) {
	tests := []struct {
		name           string
		ip             net.IP
		expectedNextIP net.IP
	}{
		{
			name:           "next ip from 0",
			ip:             net.IPv4(127, 0, 0, 0),
			expectedNextIP: net.IPv4(127, 0, 0, 1),
		},
		{
			name:           "next ip from 2",
			ip:             net.IPv4(127, 0, 1, 2),
			expectedNextIP: net.IPv4(127, 0, 1, 3),
		},
		{
			name:           "next ip 255 to 0",
			ip:             net.IPv4(127, 0, 1, 255),
			expectedNextIP: net.IPv4(127, 0, 2, 0),
		},
		{
			name:           "next ip multiple digit change",
			ip:             net.IPv4(10, 10, 255, 255),
			expectedNextIP: net.IPv4(10, 11, 0, 0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startingIP := make(net.IP, len(tt.ip))
			copy(startingIP, tt.ip)
			incrementIP(startingIP)
			assert.Equal(t, tt.expectedNextIP, startingIP)
		})
	}
}
